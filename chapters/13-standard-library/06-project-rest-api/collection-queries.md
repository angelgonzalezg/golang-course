# Collection queries: filter, order, and paginate

Read the [REST project](README.md) and run its ordinary CRUD demonstration first.
This extension bounds collection responses and lets a client navigate selected
results. The choices below are this API's query contract.

## Parameters

| Parameter | Default | Accepted values |
| --- | --- | --- |
| `limit` | `20` | Integer from 1 to 100; maximum contacts in this response. |
| `offset` | `0` | Nonnegative integer; matching contacts to skip. Must fit Go's `int`. |
| `name_contains` | Empty; matches all names. | Trimmed text of at most 80 valid UTF-8 runes. |
| `sort` | `id` | `id`, `-id`, `name`, or `-name`; the minus sign selects descending order. |

Numbers use ordinary decimal notation without leading zeros or a plus sign.
Unknown parameters, repeated parameters, invalid values, malformed URL escapes,
and unescaped semicolons return 400 Problem Details. An explicit empty `sort`,
`limit`, or `offset` is invalid. A blank name filter is valid and matches all.

The name filter compares Unicode-lowercased text and matches any substring.
It does not remove accents or perform Unicode normalization. Name sorting uses
the same lowercased strings and Go's lexicographic string order, rather than a
locale-specific alphabet. Equal name keys, such as Ana and ana, are ordered by
increasing ID in both sort directions. Contact-name uniqueness remains case
sensitive, as in the original exercise.

The steps happen in this order:

1. Take one independent snapshot of the store.
2. Filter all contacts and count the matches.
3. Order those matches, using an ID tie-breaker for equal name keys.
4. Skip `offset` matches and return at most `limit` of the remainder.

Filtering or sorting only after selecting a page would produce different and
misleading results. `pagination.total` counts matches before skipping or limiting.

## Try it

Start a fresh server with `-serve` as described in the [main guide](README.md).
Create a few contacts, then request a filtered page:

```sh
curl -sS -X POST http://127.0.0.1:8080/contacts \
  -H 'Content-Type: application/json' -d '{"name":"Ana"}'
curl -sS -X POST http://127.0.0.1:8080/contacts \
  -H 'Content-Type: application/json' -d '{"name":"Ana Torres"}'
curl -sS -X POST http://127.0.0.1:8080/contacts \
  -H 'Content-Type: application/json' -d '{"name":"Bob"}'
curl -sS --get http://127.0.0.1:8080/contacts \
  --data-urlencode 'name_contains=ana' \
  --data-urlencode 'sort=name' --data-urlencode 'limit=1'
```

The response contains the first matching contact, pagination metadata, and links:

```json
{
  "contacts": [
    {"id": 1, "name": "Ana", "links": {"self": "/contacts/1", "collection": "/contacts"}}
  ],
  "links": {
    "self": "/contacts?limit=1&name_contains=ana&offset=0&sort=name",
    "first": "/contacts?limit=1&name_contains=ana&offset=0&sort=name",
    "next": "/contacts?limit=1&name_contains=ana&offset=1&sort=name"
  },
  "pagination": {"limit": 1, "offset": 0, "total": 2}
}
```

Follow `links.next` to retrieve Ana Torres. That page supplies `links.prev` and
omits `links.next`. The links preserve the normalized filter, limit, and order;
clients do not need to recalculate them. `links.first` always goes to offset 0.
On an arbitrary nonzero offset, `prev` subtracts one limit and stops at zero.
If the requested offset is far beyond the results, that previous page can also
be empty; `first` returns directly to the beginning.

No matches, an empty store, or an offset at or beyond the match count produce
200 with `contacts: []`, the requested limit and offset, and the current total.
They do not produce 404. The requested offset is preserved in metadata even
when no records are returned.

## Read the Go code

- [contacts/query.go](contacts/query.go) defines transport-independent query and
  page values, validation, filtering, ordering, and page selection. `Search`
  requires a valid limit; the HTTP adapter supplies the default of 20. The basic
  `List` operation remains available for the original snapshot exercises.
- [httpapi/query.go](httpapi/query.go) parses URL input, applies HTTP defaults,
  defines pagination metadata, and builds relative navigation links. It calls
  `url.ParseQuery` explicitly so parse errors are reported. `url.Values.Encode`
  escapes values, including spaces, plus signs, ampersands, and Unicode.
- [httpapi/handler.go](httpapi/handler.go) sends the request context and query to
  `Search`, converts its values to HTTP representations, and returns one page.
- [Store tests](contacts/query_test.go) check order, Unicode, tie-breaking,
  snapshots, bounds, cancellation, and concurrent writes. [HTTP tests](httpapi/query_test.go)
  follow navigation links, check defaults and empty pages, round-trip encoded
  filters, and reject invalid query strings.

The search works on an owned snapshot after the store lock is released. It
checks context cancellation during filtering and after sorting; standard sorting
itself is not preempted by context. The returned slice copies only the page's
contacts, so keeping a small page does not retain the whole snapshot array.

## Limits to explain

An individual request has a consistent count and page because both come from
the same snapshot. Each later request reads current data. Adding, deleting, or
renaming contacts between requests can move rows, causing skipped or repeated
contacts across pages. A deterministic order makes paging predictable while
the data stays unchanged; it does not freeze a browsing session.

The response item count is bounded. This small in-memory implementation still
copies, scans, and sorts the collection; it does not bound total search work.
A database implementation would apply filtering, ordering, and limits in its
query and choose appropriate indexes. Cursor pagination is a possible later
exercise when a workload needs different traversal behavior.

Use the [project's validation commands](README.md#run-the-demonstration) to run
all query tests and the finite demo, which follows two filtered pages itself.
Before continuing, predict which page changes after deleting an earlier match,
and explain why filtering must happen before counting and pagination.

References: [`url.ParseQuery`](https://pkg.go.dev/net/url#ParseQuery) and
[`url.Values.Encode`](https://pkg.go.dev/net/url#Values.Encode).
