# ETag and conditional requests

Read [the resource API](README.md) and [collection queries](collection-queries.md)
first. This extension adds two useful behaviors: revalidating a stored response
and preventing an edit based on outdated data.

An **ETag** is a label for the current representation. A client keeps the label
with its saved data and sends it back to check whether that data is still current.
The label is a **validator** because it supports that check. Treat it as opaque:
copy it as received, including quotes, instead of interpreting or constructing it.

## Read, save, and revalidate

A successful GET or HEAD returns an `ETag`, an opaque validator enclosed in
double quotes. Save the tag with the representation. Send it in
`If-None-Match` when requesting that same URL again:

| Situation | Response |
| --- | --- |
| Current tag matches the supplied tag | 304, current ETag and cache policy, no body or Content-Type. |
| Representation has changed | 200, current JSON and its new ETag. |
| Contact no longer exists | 404 Problem Details; the ordinary missing-resource error takes precedence. |

A 304 lets a client reuse its saved representation. It is not a JSON document
to decode. The finite demo shows the protocol without implementing a client
cache. HEAD never carries a response body, including on error.

Successful reads use `Cache-Control: private, no-cache`: a private cache may
store the response but must validate it before reuse. `no-cache` permits storage
with revalidation; `no-store`, used for errors and mutation responses, prohibits
storage. These are distinct policies.

GET/HEAD on `/`, `/contacts`, `/contacts/{id}`, and `/openapi.json` support both
`If-Match` and `If-None-Match`. Filtered page tags cover the actual JSON bytes,
including contacts, links, and totals. A change outside a selected page only
changes its tag if it changes that page's representation. Revalidation still
builds the selected representation; it reduces transfer, not search work.

## Read, edit, and compare before writing

Two clients can read the same contact and make different edits. A mutex makes
each write atomic but cannot decide whether a client's starting data is current.
An optional `If-Match` precondition adds that decision:

1. Both clients read version A.
2. Client one sends PATCH with `If-Match: A`; the edit succeeds and produces B.
3. Client two sends an edit with `If-Match: A`; the server returns 412 and saves
   nothing, because A no longer identifies the current representation.
4. Client two reads the new state and decides whether its edit still makes sense.

PUT, PATCH, and DELETE on an existing contact support these conditions. Omitting
them preserves the original unconditional behavior. Clients that need stale-edit
protection must send a current `If-Match`; `If-Match: *` only requires existence.
A missing contact still returns 404 because these routes are update-only.

The store checks its normal rules first, then compares versions and commits the
write while holding one lock. A duplicate name remains 409, invalid contact
input remains 400, and a missing contact remains 404. A failed version condition
is 412. Comparing with Get and later calling a separate Replace would create a
gap for another writer, so it would not provide this guarantee.

POST on the collection and OPTIONS reject entity-tag condition headers with
400 in this API. They do not implement conditional creation or conditional
capability discovery. Date-based validators and range requests are outside this
exercise's contract.

### Header rules

- `If-Match` uses strong comparison. A weak `W/"tag"` cannot satisfy it.
- `If-None-Match` uses weak comparison. Either `"tag"` or `W/"tag"` can match the
  current tag. A match returns 304 for GET/HEAD and 412 for contact mutations.
- Both headers accept `*` or quoted tag lists. A comma inside quotes belongs to
  the opaque tag. Multiple header lines are combined as a list. Empty list
  elements are tolerated; an empty list matches nothing.
- `*` cannot be combined with other tags. Invalid header syntax returns 400.
- When both headers are supplied, `If-Match` is evaluated first.

## Try it

Start a fresh interactive server with `-serve`, then create a contact:

```sh
curl -i -X POST http://127.0.0.1:8080/contacts \
  -H 'Content-Type: application/json' -d '{"name":"Ana"}'
curl -i http://127.0.0.1:8080/contacts/1
```

Use the returned Location; a fresh store starts at `/contacts/1`. Copy GET's ETag,
including double quotes. Replace the placeholder in this variable with that
actual tag before running the following requests:

```sh
etag='"contact-json-v1-REPLACE_WITH_THE_RETURNED_VALUE"'
curl -i http://127.0.0.1:8080/contacts/1 -H "If-None-Match: $etag"
# 304 with no body: the saved copy is still current.

curl -i -X PATCH http://127.0.0.1:8080/contacts/1 \
  -H 'Content-Type: application/merge-patch+json' \
  -H "If-Match: $etag" -d '{"email":"current@example.com"}'
# 200 with the updated contact and its new ETag.

curl -i -X PATCH http://127.0.0.1:8080/contacts/1 \
  -H 'Content-Type: application/merge-patch+json' \
  -H "If-Match: $etag" -d '{"email":"outdated@example.com"}'
# 412: the variable still holds the old tag. No fields change.
```

The demo and tests run this scenario automatically without copying any tags.

## Understand the implementation

- [contacts/conditions.go](contacts/conditions.go) gives content an opaque
  version and defines transport-independent matching conditions. Length-prefixed
  fields are hashed with SHA-256; equal current values have equal versions.
- [contacts/store.go](contacts/store.go) exposes `ReplaceIf`, `PatchIf`, and
  `DeleteIf`. The original methods call them with zero conditions, preserving
  the basic examples. Validation, checking, and mutation share the store lock.
- [httpapi/conditions.go](httpapi/conditions.go) parses HTTP entity tags, maps
  conditions to domain versions, and handles read preconditions and 304.
  A representation-format prefix distinguishes contact JSON from other formats;
  changing the representation or encoder requires changing that prefix.
- [Domain tests](contacts/conditions_test.go) and
  [HTTP tests](httpapi/conditions_test.go) check stale writes, weak/strong
  comparisons, lists, wildcard and empty-list behavior, actual HEAD responses,
  page validators, and competing writers using the same initial tag.

Contact tags identify content rather than counting writes. No-op updates retain
the same tag. Changing data and later restoring the same values can restore the
earlier tag; a matching current representation satisfies the condition. Tags are
not authentication tokens and clients should not construct them.

### Why PUT does not return a new ETag here

POST returns Location and the new contact's ETag. PATCH returns the updated
contact and its ETag. GET and HEAD return the selected representation's ETag.

PUT trims fields, supplies defaults, and constructs a different representation
from its request document. HTTP forbids a success validator after transformed
PUT content. This API therefore omits ETag on successful PUT; follow it with GET
to obtain the current validator. DELETE has no remaining representation to tag.

The [OpenAPI contract](openapi.md) describes these headers, statuses, and bodies.
Run the ordinary and race checks in the [main guide](README.md#validate-what-you-learned).

References: [HTTP validators and preconditions, RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html#name-preconditions),
[PUT validator rule](https://www.rfc-editor.org/rfc/rfc9110.html#name-put), and
[HTTP caching, RFC 9111](https://www.rfc-editor.org/rfc/rfc9111.html).
