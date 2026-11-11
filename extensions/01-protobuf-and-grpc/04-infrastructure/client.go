package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// openConnection configures server verification and, optionally, a caller token.
// NewClient is lazy: successful construction does not prove TLS or an RPC works.
// The returned channel belongs to the caller and can be reused across calls.
func openConnection(target string, transport *tls.Config, token string) (*grpc.ClientConn, error) {
	options := []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(transport.Clone()))}
	if token != "" {
		// PerRPCCredentials attaches a token to each RPC, separately from TLS.
		// BearerToken requires a secure transport before gRPC will send it.
		options = append(options, grpc.WithPerRPCCredentials(rpcinfra.BearerToken(token)))
	}
	return grpc.NewClient(target, options...)
}

// expectCode treats a planned rejection as a successful learning check. It does
// not change the RPC's status; an unexpected outcome still stops the demo.
func expectCode(label string, expected codes.Code, err error, out io.Writer) error {
	if status.Code(err) != expected {
		if err != nil {
			return fmt.Errorf("%s: want %s: %w", label, expected, err)
		}
		return fmt.Errorf("%s: got OK, want %s", label, expected)
	}
	fmt.Fprintf(out, "%s -> %s\n", label, expected)
	return nil
}

// runClient compares anonymous, reader, and writer calls on the same TLS server,
// then inspects metadata, health, and optionally the exposed service description.
func runClient(parent context.Context, target string, transport *tls.Config, secrets tokens, reflection bool, out io.Writer) (err error) {
	// All calls, including long-lived streams, share this finite run's budget.
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	// Each channel represents one identity and is reused across its calls.
	anonymous, err := openConnection(target, transport, "")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, anonymous.Close()) }()
	reader, err := openConnection(target, transport, secrets.reader)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	writer, err := openConnection(target, transport, secrets.writer)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, writer.Close()) }()
	readClient := contactsv1.NewContactServiceClient(reader)
	writeClient := contactsv1.NewContactServiceClient(writer)

	fmt.Fprintln(out, "== Verified TLS and authentication ==")
	// This lab explicitly permits anonymous Health Check over TLS. That policy
	// does not grant anonymous access to the contact methods checked next.
	ready, err := healthv1.NewHealthClient(anonymous).Check(ctx, &healthv1.HealthCheckRequest{Service: contactService})
	if err != nil {
		return err
	}
	if ready.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		return fmt.Errorf("service is not ready: %v", ready)
	}
	fmt.Fprintln(out, "Health Check over verified TLS (no token) -> SERVING")
	_, err = contactsv1.NewContactServiceClient(anonymous).GetContact(ctx, &contactsv1.GetContactRequest{Id: 1})
	if err := expectCode("Missing token", codes.Unauthenticated, err, out); err != nil {
		return err
	}
	_, err = contactsv1.NewContactServiceClient(anonymous).GetContact(ctx, &contactsv1.GetContactRequest{Id: 1},
		grpc.PerRPCCredentials(rpcinfra.BearerToken("unrecognized-demo-token")))
	if err := expectCode("Invalid token", codes.Unauthenticated, err, out); err != nil {
		return err
	}
	// A recognized reader has an identity but lacks write permission. Compare
	// PermissionDenied here with Unauthenticated for the unrecognized callers.
	_, err = readClient.CreateContact(ctx, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "Denied"}})
	if err := expectCode("Reader create", codes.PermissionDenied, err, out); err != nil {
		return err
	}

	fmt.Fprintln(out, "\n== Unary metadata: request ID, response headers, and trailers ==")
	suffix := " " + strconv.FormatInt(time.Now().UnixNano(), 36)
	// Metadata labels the call; the message below carries the contact data.
	// The interceptor echoes our request ID so we can connect request and reply.
	createContext := metadata.AppendToOutgoingContext(ctx, "x-request-id", "infra-create")
	var headers, trailers metadata.MD
	created, err := writeClient.CreateContact(createContext, &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: "TLS Contact" + suffix}},
		grpc.Header(&headers), grpc.Trailer(&trailers))
	if err != nil {
		return err
	}
	// A completed unary call has delivered its final status and trailers.
	if err := printMetadata("Writer create", headers, trailers, "infra-create", "unary", out); err != nil {
		return err
	}
	if _, err := readClient.GetContact(ctx, &contactsv1.GetContactRequest{Id: created.GetId()}); err != nil {
		return err
	}
	fmt.Fprintln(out, "Reader get -> OK")

	fmt.Fprintln(out, "\n== Streaming interceptors use the same access policy ==")
	streamContext := metadata.AppendToOutgoingContext(ctx, "x-request-id", "infra-stream")
	stream, err := readClient.StreamContacts(streamContext, &contactsv1.ListContactsRequest{NameContains: suffix})
	if err != nil {
		return err
	}
	count := 0
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
	}
	if count != 1 {
		return fmt.Errorf("reader stream returned %d contacts, want 1", count)
	}
	// Read trailers only after terminal EOF/error; opening a stream did not
	// establish that it succeeded or that all response metadata was available.
	headers, err = stream.Header()
	if err != nil {
		return err
	}
	if err := printMetadata("Reader stream EOF / OK", headers, stream.Trailer(), "infra-stream", "stream", out); err != nil {
		return err
	}
	// Imports write data, so a reader is rejected. An apparently open client
	// stream still needs its final result to reveal that server-side rejection.
	denied, err := readClient.ImportContacts(ctx)
	if err == nil {
		_, err = denied.CloseAndRecv() // An opened stream can still be rejected.
	}
	if err := expectCode("Reader import", codes.PermissionDenied, err, out); err != nil {
		return err
	}
	imported, err := importOne(ctx, writeClient, "Imported over TLS"+suffix)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Writer import -> OK")
	if err := lookupOne(ctx, readClient, imported); err != nil {
		return err
	}
	fmt.Fprintln(out, "Reader bidirectional lookup -> OK")
	if err := watchHealth(ctx, reader, out); err != nil {
		return err
	}
	if reflection {
		if err := inspectReflection(ctx, writer, out); err != nil {
			return err
		}
	}
	// Delete only this run's contacts; other clients may share the server.
	for _, id := range []int64{created.GetId(), imported} {
		if _, err := writeClient.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: id}); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "Example calls completed; owned TLS channels close on return.")
	return nil
}

