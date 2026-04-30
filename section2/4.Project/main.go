// Project: Sales Order Processor
// Topic: combines maps, conditionals, strings, functions, and loops.
// Args/Input: item codes are searched in a price map; _SALE codes get a discount.
// Usage: calculate each item price, skip missing products, and build a subtotal.
// Key idea: combine small language features to solve a practical processing task.
package main

import (
	"fmt"
	"strings"
)

var productPrices = map[string]float64{
	"TSHIRT": 20.00,
	"HUG":    12.5,
	"HAT":    18.00,
	"BOOK":   25.99,
}

// calculateItemPrice returns the price and whether the item was found.
func calculateItemPrice(itemCode string) (float64, bool) {
	basePrice, found := productPrices[itemCode] // Map lookup

	if !found {
		if strings.HasSuffix(itemCode, "_SALE") {
			originalItemCode := strings.TrimSuffix(itemCode, "_SALE") // Remove sale suffix
			basePrice, found := productPrices[originalItemCode]
			if found {
				salePrice := basePrice * 0.80 // Apply 20% discount
				fmt.Printf(" - Item %s (Sale! Original: $%.2f, Sale Price: $%.2f)\n",
					originalItemCode, basePrice, salePrice)
				return salePrice, true
			}
		}
		fmt.Printf(" - Item: %s (Product not found)\n", itemCode)
		return 0.0, false
	}
	fmt.Printf(" - Item %s (Sale! Price: $%.2f)\n", itemCode, basePrice)
	return basePrice, true
}

func main() {

	fmt.Println("---------- Simple Sales Order Processor ----------")

	orderItems := []string{
		"TSHIRT", "HUG_SALE", "HAT", "BOOK",
	}

	var subtotal float64
	fmt.Println("---------- Processing Order Items")

	for _, item := range orderItems { // Process every item
		price, found := calculateItemPrice(item)
		if found {
			subtotal += price
		}
	}
	fmt.Printf("Subtotal Price: %.2f\n", subtotal)

}
