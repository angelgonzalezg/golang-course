# JSON, field semantics, and validation

Practice typed JSON, optional fields, number precision, custom representations,
and record streams. This chapter 13 lesson builds on structs, pointers, interfaces,
errors, I/O, and time.

The book and earlier lessons use `encoding/json`, now called v1. Go 1.27 adds
`encoding/json/v2` and `encoding/json/jsontext`. This lesson uses v2 for new code
and explicitly aliases v1 for compatibility comparisons. The new APIs are in the
standard library under the course's existing Go 1.27 requirement.

## Run and validate

Run from the repository root:

```sh
go run ./chapters/13-standard-library/03-json
go test ./chapters/13-standard-library/03-json
go vet ./chapters/13-standard-library/03-json
```

All input is constructed in memory. The example needs no files or network access.
It prints successful encodings, expected rejections, a large-number precision
comparison, and a two-record stream.

## Read the example

- [main.go](main.go) runs ten sections with comments about the choices in use.
- [model.go](model.go) separates the input request from a validated `Task`, applies
  defaults, owns copied tags, and defines custom JSON methods for `TaskState`.
- [decode.go](decode.go) accepts one bounded request and returns no partial task.
- [model_test.go](model_test.go) checks ownership, defaults, the response shape,
  and custom enum behavior with both JSON APIs.
- [decode_test.go](decode_test.go) checks byte boundaries, field types, unknown
  and duplicate names, framing, malformed input, and reader ownership.

## Choose an API for the data you have

| Need | Go 1.27 v2 API | v1 counterpart |
| --- | --- | --- |
| Encode to bytes | `Marshal` | `Marshal` |
| Decode one document from bytes | `Unmarshal` | `Unmarshal` |
| Write one value to a writer | `MarshalWrite` | `Encoder.Encode`, which also appends a newline |
| Read exactly one document through EOF | `UnmarshalRead` | A `Decoder.Decode` call followed by a check that the next decode returns EOF |
| Produce a stream of values | `MarshalEncode` with a shared `jsontext.Encoder` | Repeated `Encoder.Encode` calls |
| Consume a stream of values | `UnmarshalDecode` with a shared `jsontext.Decoder` | Repeated `Decoder.Decode` calls |
| Delay decoding a payload | `jsontext.Value` | `json.RawMessage` |

Decode functions require a non-nil pointer to the destination. Marshal accepts
both values and pointers; v2 recommends a non-nil pointer for addressability and
performance. The example passes addresses of existing values without transferring
ownership. This is separate from using pointer fields to represent optional data.

Structs make a known schema explicit. Use `map[string]any` when the structure is
truly dynamic, accounting for type checks and number representation. Tags rename
exported fields; `json:"-"` excludes a field, and ordinary unexported fields are
not serialized. `time.Time` has a timestamp representation. A `[]byte` field
represents binary data and encodes as a base64 string, not a nested JSON document.

## Compare v1 and v2 defaults deliberately

| Behavior | v1 | v2 |
| --- | --- | --- |
| Duplicate object names | Accepted; later values replace or merge according to the Go type. | Rejected by default. |
| Matching object names to struct fields | Also accepts case-insensitive matches. | Case-sensitive by default. |
| Unknown object names | Ignored unless `DisallowUnknownFields` is enabled on a decoder. | Ignored unless `RejectUnknownMembers(true)` is supplied. |
| Invalid UTF-8 in strings | Replaced with the Unicode replacement character. | Rejected by default. |
| Nil ordinary slice/map without omission tags | Encoded as `null`. | Encoded as `[]` / `{}`. |
| `false` or numeric zero with `omitempty` | Omitted. | Included, since these are not empty JSON values. |
| Zero Go value with `omitzero` | Omitted. | Omitted. |
| Map output order | Deterministic by default. | Unspecified by default; request `Deterministic(true)` when needed. |

JSON object member order has no semantic meaning. Tests inspect the response's
members independently of their output order. Review wire contracts when changing
an existing import; switching packages can change an API's observable behavior.

`omitempty` in v2 omits values represented as JSON null, an empty string, empty
array, or empty object. `omitzero`, available since Go 1.24, uses the Go zero
value or a type's `IsZero` method in both APIs. The example shows why zero
`time.Time` is still emitted with `omitempty`, while `omitzero` omits it.

`time.Duration` also needs an explicit representation policy in new v2 code.
For configuration, a string such as `250ms` can be parsed with the previous
lesson's duration approach; an integer field should identify its intended unit.
Do not assume a duration will have the same default representation as in v1.

## Optional values, null, and defaults

The request's `notifications` field defaults to true:

| Input | Decoded request pointer | Task value |
| --- | --- | --- |
| Member absent | nil | true |
| `"notifications": null` | nil | true |
| `"notifications": false` | Pointer to false | false |
| `"notifications": true` | Pointer to true | true |