// printMetadata checks this lesson's custom fields. x-rpc-kind is explanatory
// metadata, not a replacement for gRPC's standard final status.
func printMetadata(label string, headers, trailers metadata.MD, expectedID, expectedKind string, out io.Writer) error {
	ids, kinds := headers.Get("x-request-id"), trailers.Get("x-rpc-kind")
	if len(ids) != 1 || ids[0] != expectedID || len(kinds) != 1 || kinds[0] != expectedKind {
		return fmt.Errorf("%s: unexpected lesson metadata", label)
	}
	fmt.Fprintf(out, "%s: header x-request-id=%s; trailer x-rpc-kind=%s\n", label, ids[0], kinds[0])
	return nil
}

// importOne is a one-item client stream: send, finish that direction, then read
// the summary. A Send error of EOF still requires reading the final status.
func importOne(ctx context.Context, client contactsv1.ContactServiceClient, name string) (int64, error) {
	stream, err := client.ImportContacts(ctx)
	if err != nil {
		return 0, err
	}
	if err := stream.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: name}}); err != nil && err != io.EOF {
		return 0, err
	}
	summary, err := stream.CloseAndRecv()
	if err != nil {
		return 0, err
	}
	if summary.GetCreatedCount() != 1 || len(summary.GetIds()) != 1 {
		return 0, fmt.Errorf("unexpected import summary: %v", summary)
	}
	return summary.Ids[0], nil
}

// lookupOne sends one request, half-closes, and verifies both its reply and EOF.
// Half-closing alone would leave the RPC's final outcome unread.
func lookupOne(ctx context.Context, client contactsv1.ContactServiceClient, id int64) error {
	stream, err := client.LookupContacts(ctx)
	if err != nil {
		return err
	}
	// One bounded request cannot fill both transport directions. For many
	// requests, retain stage 3's concurrent sender/receiver pattern.
	if err := stream.Send(&contactsv1.LookupContactsRequest{RequestId: 1, Id: id}); err != nil {
		return err
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	response, err := stream.Recv()
	if err != nil {
		return err
	}
	if response.GetRequestId() != 1 || response.GetContact().GetId() != id {
		return fmt.Errorf("unexpected lookup: %v", response)
	}
	if _, err := stream.Recv(); err != io.EOF {
		return fmt.Errorf("lookup completion: want EOF, got %v", err)
	}
	return nil
}

// watchHealth reads an initial readiness update, then ends its own subscription.
// SERVING is a status message; it does not make this long-lived Watch finish.
func watchHealth(parent context.Context, connection *grpc.ClientConn, out io.Writer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stream, err := healthv1.NewHealthClient(connection).Watch(ctx, &healthv1.HealthCheckRequest{Service: contactService})
	if err != nil {
		return err
	}
	response, err := stream.Recv()
	if err != nil {
		return err
	}
	if response.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		return fmt.Errorf("unexpected health state: %v", response)
	}
	cancel() // Watch is long-lived; the client owns ending its subscription.
	_, err = stream.Recv()
	return expectCode("Health Watch: SERVING, then cancellation", codes.Canceled, err, out)
}

// inspectReflection asks the server for its API description, as grpcurl would.
// This is gRPC service discovery, not runtime inspection with Go's reflect package.
func inspectReflection(ctx context.Context, connection *grpc.ClientConn, out io.Writer) error {
	stream, err := reflectionv1.NewServerReflectionClient(connection).ServerReflectionInfo(ctx)
	if err != nil {
		return err
	}
	// First discover the service name; then ask which .proto file defines it.
	if err := stream.Send(&reflectionv1.ServerReflectionRequest{MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{ListServices: ""}}); err != nil {
		return err
	}
	response, err := stream.Recv()
	if err != nil {
		return err
	}
	found := false
	for _, service := range response.GetListServicesResponse().GetService() {
		if service.GetName() == contactService {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("contact service absent from reflection: %v", response)
	}
	if err := stream.Send(&reflectionv1.ServerReflectionRequest{MessageRequest: &reflectionv1.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: contactService}}); err != nil {
		return err
	}
	response, err = stream.Recv()
	if err != nil {
		return err
	}
	// Descriptors are encoded protobuf messages describing messages and methods.
	// Decode the contact file and check the six unary plus three streaming RPCs.
	methods := 0
	for _, data := range response.GetFileDescriptorResponse().GetFileDescriptorProto() {
		file := new(descriptorpb.FileDescriptorProto)
		if err := proto.Unmarshal(data, file); err != nil {
			return err
		}
		if file.GetName() == "contacts/v1/contacts.proto" {
			for _, service := range file.GetService() {
				if service.GetName() == "ContactService" {
					methods = len(service.GetMethod())
				}
			}
		}
	}
	if methods != 9 {
		return fmt.Errorf("reflection exposed %d contact methods, want 9", methods)
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != io.EOF {
		return fmt.Errorf("reflection completion: want EOF, got %v", err)
	}
	fmt.Fprintf(out, "Reflection v1 -> %s (%d methods)\n", contactService, methods)
	return nil
}
