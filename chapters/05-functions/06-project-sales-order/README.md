# Sales order processor project

Practice maps, structs, slices, conditionals, loops, strings, and functions that
return `(value, ok)`. Read the composite types and function lessons first.

## Run and test

Run these commands from the repository root:

```sh
go run ./chapters/05-functions/06-project-sales-order
go test ./chapters/05-functions/06-project-sales-order
```

## Prices and lookup rules

The catalog is passed explicitly to the pricing functions. Prices are integers
in cents: `2599` means `$25.99`. Pricing functions return data without printing
or modifying the catalog or input codes.

- Product codes are case sensitive and used exactly as supplied.
- An exact catalog match takes priority, even if the code ends in `_SALE`.
- Otherwise, one trailing `_SALE` is removed to look up the original product,
  and a 20% discount is applied.
- Empty codes, unavailable products, and negative catalog prices are skipped.
  A price of zero is valid.
- Each code occurrence represents one unit, so repeated codes are charged again.

The `PricedItem` result contains the requested code, original price, final price,
and applied discount. Presentation uses the discount field to decide whether to
display a sale label.

## Rounding policy

Round the **final discounted unit price** to the nearest whole cent, with half
cents rounded up. Round each unit before adding it to the subtotal.

| Original price | Discount | Exact final price in cents | Rounded final price |
| --- | --- | --- | --- |
| $25.99 | 20% | 2079.2 | $20.79 |
| $0.02 | 20% | 1.6 | $0.02 |
| $0.03 | 20% | 2.4 | $0.02 |
| $0.05 | 50% | 2.5 | $0.03 |

The `_SALE` rule always uses 20%. The `applyDiscount` helper accepts percentages
from 0 through 100, so its tests also cover half-cent ties using 50%.

The calculation splits the price into a quotient and a remainder before
multiplying. This keeps intermediate values within the range of `int` even
when the input is its maximum value. No floating-point conversion is needed
to calculate or display a price.

## Order results

`processOrder` returns an `OrderSummary` and an `ok` flag. Accepted items and
skipped codes each retain their relative input order. The printer lists the
accepted items first and then the skipped codes.

An empty order succeeds with a zero subtotal. If adding a price would overflow
`int`, processing returns `false` and an empty summary. Check `ok` before using
the result so a partial subtotal is never presented as a completed order.

The example demonstrates three orders:

| Order | Processed | Skipped | Subtotal |
| --- | --- | --- | --- |
| `TSHIRT`, `HUG_SALE`, `HAT`, `BOOK` | 4 | 0 | $73.99 |
| `BOOK_SALE`, `MISSING`, `HAT` | 2 | 1 | $38.79 |
| Empty | 0 | 0 | $0.00 |

## Further practice

Add explicit quantities and validate them before pricing. Use checked
multiplication for line totals and define whether rounding occurs per unit or
per line. After chapter 9, try returning errors that distinguish unavailable
products, invalid prices, and subtotal overflow.
