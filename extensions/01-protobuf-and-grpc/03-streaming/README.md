# Streaming contacts over gRPC

Read the [unary stage](../02-unary/README.md) first. A unary call has one request
and one response. A **streaming** call can exchange several messages during
that one RPC. Learn one direction at a time before sending and receiving together.
The contact validation, store, and server lifecycle are shared with earlier stages.

## Run from the repository root

Requirements: Go 1.27 on PATH and a full repository checkout, including the local
modules. Use bash/zsh from the root containing `go.mod`, `chapters`, and
`extensions`. The demo and ordinary tests start their own local RPC servers and
use memory storage. Docker, a database, and separately generated certificates
are not runtime requirements. Generated Protobuf bindings are supplied; the
first run can download pinned Go dependencies. Race checks need clang or gcc.

```sh
go version
go -C extensions/01-protobuf-and-grpc run ./03-streaming
```

The default demo starts a temporary loopback TCP server, imports three contacts,
streams an ordered two-contact page, performs bidirectional lookups, observes
item failures and cancellation, deletes its contacts, and shuts down. Names
include a per-run suffix so repeated client runs can share a running server.
An interrupted or failed run can leave contacts in that process's store.

## Learn in passes

The executable imports its sample data first. For learning the call shapes,
read server streaming first, then import, then bidirectional lookup:

| Pass | Focus | Check your understanding |
| --- | --- | --- |
| 1 | Follow the server stream: two contacts arrive, then `io.EOF`. | Explain why receiving one contact does not establish successful completion. |
| 2 | Follow the client import: several sends, then one summary. | Explain why finishing the request direction still leaves a response to read. |
| 3 | Follow bidirectional lookup and its item failures. | Explain how a missing contact can coexist with a successful whole RPC. |
| 4 | Read cancellation, flow control, and the sender goroutine in [client.go](client.go). | Identify who sends, who receives, and how both finish on an error. |

Find these summaries among the output:

```text
ImportContacts -> OK: created=3, ids=[1 2 3]
StreamContacts -> EOF / OK: received=2
LookupContacts -> EOF / OK: replies=4
```

The IDs above come from a fresh temporary server. Names include a changing
suffix, omitted from the small examples below.

## Words and methods used by a stream

| Term / method | Meaning here |
| --- | --- |
| `Send(message)` | Hand the next message to gRPC for sending. This can wait and does not acknowledge the peer's business operation. |
| `Recv()` | Wait for the next incoming message or the end/error of that direction. |
| Half-close | Finish sending while continuing to receive. The other direction stays open. |
| `CloseSend()` | Half-close the client's request direction. It does not read the RPC's final status. |
| `CloseAndRecv()` | For client streaming, finish sending and wait for the single response plus final status. |
| `io.EOF` | The end of incoming messages. On the server it means the client finished sending; on the receiving client it means the response stream ended OK. |
| Snapshot | A copy of data selected for this query. Later store edits do not change that copy. |

`Recv` returns a message with nil error when one arrives. An `io.EOF` result is
an ordinary end to handle; another error means the call failed. The location
matters: server-side request EOF may still require sending the final response.

## Choose a call shape

| Shape | Requests → responses | Example here | When it fits |
| --- | --- | --- | --- |
| Unary | 1 → 1 | `ListContacts` | A bounded page with total/next-offset metadata. |
| Server streaming | 1 → many | `StreamContacts` | Consume results as messages arrive. |
| Client streaming | many → 1 | `ImportContacts` | Supply sequential inputs and receive one final summary. |
| Bidirectional | many → many | `LookupContacts` | Exchange requests and replies before either direction ends. |

