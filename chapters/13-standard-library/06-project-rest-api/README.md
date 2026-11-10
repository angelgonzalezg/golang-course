# Contact resources over HTTP

Build a small REST-style contacts API with `net/http` and the standard library.
This project extends the [HTTP lesson](../04-http/README.md) from computing a
greeting to creating and managing resources with stable URLs.

Read methods, interfaces, errors, packages, concurrency, JSON, HTTP, and logging
first. Revisit [context](../../14-context/01-context/README.md) and
[testing](../../15-testing/README.md) to understand propagation and the tests.
The project uses the course's Go 1.27 toolchain and `encoding/json/v2`.

## Run the demonstration

From the repository root:

```sh
go run ./chapters/13-standard-library/06-project-rest-api
go test -count=1 -timeout=30s ./chapters/13-standard-library/06-project-rest-api/...
go test -race -count=3 -cpu=1,2 -timeout=30s ./chapters/13-standard-library/06-project-rest-api/...
go vet ./chapters/13-standard-library/06-project-rest-api/...
```

The default command binds an available loopback port, runs its own requests,
then shuts down and joins the server goroutine. It demonstrates creation,
listing, retrieval, a duplicate conflict, partial updates with PATCH, full
replacement with PUT, repeated requests, deletion, and following filtered
collection pages. It also demonstrates 304 revalidation and rejection of a
stale write with 412. It requires no external service and exits by itself.

For interactive requests, keep a server running in one terminal:

```sh
go run ./chapters/13-standard-library/06-project-rest-api -serve
# Optional: choose another address, or :0 for an available port.
go run ./chapters/13-standard-library/06-project-rest-api -serve -addr 127.0.0.1:8081
```

Choose one server command. Stop it with Ctrl+C. Each process starts with an
empty store; restarting loses its contacts. The default address is loopback.

In another terminal, using the default port:

```sh
curl -i http://127.0.0.1:8080/
curl -i -X POST http://127.0.0.1:8080/contacts \
  -H 'Content-Type: application/json' \
  -d '{"name":"Ana","email":"ana@example.com","phone":"555-0100"}'
curl -i http://127.0.0.1:8080/contacts
# Use the Location returned by POST. A fresh store assigns /contacts/1 first.
curl -i http://127.0.0.1:8080/contacts/1
# PATCH changes the email and keeps the current name and phone.
curl -i -X PATCH http://127.0.0.1:8080/contacts/1 \
  -H 'Content-Type: application/merge-patch+json' \
  -d '{"email":"ana.torres@example.com"}'
# null removes an optional value; name and email remain as they were.
curl -i -X PATCH http://127.0.0.1:8080/contacts/1 \
  -H 'Content-Type: application/merge-patch+json' -d '{"phone":null}'
# PUT replaces every editable field, clearing omitted email and phone.
curl -i -X PUT http://127.0.0.1:8080/contacts/1 \
  -H 'Content-Type: application/json' -d '{"name":"Ana Torres"}'
curl -i -X DELETE http://127.0.0.1:8080/contacts/1
curl -i -X DELETE http://127.0.0.1:8080/contacts/1
```

## Read the project in stages

1. [contacts/model.go](contacts/model.go): ordinary Go values, editable input,
   selected changes, normalization, limits, and errors. No HTTP or JSON types
   are needed here. In `Changes`, nil means keep; a pointer supplies a new value.
2. [contacts/store.go](contacts/store.go): operations preserve IDs and the unique
   name index. A mutex protects all related state together, since handlers run
   concurrently. Its tests check snapshots, rejected writes, cancellation, and
   concurrent operations. `Patch` reads, validates, and writes under one lock
   so requests updating different fields preserve both changes.
   Then read [collection queries](collection-queries.md) for a bounded search
   over independent snapshots.
   [Conditional requests](conditional-requests.md) add atomic version checks;
   the [OpenAPI guide](openapi.md) describes the served contract and validation.
3. [httpapi/handler.go](httpapi/handler.go): a consumer-defined interface connects
   HTTP to those operations. Separate representation structs define JSON fields
   and navigation links without changing the contact model.
