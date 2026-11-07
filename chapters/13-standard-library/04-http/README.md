# HTTP handlers, clients, and shutdown

Practice `net/http` routing, middleware, JSON requests, HTTP status handling,
bounded bodies, reusable clients, timeouts, and graceful server shutdown.
This chapter 13 lesson builds on concurrency, I/O, time, and JSON. It uses the
course's Go 1.27 toolchain and `encoding/json/v2`.

## Run and validate

Run from the repository root:

```sh
go run ./chapters/13-standard-library/04-http
go test ./chapters/13-standard-library/04-http
go test -race -timeout=30s ./chapters/13-standard-library/04-http
go vet ./chapters/13-standard-library/04-http
```

To exercise concurrent requests and shutdown across more schedules:

```sh
go test -race -count=3 -cpu=1,2 -timeout=30s ./chapters/13-standard-library/04-http
```

The program binds `127.0.0.1:0`, letting the OS choose an available local port.
It runs its own requests, closes client resources, shuts down the server, and
joins its `Serve` goroutine before exiting. No public service, fixed port,
certificate files, or manual stop is required. It is a finite demonstration;
it does not remain running for interactive requests afterward.

## Read the example

- [handler.go](handler.go) defines the wire types, pure greeting logic, private
  mux, header middleware, request validation, and JSON response writer.
- [client.go](client.go) constructs a reusable client, builds requests with
  context, and handles bounded responses and cleanup.
- [server.go](server.go) sets server I/O budgets and owns graceful shutdown.
- [main.go](main.go) starts the local server, exercises success/error paths,
  demonstrates HEAD and cancellation, and waits for cleanup.
- [handler_test.go](handler_test.go) checks routing, representation, domain rules,
  size limits, and serialization failure before a response is committed.
- [client_test.go](client_test.go) checks body closure, combined failures, status
  handling, response validation, and timeouts with a controlled clock.
- [server_test.go](server_test.go) checks local HTTP/HTTPS, concurrent requests,
  unknown-length uploads, HEAD behavior, and an active request during shutdown.

## Routes and handler composition

| Route | Input | Success |
| --- | --- | --- |
| `GET /health` | No input required. | `200` with `{"status":"ok"}`. |
| `GET /greetings/{language}` | `name` query parameter; language is `en` or `es`. | `200` with message and language. |
| `POST /greetings` | JSON object with `name` and optional `language`. | `200` with message and language; omitted language defaults to `en`. |

`http.Handler` is the `ServeHTTP` contract. `http.HandlerFunc` adapts an ordinary
function to that interface. A private `ServeMux` selects handlers without changing
the process-global default mux. Method-aware patterns and `PathValue` use routing
introduced in Go 1.22, after the book's original examples.

The middleware sets `X-Course: learning-go` before delegating. It wraps the mux,
so the header also appears on routing errors. Set headers before `WriteHeader`
or `Write` commits a response. `Write` implicitly commits status 200 if needed;
later header mutations cannot rewrite an already sent response.

GET patterns also match HEAD. The server suppresses HEAD's response body while
preserving appropriate headers. A direct `ResponseRecorder` invocation does not
fully model that wire behavior, so the HEAD test uses a real local server.
An unmatched route gets the mux's 404; a matching path with an unsupported method
gets 405 and an `Allow` header. Those default errors are plain text. The example
keeps the mux's behavior rather than adding a catch-all that changes method matching.

`PathValue` reads the route segment and `URL.Query().Get` reads the first `name`
value. The client escapes path segments with `url.PathEscape` and uses `url.Values`
for query encoding. Concatenating an unescaped name containing `&`, `?`, or `+`
would change the request's meaning. Use each URL component's encoding rules.

## Validate the request and choose a status

Names are trimmed, must contain 1–80 valid UTF-8 runes afterward, and may include
Unicode or punctuation. Language is trimmed and must be `en` or `es`, with an
empty value defaulting to `en`. `makeGreeting` expresses these domain rules
without depending on HTTP, and uses only values local to the call.

| Failure | Status | Representation |
| --- | --- | --- |
| Missing, malformed, or unsupported POST media type | 415 | JSON problem with `error`. |
| Accepted JSON body exceeds 4,096 bytes | 413 | JSON problem. |
| Body read failure, invalid JSON/type, null, extra document, or unknown member | 400 | JSON problem. |
| Blank/long name or unsupported language | 400 | JSON problem. |
| Unknown route / wrong method | 404 / 405 | Native mux plain-text error. |
| Response cannot be encoded | 500 | Plain-text internal error before any success is committed. |

The POST handler accepts `application/json`, including parsed parameters such as
`charset=utf-8`. After that media-type check it uses `http.MaxBytesReader` before
reading the body. A `*http.MaxBytesError` identifies overflow even when the request
has no known `Content-Length`. The byte budget includes whitespace; it is separate
from the name's rune limit and the server's header limit.

Decode into fresh local storage, require one object, reject unknown members, and
then apply domain rules. v2 rejects duplicate names and invalid UTF-8 by default.
A JSON null decodes to a nil request pointer and is rejected. The operation
computes a greeting without persisting a resource, so success uses 200.

`writeJSON` serializes before setting the final status. Once response bytes are
being written, an I/O failure is recorded with the basic logger rather than
attempting a second response. The next lesson covers
[structured logging](../05-structured-logging/README.md). Each
request receives its own writer; finish using it before `ServeHTTP` returns.