Streaming does not automatically improve a small collection's performance.
This lab deliberately keeps the data small enough to inspect the lifecycle.
The [Go basics tutorial](https://grpc.io/docs/languages/go/basics/) describes
the four RPC shapes and their generated APIs.

Read [contacts.proto](../proto/contacts/v1/contacts.proto), then
[streaming.go](../rpcapi/streaming.go) and [client.go](client.go). Generic
`grpc.ServerStreamingServer[T]`, `grpc.ClientStreamingServer[Req, Res]`, and
`grpc.BidiStreamingServer[Req, Res]` are supplied by the pinned generated Go
bindings. As in stage 1, edit the contract and regenerate, not the bindings.

## Server streaming: one page, one snapshot

Follow the selected page from the default demo, which imported Ana, Bea, and Carla:

```text
Input: StreamContacts(limit=2, name sort descending)
Client Recv: Contact Carla
Client Recv: Contact Bea
Client Recv: io.EOF -> final OK
```

There is one request and several response messages. The client reads through
the end to learn whether the whole RPC succeeded.

`StreamContacts` accepts the same query as `ListContacts`: zero limit selects
20, other limits must be 1–100, offset must be nonnegative, and filter/sort rules
are shared. The handler obtains one page, releases the store lock, and sends
each contact in page order. An empty page sends no messages and ends normally.
Later store edits do not alter the selected snapshot.

The stream is finite. It does not watch changes or fetch all pages behind the
client's back. It returns contacts without total or next-offset metadata; use
unary `ListContacts` when those fields are needed. The selected page is buffered
before sending, so this is not a database cursor or an example of streaming an
unbounded collection with constant memory. The in-memory store still scans and
sorts matching data when selecting a page.

The client calls `Recv` until it returns `io.EOF` or an error. Opening a stream
successfully does not prove the server accepted its query. For example, an
invalid limit becomes `InvalidArgument` when the client receives the terminal
status. Receiving some contacts also does not guarantee the RPC will end OK.

## Client streaming: bounded import with partial commits

For a small successful import into an empty store:

```text
Client Send: create Ana
Client Send: create Bea
Client CloseAndRecv: finish request direction and wait
Server Recv: io.EOF -> send count=2, ids=[1, 2]
Client result: summary, nil error -> final OK
```

The server saves each item as it reads it; it can start work before the client
finishes sending. A transactional batch would save all items or none. This
import saves immediately, so a later failure can leave earlier contacts stored.

The client sends `CreateContactRequest` messages, then calls `CloseAndRecv`.
That call half-closes the request direction and waits for the summary and final
RPC status. The server creates each contact before reading the next request.
After request-side EOF, it uses `SendAndClose` to return the count and IDs in
creation order. An empty batch succeeds with a zero count and no IDs.

`MaxImportContacts` is 100. The 101st request fails with `ResourceExhausted`
before being created. This bounds per-RPC creates and the summary's ID list.
It is an application batch limit; transport message-size policies are a
separate topic in stage 5.

The import is **not transactional**. A blank name or duplicate ends the RPC
with the same error as unary creation. Earlier successful contacts remain:

```text
Send Ana          -> create commits
Send blank name   -> InvalidArgument; handler stops
CloseAndRecv      -> error, no successful summary; Ana remains in the store
```

Cancellation and deadlines also do not roll back committed contacts. A client
can lose the final response after the server has changed data, so a failed call
cannot establish that nothing was created. There is no automatic retry or
idempotency key here; blindly resending can duplicate work or hit name conflicts.
Stage 5 addresses explicit retry/idempotency policies.

`Send` returning nil means the transport accepted the message for sending, not
that the store committed it. A client-side `Send` returning `io.EOF` means the
server ended early: call `CloseAndRecv` to learn the actual final status. It
does not mean the batch succeeded. These lifecycle rules are documented by
[`ClientStreamingClient`](https://pkg.go.dev/google.golang.org/grpc@v1.85.0#ClientStreamingClient).

## Bidirectional: per-item outcomes and terminal failures

Both sides can exchange messages before either direction finishes. A short
conversation after contact 1 exists could be:

```text
Client Send: request_id=1, contact id=1 -> server replies with the contact
Client Send: request_id=2, contact id=0 -> server replies with an item failure
Client CloseSend: no more lookup requests
Client keeps receiving, then sees io.EOF -> final OK
```

The request ID connects each reply to the input that caused it. The invalid
contact ID is represented inside one reply; it does not terminate this API's
conversation. The demo sends and receives concurrently, as explained below.

Each `LookupContactsRequest` contains a contact ID and a client-chosen
`request_id`. The server echoes the correlation value and sends one response
per request, in request order, without waiting for the client to half-close.
Clients should choose distinct correlation values if they need to distinguish
requests. The service does not keep a growing history of seen request IDs.

The response uses a Protobuf `oneof result`. The service sets either `contact`
or `failure`. Inspect the selected Go wrapper with a type switch; the zero value
of a getter alone does not identify the selected branch. Protobuf permits an
unset oneof, so the client also rejects responses without an outcome. See the
[generated oneof guide](https://protobuf.dev/reference/go/go-generated/#oneof).

| Result | Representation | Does the conversation continue? |
| --- | --- | --- |
| Contact exists | `contact` branch with its current version | Yes. |
| Invalid ID | `failure` branch, `LOOKUP_FAILURE_CODE_INVALID_ARGUMENT` | Yes. |
| Contact missing | `failure` branch, `LOOKUP_FAILURE_CODE_NOT_FOUND` | Yes. |
| Unexpected store error | Terminal `Internal` status with redacted details | No. |
| Cancellation/deadline | Terminal `Canceled` / `DeadlineExceeded` status | No. |

Item failures are application data, not gRPC status codes. The entire RPC can
finish OK even when some items failed. Conversely, a terminal error can follow
already delivered successful replies. This API makes that distinction explicit;
other streaming APIs may choose different policies.

The server processes one request and sends its reply before receiving another.
The client uses one sender goroutine and receives concurrently. That goroutine
owns both `Send` and `CloseSend`; the receiving goroutine never calls them.
There may be one concurrent sender and one receiver on a stream, but concurrent
sends or concurrent receives on the same stream are unsupported. After sending,
`CloseSend` ends only the request direction; the client keeps receiving until
the final EOF/error. On an early receive or processing error, the client cancels
the stream and joins its sender before returning. See
[`ClientStream`](https://pkg.go.dev/google.golang.org/grpc@v1.85.0#ClientStream).

## Completion, cancellation, and flow control

| Observation | Meaning |
| --- | --- |
| Server `Recv` returns `io.EOF` | Client finished sending; the server may still need to reply. |
| Client `Recv` returns `io.EOF` | Response stream ended with OK status. |
| Client `Recv` returns another error | RPC ended unsuccessfully; inspect its status. |
| `CloseSend` returns nil | Request direction was half-closed; final RPC status is still unread. |
| `CloseAndRecv` returns response, nil | Client-streaming RPC completed successfully with its summary. |

`Send` can block when a receiver is slow and transport buffers/windows fill.
This waiting is **backpressure**: the sending side slows down when the receiving
side cannot keep up. **Flow control** is the transport's mechanism for regulating
how much data can be sent before more capacity becomes available.
Wait for it; creating a goroutine or an unbounded queue per outgoing message
would only move the pressure into application memory. The handlers here add
no such queue and hold no store lock during sends. Actual gRPC/HTTP2 buffers
still exist, and the store itself grows with created contacts. There is no
claim that one `Send` corresponds to immediate peer receipt.

For bidirectional calls, sending all messages before reading can deadlock when
both peers fill their buffers. Concurrent sending/receiving lets both
directions progress. These effects are described in the official
[flow-control guide](https://grpc.io/docs/guides/flow-control/).

Every stream has a deadline and an owned cancel function. The finite client
uses a 10-second run budget and a 3-second budget for each streaming helper.
Cancellation reaches `stream.Context()`, store operations, and blocked transport
operations. The cancellation demo first receives a reply from an active lookup,
then cancels while the server awaits more requests. A separate shutdown context
allows graceful draining; after its budget, forced stop cancels active streams.

Do not mutate a message after passing it to `Send`. As with unary messages,
construct a fresh message or clone it before independent changes. Neither
generated message pointers nor a stream make shared mutable state safe.

## What the tests verify

After following the message flows, run the checks:

```sh
go -C extensions/01-protobuf-and-grpc test -count=1 -timeout=30s ./...
CGO_ENABLED=1 go -C extensions/01-protobuf-and-grpc test -race -count=3 -cpu=1,2 -timeout=45s ./...
go -C extensions/01-protobuf-and-grpc vet ./...
```

- Bufconn RPCs check filtered ordering, empty streams, invalid queries, import
  summaries, the batch cap, partial commits, correlation, item failures,
  request half-close, and a terminal failure after a delivered reply.
- Controlled store operations verify in-flight cancellation and deadlines for
  all three stream shapes, including release of the server's blocked work.
- A gated `Send` models backpressure and verifies snapshot ownership, released
  store locks, sequential sends, and cancellation. This deterministic test does
  not measure real HTTP2 window sizes or transport memory consumption.
- Client tests verify that early send EOF still leads to reading the final
  status and that a receive failure cancels and joins a blocked sender.
- TCP tests run the finite demo, repeated clients against a standalone server,
  and graceful/forced shutdown with an active streaming call. Race checks cover
  sending and receiving concurrently.

## Separate server and client

After the finite demo, keep a server running in one terminal:

```sh
go -C extensions/01-protobuf-and-grpc run ./03-streaming -serve
```

Then run the finite client in another terminal:

```sh
go -C extensions/01-protobuf-and-grpc run ./03-streaming -target 127.0.0.1:50051
```

Stop the server with Ctrl+C. Use `-addr 127.0.0.1:50052` and a matching `-target`
if the default port is busy. Both the unary and streaming commands register the
complete service; either running server supports both clients. Standalone
processes each own an independent in-memory store. This stage still uses local
plaintext credentials; TLS, metadata, and interceptors follow in stage 4.

Before continuing, explain why a failed import can leave data, why an item
failure can coexist with final OK, and why half-closing is different from
receiving the final status. Then continue to the
[infrastructure stage](../04-infrastructure/README.md) for TLS, metadata,
interceptors, identity, health, reflection, and grpcurl. Resilience follows in
[stage 5](../05-resilience/README.md).
