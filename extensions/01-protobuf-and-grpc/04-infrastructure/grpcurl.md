# Inspect and call the TLS server with grpcurl

Start the [standalone infrastructure server](README.md#separate-server-and-client)
with `-reflection`, using the CA directory and example credentials from that
guide. Set the same environment variables in this terminal. Commands below run
from the repository root and expect `127.0.0.1:50052`.

The Go demo/tests do not require grpcurl. For these optional exercises, use
grpcurl v1.9.4, independently from the course module. Install the pinned command
with `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@v1.9.4` and place its
installation directory on PATH, or replace `grpcurl` below with
`go run github.com/fullstorydev/grpcurl/cmd/grpcurl@v1.9.4`. Versioned `go run`
does not add it to this lab's `go.mod`. See the
[tool's own guide](https://github.com/fullstorydev/grpcurl).

## List and describe services

```sh
grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  127.0.0.1:50052 list

grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  127.0.0.1:50052 describe contacts.v1.ContactService
```

Expect the contact, health, and reflection services, then the nine contact RPCs.
The CA file verifies the server; neither `-plaintext` nor `-insecure` belongs in
these verified-TLS commands. The flags precede the address and method.

## Unary calls and metadata

```sh
grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 -v \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  -H 'x-request-id: grpcurl-create' \
  -d '{"input":{"name":"Grpcurl TLS"}}' \
  127.0.0.1:50052 contacts.v1.ContactService/CreateContact
```

Inspect the returned contact, `x-request-id` response header, and `x-rpc-kind`
trailer. Copy the ID from the response into subsequent requests; `"1"` below
assumes a fresh server's first contact. IDs use ProtoJSON string representation.
Creating the same name again yields `AlreadyExists`; use a new name or restart
the in-memory server when repeating these exercises.

```sh
grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" -d '{"id":"1"}' \
  127.0.0.1:50052 contacts.v1.ContactService/GetContact
```

## Three streaming shapes

Server streaming returns one bounded ordered page, without collection metadata:

```sh
grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
  -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" -d '{"limit":1}' \
  127.0.0.1:50052 contacts.v1.ContactService/StreamContacts
```

Client streaming accepts whitespace-separated JSON request messages on stdin.
After stdin EOF, grpcurl waits for the summary and terminal status:

```sh
printf '%s\n' '{"input":{"name":"Grpcurl import A"}}' '{"input":{"name":"Grpcurl import B"}}' | \
  grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
    -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" -d @ \
    127.0.0.1:50052 contacts.v1.ContactService/ImportContacts
```

Bidirectional lookup replies independently for each input. Replace both IDs
as needed to compare an existing contact with a missing one:

```sh
printf '%s\n' '{"requestId":"1","id":"1"}' '{"requestId":"2","id":"999999"}' | \
  grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
    -H "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" -d @ \
    127.0.0.1:50052 contacts.v1.ContactService/LookupContacts
```

The missing contact is an item failure, while the whole lookup can end OK.
Failed imports can leave earlier creates committed, as explained in
[stage 3](../03-streaming/README.md#client-streaming-bounded-import-with-partial-commits).

## Discovery permissions versus method permissions

`-H` attaches metadata to both reflection and the target RPC. A reader can call
GetContact, but cannot discover descriptors through this lab's writer-only
reflection service. Use the local schema to make a reader call without reflection:

```sh
grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
  -import-path extensions/01-protobuf-and-grpc/proto \
  -proto contacts/v1/contacts.proto \
  -H "authorization: Bearer $GRPC_LAB_READER_TOKEN" -d '{"id":"1"}' \
  127.0.0.1:50052 contacts.v1.ContactService/GetContact
```

Alternatively, separate descriptor discovery credentials from target RPC
credentials. This request should reach CreateContact and yield `PermissionDenied`:

```sh
grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
  -reflect-header "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  -rpc-header "authorization: Bearer $GRPC_LAB_READER_TOKEN" \
  -d '{"input":{"name":"Denied"}}' \
  127.0.0.1:50052 contacts.v1.ContactService/CreateContact
```

Omit `-rpc-header` in that command to observe `Unauthenticated` from the contact
method. Descriptor discovery remains authorized using `-reflect-header`.

The same separation demonstrates the deliberately public health Check: schema
discovery uses the writer token, but the health method itself receives no token:

```sh
grpcurl -cacert /tmp/golang-course-grpc-tls/ca.pem -max-time 5 \
  -reflect-header "authorization: Bearer $GRPC_LAB_WRITER_TOKEN" \
  -d '{"service":"contacts.v1.ContactService"}' \
  127.0.0.1:50052 grpc.health.v1.Health/Check
```

This reports `SERVING`. Reflection is optional tooling; supplying a local
`.proto` changes how grpcurl learns the schema, without bypassing TLS or the
server's method authorization.
