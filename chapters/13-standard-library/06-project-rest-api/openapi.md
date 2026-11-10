# OpenAPI contract and verification

An **API contract** describes which requests clients may send and which
responses they can expect. **OpenAPI** expresses that description in a format
tools can read. A **schema** describes the allowed shape of data, such as a
contact object's fields and types.

The [checked-in OpenAPI 3.1.1 document](httpapi/openapi.json) describes this
exercise's HTTP API. It covers discovery, contact CRUD, PUT/PATCH differences,
collection queries, optional fields, validation failures, conditional headers,
and the contract endpoint itself.

Read it alongside the [resource guide](README.md),
[collection queries](collection-queries.md), and
[conditional requests](conditional-requests.md). The contract describes HTTP;
the shared contacts package continues to use ordinary Go values and errors.

## Read or import the contract

With the interactive server running:

```sh
curl -i http://127.0.0.1:8080/openapi.json
```

GET `/` also provides the `links.openapi` discovery link. Import the JSON file
or endpoint into an OpenAPI-compatible viewer or client tool. The server
provides the document directly; it does not download or host a documentation UI.

[openapi.go](httpapi/openapi.go) embeds that exact file with `go:embed`. This
works even when the binary runs outside the repository. GET and HEAD on the
contract support ETag revalidation just like the resource reads. No source
generation or working-directory-dependent file lookup is required.

## What the document specifies

| Section | Purpose |
| --- | --- |
| `info` and `servers` | Contract version, scope, and same-origin server URL. |
| `paths` | Each supported path and method, operation IDs, parameters, request bodies, statuses, response headers, and payload schemas. |
| `components.schemas` | Separate writable input, patch, contact, collection, links, pagination, discovery, and Problem Details schemas. |
| `components.parameters` | Reusable ID/query parameters and optional conditional headers. |
| `components.headers` | ETag, Location, cache policy, Allow, patch format, and nosniff. |
| `components.responses` | Shared errors and 304, avoiding separate definitions for every operation. |

`$ref` points to reusable definitions in the same file. Optional contact fields
are absent when empty. Request schemas reject unknown members and keep ID and
links out of writable input. Merge Patch distinguishes omission, strings, and
null; runtime validation protects the required name. Responses with 204 or 304
have no content schema because they carry no body. HEAD also carries no body.

Schema validation complements the Go rules. Name uniqueness, maximum body
bytes, duplicate-key rejection, normalization, and validity after applying a
patch are runtime constraints. Input length limits are described after trimming;
a raw `maxLength` would incorrectly reject valid padded input. Response schemas
can state the normalized field lengths directly.

OpenAPI paths list supported operations. The `x-method-not-allowed-response`
and `x-unknown-route-response` extensions describe this exercise's 405 and 404
fallback responses for the live checker; ordinary OpenAPI tooling may ignore
these custom extensions. Normal mux path-cleaning redirects and transport-level
request rejection are handled by `net/http`, outside application JSON responses.

## Validate without adding Go dependencies

The normal Go tests check the embedded document, public-operation coverage,
conditional serving, and application behavior. Run them using the
[main guide](README.md#validate-what-you-learned).

For full document validation and a live contract check, use the optional Python
developer tool with Python 3.10+ on macOS/Linux and Go on PATH. From the repository
root, create an isolated environment in a temporary directory:

```sh
contract_env=$(mktemp -d)
python3 -m venv "$contract_env"
"$contract_env/bin/python" -m pip install \
  -r chapters/13-standard-library/06-project-rest-api/validation/requirements.txt
"$contract_env/bin/python" \
  chapters/13-standard-library/06-project-rest-api/validation/validate_contract.py
```

The initial installation downloads validation tools. Running the check afterward
uses the local document and loopback HTTP, with no external service. The
environment can be removed after use. These packages are validation dependencies;
the Go application and its normal tests still use only the standard library.

The [live checker](validation/validate_contract.py):

1. Validates the document with `openapi-spec-validator`.
2. Builds a temporary Go binary, starts it on an available loopback port, and
   runs it outside the source directory.
3. Sends 37 requests covering success/error paths, pagination, ETag revalidation,
   stale writes, body/media-type failures, methods, HEAD, and deletion.
4. Uses JSON Schema 2020-12 validation for successful request fixtures, documented
   response bodies, and required headers. It checks status agreement for Problem
   Details and that HEAD/204/304 responses have no body.
5. Stops the owned server and removes the temporary binary, including on failure.

This is contract checking over selected fixtures, not exhaustive coverage of all
inputs. Go tests add controlled failures, cancellation, and concurrent schedules.
When changing an endpoint, update the document, tests, examples, and relevant
guide together. The embedded contract is refreshed by rebuilding the Go binary.

References: [OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html) and
[`openapi-spec-validator`](https://pypi.org/project/openapi-spec-validator/).