Use a value field when absent and zero have the same meaning. Use a pointer when
the pointed-to value must be distinguished from absence, as in explicit false
versus the default here. A pointer alone does not distinguish absent from null
when decoding into fresh storage. A PATCH contract with three separate meanings
would need explicit presence tracking, such as a custom wrapper or member map.
These pointer choices express semantics; they do not imply a memory saving.

The output `Task` stores an ordinary bool and always emits `notifications`, so
false survives serialization. It normalizes missing/null tags to an empty slice
and copies the request's tag storage. The ownership copy is useful if the
converter is called directly with caller-owned data; strings need no deeper copy.

## Decode a complete request, then validate domain rules

`readTask` implements this exercise's input contract:

1. Read at most 4,097 bytes to enforce a 4,096-byte document limit, including
   whitespace. The extra byte detects an oversized source instead of accepting
   a truncated prefix. Larger sources are not fully drained.
2. Decode exactly one JSON value from the bounded bytes. Extra values, trailing
   junk, malformed input, duplicate names, and invalid UTF-8 fail.
3. Reject unknown members, including case mismatches. This option is explicit;
   the default v2 decoder still ignores unknown names.
4. Require an object. A local `*taskRequest` lets null decode to nil while a
   valid object allocates a request. Arrays and scalars fail the target type.
5. Trim the title, reject blank titles, apply the notification default, copy
   tags, and initialize the state to pending.

The function borrows its reader and returns a zero task on every failure.
An I/O error after a complete-looking JSON prefix still fails the request.
No partially decoded destination is exposed; ordinary decoding can mutate a
destination before reporting an error. Fresh local storage also avoids merging
new input into fields left over from a previous request.

Parsing into a Go type does not enforce application requirements such as a
nonblank title. `TaskState` is not accepted in the create request; the application
chooses its initial state. This example's validation policy is intentionally
small and explicit rather than a general schema engine.

`errors.As` exposes `*json.SemanticError` for representation failures and
`*jsontext.SyntacticError` for syntax failures. Their JSON pointers locate the
affected data. `errors.Is` finds `json.ErrUnknownName` and the exercise's sentinel
errors through wrapping. Avoid branching on the wording of error messages.

## Preserve numeric precision

Both APIs use `float64` when decoding a number into a fresh `any` value. The
integer `9007199254740993` becomes `9007199254740992` that way. Prefer a concrete
`int64` field for a known integer schema; its type also makes overflow and
incompatible values errors. A `json:",string"` option quotes a numeric field
for clients that cannot preserve large JSON integers. It changes the wire
contract, so both producers and consumers must agree on the quoted form.

For existing v1 code with dynamic input, `Decoder.UseNumber` retains number
text as `json.Number`. Its `Int64` conversion still returns an error for values
that do not fit, including fractions and overflow. v2 does not have that decoder
method; choose concrete types, raw values, or an explicit custom unmarshal policy.

## Custom methods and deferred payloads

`TaskState` is a numeric Go enum encoded as `"pending"` or `"done"`. Its value
receiver `MarshalJSON` reads the enum, while pointer receiver `UnmarshalJSON`
updates it only after a temporary value passes validation. Invalid strings and
null are rejected, preserving the prior state. This guarantee concerns that
receiver, not other fields of an enclosing object. Tests verify compatibility
with both v1 and v2 callers.

The envelope example reads a discriminator before choosing the payload's Go
type. `jsontext.Value` retains raw JSON for that second decoding step. The raw
value still needs domain validation after a type is selected. An ordinary
`[]byte` field would instead describe base64 binary data.

## Single documents and streams have different framing

`MarshalWrite` writes one value without an automatic newline. The shared
`jsontext.Encoder` used with `MarshalEncode` terminates each top-level record
with a newline. `UnmarshalDecode` reads the next record and returns `io.EOF`
only when the stream is exhausted; other errors must be handled. The decoder
accepts whitespace-separated values, not a strict one-line-per-record protocol.
Use fresh destination storage for each record. `More` in the v1 decoder is for
array/object contents, not a replacement for top-level EOF handling.

The stream in `main` is produced locally and finite. Incremental decoding avoids
collecting all decoded records, but one large record can still require substantial
memory. External input needs a size/framing policy, such as the small complete
document policy in `readTask`. Already processed records or bytes already written
cannot be rolled back on a later failure. Reader/writer ownership and any
buffered writer's final flush remain the caller's responsibilities.

References: [JSON tutorial](https://go.dev/doc/tutorial/json),
[`encoding/json/v2`](https://pkg.go.dev/encoding/json/v2),
[`encoding/json`](https://pkg.go.dev/encoding/json),
[`jsontext`](https://pkg.go.dev/encoding/json/jsontext),
and [v2 migration guide](https://go.dev/doc/jsonv2-migration).
