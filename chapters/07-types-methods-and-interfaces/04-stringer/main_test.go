package main

import (
	"fmt"
	"strconv"
	"testing"
)

func TestMoneyCentsString(t *testing.T) {
	tests := []struct {
		name  string
		value MoneyCents
		want  string
	}{
		{name: "zero", value: 0, want: "$0.00"},
		{name: "one cent", value: 1, want: "$0.01"},
		{name: "fractional dollar", value: 99, want: "$0.99"},
		{name: "whole dollar", value: 100, want: "$1.00"},
		{name: "positive amount", value: 1299, want: "$12.99"},
		{name: "negative cent", value: -1, want: "-$0.01"},
		{name: "negative fractional dollar", value: -99, want: "-$0.99"},
		{name: "negative whole dollar", value: -100, want: "-$1.00"},
		{name: "negative amount", value: -1299, want: "-$12.99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.String(); got != tt.want {
				t.Errorf("MoneyCents(%d).String() = %q, want %q", int(tt.value), got, tt.want)
			}
		})
	}
}

func TestMoneyCentsStringIntLimits(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	wantMin, wantMax := "-$21474836.48", "$21474836.47"
	if strconv.IntSize == 64 {
		wantMin, wantMax = "-$92233720368547758.08", "$92233720368547758.07"
	}

	if got := MoneyCents(minInt).String(); got != wantMin {
		t.Errorf("minimum int: got %q, want %q", got, wantMin)
	}
	if got := MoneyCents(maxInt).String(); got != wantMax {
		t.Errorf("maximum int: got %q, want %q", got, wantMax)
	}
}

func TestOrderStatusString(t *testing.T) {
	tests := []struct {
		status OrderStatus
		want   string
	}{
		{status: StatusUnknown, want: "unknown"},
		{status: StatusPending, want: "pending"},
		{status: StatusPaid, want: "paid"},
		{status: StatusShipped, want: "shipped"},
		{status: StatusCanceled, want: "canceled"},
		{status: OrderStatus(-1), want: "OrderStatus(-1)"},
		{status: OrderStatus(99), want: "OrderStatus(99)"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("OrderStatus(%d).String() = %q, want %q", int(tt.status), got, tt.want)
		}
	}
}

func TestStringerFormattingVerbs(t *testing.T) {
	if got := fmt.Sprintf("%v %s %d", ID(30), ID(30), ID(30)); got != "ID-030 ID-030 30" {
		t.Errorf("ID formatting = %q", got)
	}
	if got := fmt.Sprint(MoneyCents(-1299)); got != "-$12.99" {
		t.Errorf("money formatting = %q", got)
	}
}
