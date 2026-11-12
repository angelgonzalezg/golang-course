# Protobuf: messages before remote calls

This first stage works locally. Learn to describe data, encode it, and recover
it before sending it to another process. Later stages use the same messages
for gRPC calls. Read structs, pointers, errors, and packages first.

## Run from the repository root

Requirements: Go 1.27 on PATH and a full repository checkout, including the local
course/SQL modules referenced by the gRPC module. Use bash/zsh from the root
directory containing `go.mod`, `chapters`, and `extensions`. This serialization
lab needs no Docker, database, credentials, or external service. Go can download
the pinned dependencies on the first run; generated bindings are supplied.

```sh
go version
go -C extensions/01-protobuf-and-grpc run ./01-protobuf
```

The program prints five numbered examples and exits. Generated Go files are
already checked in; running this lesson needs no protoc compiler or server.
The first run may download the module's pinned dependencies.

## Learn in passes

Each pass follows the matching numbered section in [main.go](main.go).

| Pass | Focus | Check your understanding |
| --- | --- | --- |
| 1 | A message and its binary form. | Explain the path from a Go message to bytes and back. |
| 2 | The separate ProtoJSON form. | Explain why the displayed ID is a JSON string. |
| 3 | Absent versus explicitly empty input. | Find two messages whose getter returns the same value but whose presence differs. |
| 4 | A field mask selecting an empty value. | Explain how a client can request clearing email. |
| 5 | Independent edits with `proto.Clone`. | Explain which variable can be edited without changing the original. |

## 1. Describe data, then encode and decode it

| Term | Meaning here |
| --- | --- |
| Message | A structured value, such as a contact with ID and name fields. |
| `.proto` contract | The file describing message fields and RPC signatures. |
| Generated Go code | Types and helpers produced from that contract. |
| Serialize / encode | Convert a message into bytes for storage or transfer. |
| Deserialize / decode | Read those bytes into a message again. |

Follow the first example:

```text
Input: Contact with id=42 and name="Ana López"
proto.Marshal: encode the message into []byte
proto.Unmarshal: decode those bytes into a new Contact
Result: the new message has the same field values
```

The output confirms `binary round-trip preserved the message`. A round-trip
means encoding and decoding the same data. `proto.Equal` compares the message
values; it does not ask whether the two Go pointers are the same address.

In [contacts.proto](../proto/contacts/v1/contacts.proto), `int64 id = 1` gives the
field **number 1** in the binary format. The contact's actual ID can still be
42. The field number identifies which field the bytes belong to; it is not the
field's value. Keep established field numbers when changing the contract.

The application uses the types in
[contacts.pb.go](../gen/contacts/v1/contacts.pb.go). Edit the `.proto` and
regenerate when the contract changes; generated Go files are compiler output.

## 2. Recognize binary Protobuf and ProtoJSON

The binary round-trip above uses `proto.Marshal` and `proto.Unmarshal`.
The next example uses `protojson.Marshal` to display the message as readable JSON:

```json
{"id":"42","name":"Ana López","email":"ana@example.com","version":"example-version"}
```

The same message has two representations. gRPC uses its binary representation
in this lab; ProtoJSON makes it convenient to inspect in the terminal. Protobuf
JSON rules represent this int64 ID as a string. Use `protojson` when you want
those rules, rather than assuming an ordinary `encoding/json` encoding is equivalent.

## 3. Distinguish a value from its presence

**Presence** tells you whether a field was supplied. An empty string alone
cannot always answer that question. The optional `expected_version` field
lets the application distinguish these two requests:

| Go field | Getter result | Present? | Meaning in the next stage's service |
| --- | --- | --- | --- |
| `ExpectedVersion: nil` | `""` | No | Perform the operation without a version condition. |
| `ExpectedVersion: proto.String("")` | `""` | Yes | Reject the explicitly empty version. |

The output prints the equal getter values and different `present` flags.
Inspect the optional pointer when presence matters. Ordinary non-optional
proto3 strings, such as `ContactInput.Email`, do not retain that distinction.

This local example only constructs and encodes messages. Encoding does not
enforce contact rules: an empty name can be encoded, while the service in
stage 2 rejects it. Syntax and business validation are separate steps.

## 4. Select the field to change with a mask

A **field mask** lists which fields a partial update selects. The input carries
the new values, and the mask tells the service which ones to apply. Consider
this request's meaning when sent to the later unary service:

```text
Current contact: name="Ana", email="old@example.com", phone="555-0100"
Update input:    email=""
Update mask:     ["email"]
Intended result: name="Ana", email="", phone="555-0100"
```

An empty email is the Go zero value, so it may be absent from the encoded input.
The mask still says to change email. Without that selection, the service could
not tell this clearing request from an unselected empty field. The local demo
shows the request as ProtoJSON, where the mask is displayed as `"email"`.
Stage 2 implements its effect and validates the allowed mask paths.

## 5. Clone a message before independent edits

Assigning a message pointer to another variable gives both variables access to
the same message. `proto.Clone` creates an independent deep copy, including
nested message data. Follow the final example:

```text
Original message: name="Ana López"
Clone the message, then set the clone's name to "Edited copy"
Result: original name="Ana López", clone name="Edited copy"
```

Generated message structs contain runtime state. Pass pointers and use
`proto.Clone` for independent edits; an ordinary struct assignment is not a
deep clone. Synchronize access if goroutines share a message that can change.

## Validate and continue

```sh
go -C extensions/01-protobuf-and-grpc test -count=1 -timeout=30s ./01-protobuf
```

[The tests](main_test.go) also check a fixed binary fixture, unknown fields,
presence after encoding, masks, and cloning. After the five examples, read
[ownership and compatibility](../README.md#ownership-and-compatibility) and the
[generation instructions](../README.md#regenerate-reproducibly) for deeper detail.
Then continue to the [unary application](../02-unary/README.md) to send these
typed messages through a real client/server call.
