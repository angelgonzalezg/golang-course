# Reflection: types, values, and runtime structure

Start with [structs](../../03-composite-types/06-structs/main.go),
[pointers](../../06-pointers/01-pointers/main.go),
[interfaces](../../07-types-methods-and-interfaces/03-interfaces/main.go),
[generics](../../08-generics/01-generics/main.go), and
[errors](../../09-errors/01-error-handling/main.go).
The tests build on [chapter 15](../../15-testing/README.md).

## Run and read

From the repository root:

```sh
go run ./chapters/16-reflect-unsafe-and-cgo/01-reflection
go test -v ./chapters/16-reflect-unsafe-and-cgo/01-reflection
go vet ./chapters/16-reflect-unsafe-and-cgo/01-reflection
```

Read the numbered demonstrations in [main.go](main.go), then the two small
helpers in [structs.go](structs.go) and their [tests](structs_test.go).
The walkthrough uses fixed types and names so its results are easy to trace.
A library doing runtime inspection could receive those choices from callers.

## Choose the amount of runtime knowledge you need

| Situation | Start with | Why |
| --- | --- | --- |
| Model and fields are known | Direct field access or a typed function | The compiler checks names and types. |
| Several types share behavior | An interface | Call the required methods through a declared contract. |
| One algorithm works across a family of types | Generics with suitable constraints | Preserve type relationships at compile time. |
| A small set of dynamic cases is known | A type switch or assertion | State those cases explicitly. |
| Arbitrary runtime struct metadata must be examined | Reflection | Inspect field definitions, tags, and kinds without knowing each model. |

`describeStruct` demonstrates the last case. `setStringField` adds a bounded
runtime editing operation. For the known `Profile.Name`, direct assignment in
section 5 is clearer. Reflection introduces runtime checks and may add lookup,
dispatch, or allocation costs. Measure relevant workloads before optimizing;
see the [benchmark lesson](../../15-testing/05-benchmarks-and-races/benchmark_test.go).

## 1–3. Types, nil, and settable values

`DisplayName` and `string` have different types and the same string kind.
`TypeOf` examines the dynamic type stored in an interface. `TypeFor[T]` asks
for a type directly, including an interface type; it was added in Go 1.22 and
fits this course's Go 1.27 requirement.

A nil interface produces a nil `Type` and an invalid `Value`. An interface
holding `(*Profile)(nil)` has a concrete pointer type. Check validity and the
appropriate kind before using operations such as `IsNil`, `Elem`, or `NumField`.
The pointer's nil element is also invalid. `Kind` itself can report `Invalid`.

`ValueOf(profile)` represents a struct copy. `ValueOf(&profile).Elem()` can
reach the caller's original. An unexported field can be addressable while
`CanSet` and `CanInterface` remain false. The example checks these properties
and leaves the private value alone. The
[laws of reflection](https://go.dev/blog/laws-of-reflection) explain these
relationships; individual operations have additional requirements in the
[`reflect` API](https://pkg.go.dev/reflect).

## 4–5. Inspect metadata, then validate an edit

`describeStruct` accepts a struct or one pointer to it, including a typed nil
pointer. It reports direct fields in declaration order, their types and export
status, and the custom `display` tag. It never reads field values or traverses
embedded objects. Other input shapes return `ErrExpectedStruct` and no result.

`Lookup` distinguishes `display:""` on `Email` from an absent key on `Age`.
The tag is metadata: this example decides how to display it. Tags do not enforce
access, rename fields, or add validation by themselves. See the language rules
for [struct fields and tags](https://go.dev/ref/spec#Struct_types).

`setStringField` requires a non-nil pointer to a struct. It selects a direct
field by its exact Go name, requires `CanSet`, and checks string kind before
calling `SetString`. Defined string fields such as `DisplayName` are supported;
the operation preserves their type. Empty replacement text is allowed.

| Rejected input | Error cause |
| --- | --- |
| Nil, struct value, pointer to another kind, or extra pointer level | `ErrInvalidTarget` |
| Missing, empty, or promoted field name | `ErrFieldNotFound` |
| Unexported field | `ErrReadOnlyField` |
| Exported field with another kind | `ErrNotString` |

Every check precedes the write. The tests verify that rejection preserves the
original data, including when an embedded pointer is nil. Field errors retain
their cause for `errors.Is`. The helper updates one field; it does not interpret
tags, coerce input strings to other types, validate domain rules, or provide
synchronization for shared data.

## 6–7. References and runtime construction

The reflected slice descriptor cannot itself be replaced, but its element is
settable and still belongs to the original backing array. A map lookup returns
a value that cannot be assigned directly; `SetMapIndex` changes the map entry.
The demonstrations use a non-nil map and matching key/value types. A missing
lookup returns an invalid `Value`.

`New`, `SliceOf`, `MakeSlice`, and `Append` create values from runtime type
descriptors. `Interface` lets the constructed pointer reach `setStringField` as
an ordinary Go value. With known types, use `new(Profile)` or a slice literal.
Reflection preserves the usual ownership and synchronization responsibilities;
it does not clone referenced data automatically.

## 8–9. Calls and equality

`Call` demonstrates a function with a known matching signature. A dispatcher
handling arbitrary functions must check callable values and assignable
arguments; invoking user code can still panic. A direct function call or an
interface method fits the known operations shown alongside it.

`DeepEqual` distinguishes a nil slice from a non-nil empty slice.
[`slices.Equal`](https://pkg.go.dev/slices#Equal) treats them as equal by element
contents. Pick equality that matches the application's contract: comparable
values can use `==`; collections or domain objects may need dedicated rules.
`DeepEqual` does not supply a universal definition of application equality.

## Further practice

1. Add another model and inspect it without modifying `describeStruct`.
2. Add a typed setter for `Profile.Name` that enforces a nonblank name. Decide
   where validation belongs before allowing reflective edits to that model.
3. Extend lookup to promoted fields using explicit traversal and
   `FieldByIndexErr`. Specify how nil embedded pointers should behave first.
4. Benchmark repeated metadata lookup against a typed operation. If caching is
   justified, cache type metadata separately from values tied to an instance.
