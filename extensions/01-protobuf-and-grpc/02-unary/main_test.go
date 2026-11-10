package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestFiniteDemoOverLocalTCP(t *testing.T) {
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runDemo(ctx, &output); err != nil {
		t.Fatalf("demo failed: %v\n%s", err, output.String())
	}
	for _, expected := range []string{"Duplicate name -> AlreadyExists", "Stale update -> Aborted", "Invalid ID -> InvalidArgument", "Pre-canceled call -> Canceled", "Expired deadline -> DeadlineExceeded", "Deleted contact -> NotFound"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("demo omitted %q", expected)
		}
	}
}

func TestLazyChannelAndUnexpectedTransportError(t *testing.T) {
	dialed := make(chan struct{}, 1)
	connection, err := grpc.NewClient("passthrough:///offline-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			select {
			case dialed <- struct{}{}:
			default:
			}
			return nil, errors.New("test transport is unavailable")
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	select {
	case <-dialed:
		t.Fatal("channel construction unexpectedly performed connection I/O")
	default:
	}
	var output bytes.Buffer
	client := contactsv1.NewContactServiceClient(connection)
	_, err = call(t.Context(), &output, "offline call", codes.OK, func(ctx context.Context) (*contactsv1.Contact, error) {
		return client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1})
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("unexpected transport error must retain its gRPC status through wrapping: %v", err)
	}
}

type startupWriter struct{ started chan string }

func (w startupWriter) Write(data []byte) (int, error) {
	w.started <- string(data)
	return len(data), nil
}

func TestStandaloneServerAndClient(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writer := startupWriter{started: make(chan string, 1)}
	done := make(chan error, 1)
	go func() { done <- runServer(ctx, "127.0.0.1:0", writer) }()
	var announcement string
	select {
	case announcement = <-writer.started:
	case err := <-done:
		t.Fatalf("server exited before startup: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	line, _, _ := strings.Cut(announcement, "\n")
	target, ok := strings.CutPrefix(line, "Unary contacts server: ")
	if !ok {
		t.Fatalf("unexpected server announcement: %q", announcement)
	}
	var output bytes.Buffer
	clientCtx, cancelClient := context.WithTimeout(t.Context(), 10*time.Second)
	err := runClient(clientCtx, target, &output)
	cancelClient()
	if err != nil {
		t.Fatalf("standalone client: %v\n%s", err, output.String())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("standalone server did not finish shutdown")
	}
}