## Reuse the client and inspect the response

`newClient` clones the default transport, sets explicit budgets, and reuses its
connection pool across requests. A client and its transport support concurrent
requests; configure them before sharing, and do not change shared transport
fields while requests are running. The handler here has no shared mutable data;
adding a shared map or store would require its own synchronization contract.

`NewRequestWithContext` binds each request to its caller's cancellation.
`getGreeting` builds the path/query and `postGreeting` encodes JSON and sets its
media type. Context use here previews chapter 14's dedicated
[propagation lesson](../../14-context/01-context/README.md).

`Client.Do` returning nil error means a response was received, not that the status
is successful. `doGreeting` distinguishes:

- Transport/cancellation failures, preserved through wrapping.
- Non-2xx responses, represented by `*HTTPStatusError` even for non-JSON bodies.
- Body read, size, or close failures, retaining an HTTP status cause when relevant.
- Successful statuses with incompatible content type, malformed JSON, or an
  incomplete greeting representation.

Error response bodies may supply the problem's message; otherwise the status text
is used. Success responses allow additive JSON members for compatibility with a
newer server, while validating the message and language needed by this client.
The standard client follows redirects by default, so these status checks apply
to the response returned after that redirect policy.

The response body is read through a 4,097-byte probe and closed on every path.
Small bodies reach EOF, allowing connection reuse when the transport and server
permit it. Oversized bodies are closed without unbounded draining merely to
preserve a connection. A close failure is joined with earlier failures, and any
error returns a zero greeting. The owner closes idle connections when finished
with the client; doing so after each request would discard useful pooling.

## Resource ownership

| Resource | Owner and responsibility |
| --- | --- |
| Incoming server request body | `net/http` closes it after the handler; the handler applies the read bound. |
| Outgoing client request body | The transport closes it, including on request errors. |
| Client response body | The caller receiving the response consumes as appropriate and closes it, including for errors and HEAD. |
| Client/transport pool | The creator reuses it, then calls `CloseIdleConnections` when appropriate. This does not join active requests. |
| Response writer | Used only during the handler call; no background writer is started. |
| Listener and server | `Serve` owns the listener; the application initiates shutdown and observes `Serve` completion. |

This example uses ordinary local HTTP. HTTPS clients use TLS verification and
their configured trust roots. The HTTPS test uses `httptest.NewTLSServer` and
its trusted client rather than disabling certificate verification. Serving HTTPS
directly would also require certificate configuration, such as with `ServeTLS`.

## Timeouts cover different phases

The configured numbers are example budgets for this small API, not universal
defaults for every workload. A zero client timeout has no total deadline.

| Budget | Configured value | Scope |
| --- | --- | --- |
| Client `Timeout` | 5 seconds | Entire request, including connection, redirects, and response body reading. |
| Dialer `Timeout` | 2 seconds | Establishing the network connection. |
| Transport `TLSHandshakeTimeout` | 2 seconds | TLS handshake on HTTPS connections. |
| Transport `ResponseHeaderTimeout` | 2 seconds | Waiting for response headers after the request, including its body, has been written. |
| Transport `IdleConnTimeout` | 30 seconds | Retention of idle pooled connections. |
| Server `ReadHeaderTimeout` | 2 seconds | Reading request headers. |
| Server `ReadTimeout` | 5 seconds | Reading the entire request, including the body. |
| Server `WriteTimeout` | 5 seconds | Response writes; reset when a new request's headers are read. |
| Server `IdleTimeout` | 30 seconds | Waiting for the next request on a keep-alive connection. |

`MaxHeaderBytes` separately bounds header parsing, including the request line;
it does not bound bodies. Server I/O timeouts do not preempt arbitrary handler
computation. Work that blocks on something else must observe a cancellation or
deadline protocol of its own. Incoming `r.Context()` is available for propagating
request cancellation into that work; the pure greeting operation needs no wait.
These small fixed request/response budgets would need revisiting for streaming
responses or large uploads.

## Finish shutdown, then exit

`Shutdown` stops accepting connections and waits for normal active requests to
finish. `Serve` returns `http.ErrServerClosed` during that process; treat it as
normal, and wait for `Shutdown` itself rather than exiting as soon as `Serve` returns.

The owner uses a fresh context with a three-second shutdown budget and joins its
`Serve` goroutine. If graceful shutdown fails, it preserves the error and falls
back to `Close`. Closing connections cannot force arbitrary application code to
return; long-running handlers still need a cooperative stop policy. Graceful
shutdown also needs separate coordination for hijacked connections, which this
lesson does not use. Defers run inside `run` before `main` can call `os.Exit`.

The shutdown test holds one active handler behind an explicit gate, starts
shutdown, verifies it has not returned, and releases the handler. No `Sleep`
guesses establish ordering. Five-second test timers are watchdogs. Client timeout
tests use `testing/synctest` with a context-aware fake transport, checking both
header waiting and body reading without network I/O or real-time delays inside
the fake-clock bubble.

References: [`net/http`](https://pkg.go.dev/net/http),
[routing enhancements](https://go.dev/blog/routing-enhancements),
[`httptest`](https://pkg.go.dev/net/http/httptest),
[`net/url`](https://pkg.go.dev/net/url),
and [`testing/synctest`](https://pkg.go.dev/testing/synctest).