4. [httpapi/json.go](httpapi/json.go): bounded decoding, member presence,
   domain-error mapping, and consistent problem responses. The
   [PATCH tests](httpapi/patch_test.go) compare PUT and PATCH, removals, rejected
   requests, and capability discovery. [Other HTTP tests](httpapi/handler_test.go)
   cover the original contract, concurrent requests, and HEAD on a real connection.
5. [demo.go](demo.go), [server.go](server.go), and [main.go](main.go): client body
   cleanup, request context, server budgets, the finite demo, and interactive
   shutdown. [Server tests](server_test.go) check cancellation and an active
   request finishing during shutdown.

The earlier [contact book](../../06-pointers/04-project-contact-list/README.md)
remains the pointer practice. This separate library adds the errors,
synchronization, and context contract needed at a network boundary. A future
gRPC adapter can use `contacts` without importing `httpapi` or duplicating its
rules. `NewHandler` receives its store; it does not construct global state.

## Resources and methods

| Request | Meaning | Successful response |
| --- | --- | --- |
| `GET /` | Discover the collection and contract links. | `200`, JSON links. |
| `GET /openapi.json` | Retrieve the embedded OpenAPI 3.1.1 contract. | `200`, JSON document. |
| `GET /contacts` | Filter, order, and paginate contacts; defaults to 20 in increasing ID order. | `200`, JSON collection, including `[]` when empty. |
| `GET /contacts/{id}` | Retrieve a contact. | `200`, JSON contact. |
| `POST /contacts` | Create a contact; the server assigns its ID. | `201`, JSON contact and relative `Location` header. |
| `PUT /contacts/{id}` | Replace all editable fields of an existing contact. | `200`, JSON contact with the same ID. |
| `PATCH /contacts/{id}` | Apply selected changes to an existing contact. | `200`, JSON contact with the same ID. |
| `DELETE /contacts/{id}` | Remove a contact. | `204`, no response body. |
| `OPTIONS /contacts/{id}` | Discover this route's supported methods and patch format. | `204`, `Allow` and `Accept-Patch` headers, no body. |

GET also supports HEAD and entity-tag preconditions; matching If-None-Match
returns 304 without a body. PUT/PATCH/DELETE optionally accept version conditions
and return 412 if a condition fails. The [conditional request guide](conditional-requests.md)
explains precedence, cache policy, and the successful PUT validator restriction.
The real HTTP server suppresses every HEAD body. IDs are
positive `int64` values written as decimal digits without leading zeros.
Successful representations use `application/json`. A creation response includes:

```json
{
  "id": 1,
  "name": "Ana",
  "email": "ana@example.com",
  "phone": "555-0100",
  "links": {"self": "/contacts/1", "collection": "/contacts"}
}
```

Email and phone are optional. Empty or whitespace-only input is normalized to
missing optional data, stored as an empty string, and omitted from JSON with
`omitempty`. This representation rule applies to every route. Clients should
allow those two members to be absent. ID, name, and links remain present.

A collection contains `contacts`, `pagination` with limit/offset/total, and
navigation links: `self`, `first`, and, when applicable, `next` and `prev`.
Clients can follow returned links and `Location`; they do not need to guess
assigned IDs. URLs identify resources, HTTP methods express operations, and JSON represents their current
state. Each request supplies its own input; the API keeps resource data rather
than a per-client conversation. Successful reads use `Cache-Control: private,
no-cache` for revalidation; errors and mutation responses use `no-store`.

REST is an architectural style with constraints, not a synonym for JSON or CRUD.
This project practices resource identification, representations, links, method
semantics, conditional requests, and an explicit cache policy.

### Collection queries

Use `limit`, `offset`, `name_contains`, and `sort` to select a page. For example,
`GET /contacts?limit=2&name_contains=ana&sort=name` filters names, sorts all
matches, and returns the first two. The maximum limit is 100. Empty results
return 200 with `[]`; invalid or repeated query parameters return 400.

