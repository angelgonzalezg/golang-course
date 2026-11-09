# Here be dragons: reflect, unsafe, and cgo

These advanced topics follow the foundations of types, pointers, interfaces,
errors, and testing. Use each technique for a concrete need and keep its boundary
small enough to document and test.

| Topic | Status |
| --- | --- |
| Reflection | [Types, values, tags, runtime edits, construction, calls, and equality](01-reflection/README.md) |
| Unsafe | [Layout, pointer arithmetic, bounded views, and string ownership](02-unsafe/README.md) |
| Cgo | [Optional independent module: calls, type conversions, borrowed memory, and owned copies](03-cgo/README.md) |

Start with reflection, then study unsafe's memory representation and ownership
contracts. The optional cgo lab addresses the boundary between Go and C. It has
its own `go.mod` and requires a C compiler; run its commands separately from the
root module's checks.
