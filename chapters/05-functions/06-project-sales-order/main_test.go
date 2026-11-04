package main

import (
	"maps"
	"slices"
	"strconv"
	"testing"
)

func TestApplyDiscount(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name    string
		price   int
		percent int
		want    int
		wantOK  bool
	}{
		{name: "ordinary sale", price: 1250, percent: 20, want: 1000, wantOK: true},
		{name: "round down", price: 2599, percent: 20, want: 2079, wantOK: true},
		{name: "small price rounds up", price: 2, percent: 20, want: 2, wantOK: true},
		{name: "small price rounds down", price: 3, percent: 20, want: 2, wantOK: true},
		{name: "half cent rounds up", price: 5, percent: 50, want: 3, wantOK: true},
		{name: "zero price", percent: 20, wantOK: true},
		{name: "zero discount", price: 199, want: 199, wantOK: true},
		{name: "full discount", price: 199, percent: 100, wantOK: true},
		{name: "negative price", price: -1, percent: 20},
		{name: "negative percentage", price: 100, percent: -1},
		{name: "percentage above 100", price: 100, percent: 101},
		{name: "maximum price with no discount", price: maxInt, want: maxInt, wantOK: true},
		{name: "maximum price with half discount", price: maxInt, percent: 50, want: maxInt/2 + 1, wantOK: true},
		{name: "maximum price with full discount", price: maxInt, percent: 100, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, ok := applyDiscount(tt.price, tt.percent)
			if price != tt.want || ok != tt.wantOK {
				t.Errorf("applyDiscount(%d, %d) = (%d, %t), want (%d, %t)", tt.price, tt.percent, price, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCalculateItemPrice(t *testing.T) {
	catalog := map[string]int{
		"BOOK":         2599,
		"HUG":          1250,
		"FREE":         0,
		"INVALID":      -100,
		"SPECIAL":      2000,
		"SPECIAL_SALE": 1234,
		"":             100,
	}
	before := maps.Clone(catalog)
	tests := []struct {
		name   string
		code   string
		want   PricedItem
		wantOK bool
	}{
		{name: "regular item", code: "BOOK", want: PricedItem{Code: "BOOK", BasePriceCents: 2599, PriceCents: 2599}, wantOK: true},
		{name: "sale item", code: "HUG_SALE", want: PricedItem{Code: "HUG_SALE", BasePriceCents: 1250, PriceCents: 1000, DiscountPercent: 20}, wantOK: true},
		{name: "rounded sale item", code: "BOOK_SALE", want: PricedItem{Code: "BOOK_SALE", BasePriceCents: 2599, PriceCents: 2079, DiscountPercent: 20}, wantOK: true},
		{name: "free item", code: "FREE", want: PricedItem{Code: "FREE"}, wantOK: true},
		{name: "free sale item", code: "FREE_SALE", want: PricedItem{Code: "FREE_SALE", DiscountPercent: 20}, wantOK: true},
		{name: "exact match takes priority", code: "SPECIAL_SALE", want: PricedItem{Code: "SPECIAL_SALE", BasePriceCents: 1234, PriceCents: 1234}, wantOK: true},
		{name: "missing item", code: "MISSING"},
		{name: "missing sale item", code: "MISSING_SALE"},
		{name: "empty code"},
		{name: "suffix with no product code", code: "_SALE"},
		{name: "repeated suffix without a catalog match", code: "BOOK_SALE_SALE"},
		{name: "case sensitive code", code: "book"},
		{name: "whitespace is not normalized", code: " BOOK "},
		{name: "negative regular price", code: "INVALID"},
		{name: "negative sale price", code: "INVALID_SALE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, ok := calculateItemPrice(catalog, tt.code)
			if item != tt.want || ok != tt.wantOK {
				t.Errorf("calculateItemPrice(%q) = (%+v, %t), want (%+v, %t)", tt.code, item, ok, tt.want, tt.wantOK)
			}
		})
	}
	if !maps.Equal(catalog, before) {
		t.Fatal("pricing an item must not modify the catalog")
	}
}

func TestProcessOrder(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	catalog := map[string]int{
		"TSHIRT": 2000, "HUG": 1250, "HAT": 1800, "BOOK": 2599,
		"FREE": 0, "INVALID": -100, "MAX": maxInt, "ONE": 1,
	}
	tests := []struct {
		name        string
		codes       []string
		wantCodes   []string
		wantSkipped []string
		wantTotal   int
		wantOK      bool
	}{
		{name: "nil order", wantOK: true},
		{name: "empty order", codes: []string{}, wantOK: true},
		{name: "original order", codes: []string{"TSHIRT", "HUG_SALE", "HAT", "BOOK"}, wantCodes: []string{"TSHIRT", "HUG_SALE", "HAT", "BOOK"}, wantTotal: 7399, wantOK: true},
		{name: "mixed order", codes: []string{"BOOK_SALE", "MISSING", "HAT"}, wantCodes: []string{"BOOK_SALE", "HAT"}, wantSkipped: []string{"MISSING"}, wantTotal: 3879, wantOK: true},
		{name: "repeated items are separate units", codes: []string{"HUG_SALE", "HUG_SALE"}, wantCodes: []string{"HUG_SALE", "HUG_SALE"}, wantTotal: 2000, wantOK: true},
		{name: "free items are accepted", codes: []string{"FREE", "FREE_SALE"}, wantCodes: []string{"FREE", "FREE_SALE"}, wantOK: true},
		{name: "all rejected", codes: []string{"INVALID", "MISSING_SALE", ""}, wantSkipped: []string{"INVALID", "MISSING_SALE", ""}, wantOK: true},
		{name: "maximum subtotal is accepted", codes: []string{"MAX", "FREE"}, wantCodes: []string{"MAX", "FREE"}, wantTotal: maxInt, wantOK: true},
		{name: "subtotal overflow", codes: []string{"MAX", "ONE"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputCatalog := maps.Clone(catalog)
			inputCodes := slices.Clone(tt.codes)
			summary, ok := processOrder(inputCatalog, inputCodes)
			if !maps.Equal(inputCatalog, catalog) || !slices.Equal(inputCodes, tt.codes) {
				t.Fatal("processing an order must not modify its catalog or input codes")
			}
			if ok != tt.wantOK {
				t.Fatalf("processOrder success = %t, want %t", ok, tt.wantOK)
			}
			if !ok {
				if summary.SubtotalCents != 0 || len(summary.Items) != 0 || len(summary.SkippedCodes) != 0 {
					t.Fatal("overflow must return an empty summary instead of a partial total")
				}
				return
			}
			var codes []string
			for _, item := range summary.Items {
				codes = append(codes, item.Code)
			}
			if summary.SubtotalCents != tt.wantTotal || !slices.Equal(codes, tt.wantCodes) || !slices.Equal(summary.SkippedCodes, tt.wantSkipped) {
				t.Errorf("got total=%d codes=%v skipped=%v, want total=%d codes=%v skipped=%v", summary.SubtotalCents, codes, summary.SkippedCodes, tt.wantTotal, tt.wantCodes, tt.wantSkipped)
			}
		})
	}
}

func TestProcessOrderWithNilCatalog(t *testing.T) {
	summary, ok := processOrder(nil, []string{"BOOK", "BOOK_SALE"})
	if !ok || summary.SubtotalCents != 0 || len(summary.Items) != 0 || !slices.Equal(summary.SkippedCodes, []string{"BOOK", "BOOK_SALE"}) {
		t.Fatalf("nil catalog result = (%+v, %t)", summary, ok)
	}
}

func TestFormatCents(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	wantMin, wantMax := "-$21474836.48", "$21474836.47"
	if strconv.IntSize == 64 {
		wantMin, wantMax = "-$92233720368547758.08", "$92233720368547758.07"
	}
	tests := []struct {
		cents int
		want  string
	}{
		{cents: 0, want: "$0.00"},
		{cents: 1, want: "$0.01"},
		{cents: 99, want: "$0.99"},
		{cents: 100, want: "$1.00"},
		{cents: 2599, want: "$25.99"},
		{cents: -1, want: "-$0.01"},
		{cents: -1299, want: "-$12.99"},
		{cents: -maxInt - 1, want: wantMin},
		{cents: maxInt, want: wantMax},
	}
	for _, tt := range tests {
		if got := formatCents(tt.cents); got != tt.want {
			t.Errorf("formatCents(%d) = %q, want %q", tt.cents, got, tt.want)
		}
	}
}