The [collection query guide](collection-queries.md) explains the full contract,
encoded URLs, navigation links, deterministic ordering, concurrent changes
between pages, tests, and the in-memory implementation's costs.

### PUT versus PATCH

PUT replaces `name`, `email`, and `phone`; it keeps the server-owned ID and
links. Omitting email or phone clears them. It does not merge a partial update
and does not create a missing contact. That update-only policy is this API's
contract; HTTP also permits APIs that create resources with PUT.

PATCH accepts a JSON Merge Patch document using
`Content-Type: application/merge-patch+json`. An omitted member is unchanged;
a supplied string updates that member; null requests removal of that member.
The API still validates the resulting contact and normalizes text. Name is
required, so removing it is rejected. Removing email or phone clears the stored
text and removes that optional member from the returned JSON. Empty optional
text is also normalized to missing data by the rule above.

Compare these requests **starting independently from the original contact
above**, which has both email and phone:

| Body | PUT with `application/json` | PATCH with `application/merge-patch+json` |
| --- | --- | --- |
| `{"name":"Ana Torres"}` | Changes name and clears email and phone. | Changes name and keeps email and phone. |
| `{"email":"new@example.com"}` | Rejected: the required name is missing. | Changes email and keeps name and phone. |
| `{"phone":null}` | Rejected: the required name is missing. | Removes phone and keeps name and email. |
| `{}` | Rejected: the required name is missing. | No changes; returns the current contact. |
| `{"name":null,"email":"new@example.com"}` | Rejected: name cannot be blank. | Rejected: name cannot be removed; email is also unchanged. |

Both update routes require an existing contact and return 404 rather than
creating one. The ID and links are read-only. Only name, email, and phone may
appear in the patch, with string or null values. Other JSON Merge Patch documents
can describe arrays or complete value replacement, but those results do not
match this contact schema and are rejected. An array of `op`/`path` operations
uses the separate JSON Patch format, which this API does not support.

`OPTIONS /contacts/1` describes route capabilities without checking whether
contact 1 exists. `Allow` includes PATCH, and `Accept-Patch` advertises
`application/merge-patch+json`. PATCH responses, including unsupported-media-type
errors, also include `Accept-Patch`.

### Idempotency and atomicity

PUT and DELETE are idempotent: repeating the same request has the same intended
effect on resource state as sending it once. That does not require identical
statuses: the first DELETE returns 204 and the next returns 404, with the contact
absent in both cases. POST is not generally idempotent. Here a duplicate name
produces 409; clients still need a deliberate retry policy after an ambiguous
network failure. GET and HEAD do not request changes to resource state.

The patches accepted here only assign or remove fields, so repeating the same
patch is also idempotent. PATCH in general does not guarantee idempotency: other
patch formats could describe appending an item or incrementing a counter.

PATCH applies all requested changes atomically. If a new name conflicts or any
field fails validation, no field changes. The store reads the current contact,
builds and validates the proposed result, checks name uniqueness, and saves it
while holding one mutex. A GET followed by a separate Replace would leave a gap
where another request's change could be overwritten. The
[store tests](contacts/patch_test.go) check rejected multi-field patches and
concurrent edits to different fields of the same contact.

## Input and failures

POST and PUT accept one JSON object with only `name`, `email`, and `phone`.
The ID and links are server-owned and are rejected as input. Fields are strings:

- All three are trimmed and must contain valid UTF-8. Name must be nonblank and
  at most 80 runes; email and phone allow empty text and at most 254 and 40 runes.
  Email and phone are stored as text; format validation is a possible extension.
- Names are unique after trimming and case sensitive: `" Ana "` conflicts with
  `"Ana"`; `"ana"` is a different name in this exercise.
- Omitted optional fields and explicit JSON null become empty strings with v2.
  An omitted or null name becomes blank and fails validation. Root null, arrays,
  wrong types, extra documents, unknown members, and duplicate keys are rejected.
- The body limit is 4,096 bytes, including whitespace. It is enforced even
  without a known Content-Length. The rune limits are separate from this budget.
