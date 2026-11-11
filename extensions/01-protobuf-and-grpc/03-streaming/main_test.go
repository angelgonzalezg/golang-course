package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Run all three streaming shapes over loopback TCP. Check successful summaries,
// per-item failures, final completion, and cancellation of an active lookup.
func TestFiniteStreamingDemoOverLocalTCP(t *testing.T) {
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runDemo(ctx, &output); err != nil {
		t.Fatalf("demo: %v\n%s", err, output.String())
	}
	for _, expected := range []string{
		"ImportContacts -> OK: created=3",
		"StreamContacts -> EOF / OK: received=2",
		"Invalid stream query -> InvalidArgument",
		"LOOKUP_FAILURE_CODE_NOT_FOUND",
		"LOOKUP_FAILURE_CODE_INVALID_ARGUMENT",
		"LookupContacts -> EOF / OK: replies=4",
		"Active lookup cancellation -> Canceled",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("demo omitted %q", expected)
		}
	}
}

type lookupClient struct {
	contactsv1.ContactServiceClient
	open func(context.Context) grpc.BidiStreamingClient[contactsv1.LookupContactsRequest, contactsv1.LookupContactsResponse]
}

func (c lookupClient) LookupContacts(ctx context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[contactsv1.LookupContactsRequest, contactsv1.LookupContactsResponse], error) {
	return c.open(ctx), nil
}

// blockedLookup makes Recv fail only after Send has started waiting. These
// signals establish the order explicitly, without a sleep-based timing guess.
type blockedLookup struct {
	grpc.ClientStream
	ctx     context.Context
	entered chan struct{}
	exited  chan struct{}
}

func (s blockedLookup) Send(*contactsv1.LookupContactsRequest) error {
	close(s.entered)
	<-s.ctx.Done()
	defer close(s.exited)
	return status.FromContextError(s.ctx.Err()).Err()
}
func (s blockedLookup) Recv() (*contactsv1.LookupContactsResponse, error) {
	<-s.entered
	return nil, status.Error(codes.Internal, "stream failed while sending")
}

// Make Recv fail while Send is blocked. The client must preserve that failure,
// cancel the stream, and wait for its sender to exit before returning.
func TestBidiClientCancelsAndJoinsBlockedSenderOnReceiveFailure(t *testing.T) {
	exited := make(chan struct{})
	client := lookupClient{open: func(ctx context.Context) grpc.BidiStreamingClient[contactsv1.LookupContactsRequest, contactsv1.LookupContactsResponse] {
		return blockedLookup{ctx: ctx, entered: make(chan struct{}), exited: exited}
	}}
	err := lookupContacts(t.Context(), client, []*contactsv1.LookupContactsRequest{{RequestId: 1, Id: 1}}, io.Discard)
	if status.Code(err) != codes.Internal {
		t.Fatalf("receive error lost its status: %v", err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("client returned without joining its blocked sender")
	}
}

type importClient struct {
	contactsv1.ContactServiceClient
}
type rejectedImport struct{ grpc.ClientStream }

func (importClient) ImportContacts(context.Context, ...grpc.CallOption) (grpc.ClientStreamingClient[contactsv1.CreateContactRequest, contactsv1.ImportContactsResponse], error) {
	return rejectedImport{}, nil
}
func (rejectedImport) Send(*contactsv1.CreateContactRequest) error { return io.EOF }
func (rejectedImport) CloseAndRecv() (*contactsv1.ImportContactsResponse, error) {
	return nil, status.Error(codes.ResourceExhausted, "batch rejected")
}

// Make Send return EOF before the import finishes. The client must still read
// the terminal ResourceExhausted status instead of treating EOF as success.
func TestImportClientReadsTerminalStatusAfterSendEOF(t *testing.T) {
	response, err := importBatch(t.Context(), importClient{}, []*contactsv1.CreateContactRequest{{Input: &contactsv1.ContactInput{Name: "Ana"}}})
	if status.Code(err) != codes.ResourceExhausted || response != nil {
		t.Fatalf("Send EOF was confused with successful completion: %v, %v", response, err)
	}
}

// startupWriter signals the server's actual announcement. A channel handshake
// supplies the address before the test starts either repeated client run.
type startupWriter struct{ started chan string }

func (w startupWriter) Write(data []byte) (int, error) {
	w.started <- string(data)
	return len(data), nil
}

// Run two clients against one server to check reusable standalone operation.
// After cancellation, both normal completion and test cleanup must join the server.
func TestStandaloneStreamingServerAndRepeatedClient(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writer := startupWriter{started: make(chan string, 1)}
	done := make(chan struct{})
	var serverErr error
	go func() {
		defer close(done)
		serverErr = runServer(ctx, "127.0.0.1:0", writer)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
			if serverErr != nil {
				t.Error(serverErr)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not join shutdown work")
		}
	})
	select {
	case announcement := <-writer.started:
		line, _, _ := strings.Cut(announcement, "\n")
		target, ok := strings.CutPrefix(line, "Streaming contacts server: ")
		if !ok {
			t.Fatalf("unexpected announcement: %q", announcement)
		}
		for range 2 {
			if err := runClient(t.Context(), target, io.Discard); err != nil {
				t.Fatal(err)
			}
		}
	case <-done:
		t.Fatalf("server exited before startup: %v", serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	cancel()
	select {
	case <-done:
		if serverErr != nil {
			t.Fatal(serverErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not join shutdown work")
	}
}
