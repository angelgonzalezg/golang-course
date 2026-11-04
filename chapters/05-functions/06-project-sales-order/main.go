// Project: Sales Order Processor
// Topic: combines maps, conditionals, strings, functions, and loops.
// Args/Input: a catalog stores integer cents; a trailing _SALE requests a 20% discount.
// Usage: run from the repository root with go run ./chapters/05-functions/06-project-sales-order.
// Key idea: calculate and validate data in small functions, then present the result separately.
package main

import (
	"fmt"
	"math"
	"strings"
)

const (
	saleSuffix          = "_SALE"
	saleDiscountPercent = 20
)

/* Helper section 1: Data returned by pricing functions */
type PricedItem struct {
	Code            string
	BasePriceCents  int
	PriceCents      int
	DiscountPercent int
}

type OrderSummary struct {
	Items         []PricedItem
	SkippedCodes  []string
	SubtotalCents int
}

/* Helper section 2: Integer arithmetic and validation */
// applyDiscount rounds the final price to the nearest whole cent, with half cents rounded up.
// Prices must be nonnegative and the discount must be between 0 and 100 inclusive.
func applyDiscount(priceCents, discountPercent int) (int, bool) {
	if priceCents < 0 || discountPercent < 0 || discountPercent > 100 {
		return 0, false
	}

	keptPercent := 100 - discountPercent
	// Splitting quotient and remainder avoids overflowing priceCents * keptPercent.
	wholeCents := (priceCents / 100) * keptPercent
	// Adding 50 before dividing by 100 implements half-up rounding.
	roundedRemainder := ((priceCents%100)*keptPercent + 50) / 100
	return wholeCents + roundedRemainder, true
}

/* Helper section 3: Price an item without printing or changing the catalog */
// calculateItemPrice returns false for an empty/unavailable code or a negative catalog price.
func calculateItemPrice(catalog map[string]int, itemCode string) (PricedItem, bool) {
	if itemCode == "" {
		return PricedItem{}, false
	}

	basePrice, found := catalog[itemCode]
	discountPercent := 0
	// An exact catalog match takes priority, even when its code ends in _SALE.
	if !found {
		if !strings.HasSuffix(itemCode, saleSuffix) {
			return PricedItem{}, false
		}
		originalCode := strings.TrimSuffix(itemCode, saleSuffix)
		if originalCode == "" {
			return PricedItem{}, false
		}
		basePrice, found = catalog[originalCode]
		if !found {
			return PricedItem{}, false
		}
		discountPercent = saleDiscountPercent
	}

	price, ok := applyDiscount(basePrice, discountPercent)
	if !ok {
		return PricedItem{}, false
	}
	return PricedItem{
		Code:            itemCode,
		BasePriceCents:  basePrice,
		PriceCents:      price,
		DiscountPercent: discountPercent,
	}, true
}

/* Helper section 4: Build an order summary without presentation side effects */
// processOrder skips invalid items and charges each occurrence as one unit.
// A false result means the subtotal would overflow int; the returned summary is then zero.
func processOrder(catalog map[string]int, itemCodes []string) (OrderSummary, bool) {
	var summary OrderSummary
	for _, code := range itemCodes {
		item, ok := calculateItemPrice(catalog, code)
		if !ok {
			summary.SkippedCodes = append(summary.SkippedCodes, code)
			continue
		}
		// All accepted prices are nonnegative, so this subtraction is safe.
		if item.PriceCents > math.MaxInt-summary.SubtotalCents {
			return OrderSummary{}, false
		}
		summary.Items = append(summary.Items, item)
		summary.SubtotalCents += item.PriceCents
	}
	return summary, true
}

/* Helper section 5: Format and print already calculated data */
// formatCents formats signed integer cents without converting to floating point.
func formatCents(cents int) string {
	dollars, remainder := cents/100, cents%100
	sign := ""
	if cents < 0 {
		sign = "-"
		// Negating the divided values also works for the minimum int.
		dollars, remainder = -dollars, -remainder
	}
	return fmt.Sprintf("%s$%d.%02d", sign, dollars, remainder)
}

func printOrder(summary OrderSummary) {
	for _, item := range summary.Items {
		if item.DiscountPercent > 0 {
			fmt.Printf(" - Item %s (Sale! %d%% off, Original: %s, Price: %s)\n",
				item.Code, item.DiscountPercent, formatCents(item.BasePriceCents), formatCents(item.PriceCents))
		} else {
			fmt.Printf(" - Item %s (Price: %s)\n", item.Code, formatCents(item.PriceCents))
		}
	}
	for _, code := range summary.SkippedCodes {
		fmt.Printf(" - Item %q (Unavailable or invalid price; skipped)\n", code)
	}
	fmt.Printf("Subtotal Price: %s\n", formatCents(summary.SubtotalCents))
	fmt.Printf("Processed: %d, skipped: %d\n", len(summary.Items), len(summary.SkippedCodes))
}

func main() {
	// Pass the catalog explicitly so pricing functions can be reused and tested with other data.
	// Each integer is a price in cents: 2599 means $25.99.
	catalog := map[string]int{
		"TSHIRT": 2000,
		"HUG":    1250,
		"HAT":    1800,
		"BOOK":   2599,
	}

	examples := []struct {
		name  string
		codes []string
	}{
		{name: "Original order", codes: []string{"TSHIRT", "HUG_SALE", "HAT", "BOOK"}},
		{name: "Rounded sale price and missing product", codes: []string{"BOOK_SALE", "MISSING", "HAT"}},
		{name: "Empty order"},
	}

	fmt.Println("---------- Simple Sales Order Processor ----------")
	for _, example := range examples {
		fmt.Printf("\n== %s ==\n", example.name)
		summary, ok := processOrder(catalog, example.codes)
		if !ok {
			fmt.Println("Order subtotal exceeds the supported integer range.")
			continue
		}
		printOrder(summary)
	}
}