- The media type must be `application/json`; parameters are accepted. Compressed
  bodies are unsupported. PATCH uses `application/merge-patch+json` with the
  same byte and field limits, UTF-8 checks, duplicate-key rejection, and body
  read bound. Sending ordinary `application/json` to PATCH returns 415.
  Responses are JSON; this version does not negotiate another representation
  from `Accept`.

PATCH decoding must preserve member presence. In a `map[string]*string`, an
absent key, a present key with nil, and a present key with a string distinguish
omission, JSON null, and a new value. Only a `*string` struct field would merge
omission and null into nil. The adapter converts present null to an explicit
empty-value change; nil fields in the domain's `Changes` mean keep the stored
value. Domain values remain independent of JSON decoding rules.

| Failure | Status |
| --- | --- |
| Malformed JSON/query/condition header, invalid input, or body read failure | 400 |
| Missing resource or unknown route | 404 |
| Unsupported method on a known route; includes `Allow` | 405 |
| Duplicate name on creation, replacement, or partial update | 409 |
| Valid entity-tag precondition does not match current state | 412 |
| Accepted JSON body exceeds the byte limit | 413 |
| Missing/invalid/unsupported media type or content encoding | 415 |
| Unexpected store error or response encoding failure | 500 |
| Exhausted IDs or canceled/expired store operation | 503 |

Errors use `application/problem+json` with the RFC 9457 `about:blank` shape:

```json
{"type":"about:blank","title":"Not Found","status":404,"detail":"contact not found"}
```

The HTTP status is authoritative and matches `status`. Expected errors give
useful details; unexpected store errors are logged with `slog` and hidden from
clients. Method-specific patterns and path fallbacks keep 404/405 responses in
this format while preserving `PathValue`. The mux still performs its normal
path-cleaning redirects, for example for repeated slashes.

The adapter passes `r.Context()` to the store. This origin server chooses 503
when an operation cannot complete due to cancellation; 504 describes a gateway
timeout. A client that canceled its connection usually sees its own context
error instead of the attempted response. The store checks cancellation before
work and after acquiring the lock. Waiting for `sync.Mutex` is not cancelable,
so operations must remain short. Cancellation racing with a completed mutation
does not undo that mutation.

## Ownership and limits

Use `*contacts.Store`; copying it would copy a mutex and share its maps. A
zero-value store is usable. It returns independent contact values and list
snapshots: editing returned fields does not edit the store. Each mutation checks
its rules before changing either map, and IDs increase without being reused.

Server budgets and shutdown follow the [HTTP lesson](../04-http/README.md).
Shutdown uses a fresh three-second context, waits for active requests, and joins
`Serve`; if the budget expires it closes connections and returns the error.
Handlers do not start background work. JSON is encoded before committing a
success status, and 204 responses have no JSON body.

This is a small in-memory exercise. It has no persistence, authentication,
or deployment configuration. Without an If-Match condition, simultaneous writes
to the same field use the last successful write. Conditional writes detect stale
representations and reject them atomically.
PATCH preserves concurrent edits to unselected fields, but a PUT intentionally
replaces every editable field. The agreed REST blocks are complete: paged
queries, ETag/conditional requests, and a checked [OpenAPI contract](openapi.md).
Persistence and authentication remain later extensions. The supplementary
[Protobuf/gRPC lab](../../../extensions/01-protobuf-and-grpc/README.md) begins
with the contact contract and serialization, followed by a transport adapter
over the same contact operations.

References: [HTTP semantics, RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html),
[PATCH, RFC 5789](https://www.rfc-editor.org/rfc/rfc5789.html),
[JSON Merge Patch, RFC 7396](https://www.rfc-editor.org/rfc/rfc7396.html),
[Problem Details, RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html),
[Fielding's REST constraints](https://ics.uci.edu/~fielding/pubs/dissertation/rest_arch_style.htm),
[`net/http`](https://pkg.go.dev/net/http), and
[`encoding/json/v2`](https://pkg.go.dev/encoding/json/v2).
