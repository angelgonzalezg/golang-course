# Strings, bytes, and runes

Read this lesson after [slices](../02-slices/main.go) and
[slice internals](../03-slice-internals/main.go), before
[maps](../05-maps/main.go). It expands the rune and byte introduction in
[values](../../02-declarations/01-values/main.go).

The walkthrough uses small functions and loops to display the examples. Chapters
4 and 5 explain their syntax in more detail. The `strconv` examples check errors
immediately; chapter 9 develops the full error-handling model.

## Run and test

Run these commands from the repository root:

```sh
go run ./chapters/03-composite-types/04-strings-bytes-and-runes
go test ./chapters/03-composite-types/04-strings-bytes-and-runes
```

## Choose the unit you need

| Operation | Unit or behavior |
| --- | --- |
| `len(text)` | Number of bytes. |
| `text[index]` | One byte; `byte` is an alias for `uint8`. |
| `text[low:high]` | A substring between byte offsets; it can split a UTF-8 encoding. |
| `range text` | Decodes UTF-8 into runes and reports their starting byte offsets. |
| `[]rune(text)` | Decodes code points into a slice; `rune` is an alias for `int32`. |
| `utf8.RuneCountInString(text)` | Counts decoded runes, including replacements for invalid bytes. |
| `utf8.ValidString(text)` | Checks whether the bytes form valid UTF-8. |
| `strings.Index(text, part)` | Reports a byte offset, or `-1` if no match exists. |

The sample `Goé🙂` contains 8 bytes and 4 runes. Their starting byte offsets are
0, 1, 2, and 4. `text[:3]` includes only the first byte of `é`, leaving invalid
UTF-8; `text[:4]` includes the whole code point.

A string is an immutable byte sequence, with `""` as its zero value. It can
contain binary data, including zero bytes and invalid UTF-8. UTF-8 is the text
encoding used in these examples, rather than a validity guarantee on every
string value.

## Conversions and editing

Use `[]byte(text)` for a mutable byte sequence, and `[]rune(text)` when editing
individual decoded code points. Converting the result back with `string(...)`
creates a string whose contents are independent of later changes to the slice.
Changing either slice leaves the original string unchanged.

Converting an integer to a string encodes that code point, rather than formatting
decimal digits. `string(rune(65))` produces `"A"`; `strconv.Itoa(65)` produces
`"65"`. `strconv.Atoi` parses decimal text into `int`, while `ParseInt` and
`FormatInt` accept an explicit base. Check parsing errors before using results.
`Atoi` does not trim whitespace: decide whether to apply `strings.TrimSpace`
according to the input format you intend to accept.

## Unicode boundaries and invalid data

The text `é` can be encoded as U+00E9 or as U+0065 followed by U+0301. These
representations can look identical but have different bytes and rune counts.
Go does not normalize them automatically, and `strings.EqualFold` handles simple
Unicode case folding without performing Unicode normalization.

A visible character can contain several code points, as with the combining
accent or the sun plus variation selector `☀️`. Counting runes does not count
grapheme clusters or screen columns. A rune prefix can remove an accent or
variation selector while leaving valid UTF-8. This lesson does not implement
grapheme segmentation or normalization.

For invalid UTF-8, `range` and `[]rune` decode each invalid byte as U+FFFD,
the replacement character. Converting that rune slice back to a string changes
the original bytes. Preserve raw bytes when exact round trips matter.
`utf8.RuneCountInString` is not validation: it also counts those replacements.

`utf8.DecodeRuneInString` reports U+FFFD with width 1 for an invalid first byte.
A correctly encoded U+FFFD has width 3, so the rune value alone cannot identify
invalid input. Empty input returns U+FFFD with width 0.

## Small exercises and their contracts

`prefixRunes(text, limit)` returns at most `limit` code points with `(result,
true)`. A limit of zero returns an empty string, and a limit larger than the rune
count returns all the text. Negative limits and invalid UTF-8 return `("",
false)`, even if the requested prefix would precede the invalid byte. The helper
validates the input, then uses a rune slice for clarity; it decodes the full
string rather than reading only the prefix. It preserves code point boundaries.

`splitAssignment(line)` splits at the first `=` and trims Unicode whitespace
around the key and value. A missing separator or blank key returns `("", "",
false)`. Empty values are valid, and `token=a=b` preserves `a=b` as the value.
This exercise defines no quoting, escaping, or comment syntax.

The walkthrough also compares `strings.Fields`, which groups Unicode whitespace
and omits empty fields, with `strings.Split`, which uses an exact separator and
preserves empty fields in the demonstrated comma-separated input. It introduces
`Join`, `ReplaceAll`, `HasPrefix`, and `Contains` for ordinary text operations.

## Further practice

1. Implement a rune suffix with the same validation rules as `prefixRunes`.
2. Accept a list of assignments and report duplicate keys after the maps lesson.
3. Build a text report using `strings.Builder` after the methods lesson.
4. Add `strconv.ParseBool` and `strconv.ParseFloat` examples with valid and
   invalid input, checking every returned error.

See the official [strings and Unicode explanation](https://go.dev/blog/strings),
[UTF-8 package](https://pkg.go.dev/unicode/utf8),
[strings package](https://pkg.go.dev/strings), and
[strconv package](https://pkg.go.dev/strconv) for the language model and APIs.
