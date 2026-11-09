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
listing, retrieval, a duplicate conflict, full replacement, repeated requests,
and deletion. It requires no external service and exits by itself.

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
curl -i -X PUT http://127.0.0.1:8080/contacts/1 \
  -H 'Content-Type: application/json' -d '{"name":"Ana Torres"}'
curl -i -X DELETE http://127.0.0.1:8080/contacts/1
curl -i -X DELETE http://127.0.0.1:8080/contacts/1
```

## Read the project in stages

1. [contacts/model.go](contacts/model.go): ordinary Go values, editable input,
   normalization, limits, and errors. No HTTP or JSON types are needed here.
2. [contacts/store.go](contacts/store.go): operations preserve IDs and the unique
   name index. A mutex protects all related state together, since handlers run
   concurrently. Its tests check snapshots, rejected writes, cancellation, and
   concurrent operations.
3. [httpapi/handler.go](httpapi/handler.go): a consumer-defined interface connects
   HTTP to those operations. Separate representation structs define JSON fields
   and navigation links without changing the contact model.
4. [httpapi/json.go](httpapi/json.go): bounded decoding, domain-error mapping, and
   consistent problem responses. [HTTP tests](httpapi/handler_test.go) cover the
   public contract, concurrent requests, and HEAD over a real connection.
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
| `GET /` | Discover the collection link. | `200`, JSON links. |
| `GET /contacts` | List contacts in increasing ID order. | `200`, JSON collection, including `[]` when empty. |
| `GET /contacts/{id}` | Retrieve a contact. | `200`, JSON contact. |
| `POST /contacts` | Create a contact; the server assigns its ID. | `201`, JSON contact and relative `Location` header. |
| `PUT /contacts/{id}` | Replace all editable fields of an existing contact. | `200`, JSON contact with the same ID. |
| `DELETE /contacts/{id}` | Remove a contact. | `204`, no response body. |

GET also supports HEAD; the real HTTP server suppresses the body. IDs are
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

A collection contains `contacts` and `links.self`. Clients can follow returned
links and `Location`; they do not need to guess assigned IDs. URLs identify
resources, HTTP methods express operations, and JSON represents their current
state. Each request supplies its own input; the API keeps resource data rather
than a per-client conversation. Responses explicitly use `Cache-Control:
no-store` in this version.

REST is an architectural style with constraints, not a synonym for JSON or CRUD.
This project practices resource identification, representations, links, method
semantics, and an explicit cache policy. It leaves broader API design exercises
for later, including conditional requests and richer navigation.

### PUT and idempotency

PUT replaces `name`, `email`, and `phone`; it keeps the server-owned ID and
links. Omitting email or phone clears them. It does not merge a partial update
and does not create a missing contact. That update-only policy is this API's
contract; HTTP also permits APIs that create resources with PUT.

PUT and DELETE are idempotent: repeating the same request has the same intended
effect on resource state as sending it once. That does not require identical
statuses: the first DELETE returns 204 and the next returns 404, with the contact
absent in both cases. POST is not generally idempotent. Here a duplicate name
produces 409; clients still need a deliberate retry policy after an ambiguous
network failure. GET and HEAD do not request changes to resource state.

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
  bodies are unsupported. Responses are JSON; this version does not negotiate
  another representation from `Accept`.

| Failure | Status |
| --- | --- |
| Malformed JSON, invalid field, invalid ID, or body read failure | 400 |
| Missing resource or unknown route | 404 |
| Unsupported method on a known route; includes `Allow` | 405 |
| Duplicate name on creation or replacement | 409 |
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

This is a small in-memory exercise. It has no persistence, pagination, PATCH,
authentication, or deployment configuration. Simultaneous replacements use the
last successful write; there is no lost-update protection. Useful follow-ups
are pagination, ETag/If-Match, persistence, and tests for those contracts. The
next transport exercise is Protobuf and gRPC over the same contact operations.

References: [HTTP semantics, RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html),
[Problem Details, RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html),
[Fielding's REST constraints](https://ics.uci.edu/~fielding/pubs/dissertation/rest_arch_style.htm),
[`net/http`](https://pkg.go.dev/net/http), and
[`encoding/json/v2`](https://pkg.go.dev/encoding/json/v2).
