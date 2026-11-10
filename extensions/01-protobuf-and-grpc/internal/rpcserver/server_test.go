package rpcserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type blockedService struct {
	contactsv1.UnimplementedContactServiceServer
	entered chan struct{}
	exited  chan struct{}
	release chan struct{}
}

type observedListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *observedListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return err
}

func (s *blockedService) wait(ctx context.Context) error {
	close(s.entered)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return status.FromContextError(ctx.Err()).Err()
	}
}

func (s *blockedService) GetContact(ctx context.Context, request *contactsv1.GetContactRequest) (*contactsv1.Contact, error) {
	defer close(s.exited)
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	return &contactsv1.Contact{Id: request.GetId(), Name: "Completed"}, nil
}

func (s *blockedService) StreamContacts(_ *contactsv1.ListContactsRequest, stream grpc.ServerStreamingServer[contactsv1.Contact]) error {
	defer close(s.exited)
	if err := s.wait(stream.Context()); err != nil {
		return err
	}
	return stream.Send(&contactsv1.Contact{Id: 1, Name: "Completed"})
}

func TestGracefulShutdownWaitsForAnActiveRPC(t *testing.T) {
	for _, tc := range []struct {
		name      string
		force     bool
		streaming bool
	}{
		{"unary/graceful", false, false},
		{"unary/forced fallback", true, false},
		{"streaming/graceful", false, true},
		{"streaming/forced fallback", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			force := tc.force
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			service := &blockedService{entered: make(chan struct{}), exited: make(chan struct{}), release: make(chan struct{})}
			var once sync.Once
			release := func() { once.Do(func() { close(service.release) }) }
			defer release()
			observed := &observedListener{Listener: listener, closed: make(chan struct{})}
			running := Start(observed, service)
			t.Cleanup(func() {
				release()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := running.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			requested := make(chan error, 1)
			go func() {
				client := contactsv1.NewContactServiceClient(connection)
				var result *contactsv1.Contact
				var err error
				if tc.streaming {
					stream, openErr := client.StreamContacts(ctx, &contactsv1.ListContactsRequest{})
					err = openErr
					if err == nil {
						result, err = stream.Recv()
						if err == nil {
							if _, terminal := stream.Recv(); terminal != io.EOF {
								err = fmt.Errorf("stream completion: got %v, want EOF", terminal)
							}
						}
					}
				} else {
					result, err = client.GetContact(ctx, &contactsv1.GetContactRequest{Id: 1})
				}
				if err == nil && result.GetName() != "Completed" {
					err = errors.New("active RPC returned wrong result")
				}
				requested <- err
			}()
			select {
			case <-service.entered:
			case err := <-requested:
				t.Fatalf("call ended before handler entered: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not start")
			}
			shutdownCtx, stop := context.WithCancel(context.Background())
			defer stop()
			if force {
				stop() // The fallback must cancel and join the active handler.
			}
			stopped := make(chan error, 1)
			go func() { stopped <- running.Shutdown(shutdownCtx) }()
			select {
			case <-observed.closed: // Shutdown closed the listener; a call can remain active.
			case <-time.After(5 * time.Second):
				t.Fatal("Serve did not stop")
			}
			if !force {
				select {
				case err := <-stopped:
					t.Fatalf("shutdown returned before active handler: %v", err)
				default:
				}
				release()
			}
			err = <-stopped
			if force && !errors.Is(err, context.Canceled) || !force && err != nil {
				t.Fatalf("shutdown result = %v, force=%t", err, force)
			}
			select {
			case <-service.exited:
			default:
				t.Fatal("shutdown failed to join the handler")
			}
			err = <-requested
			if force && status.Code(err) == codes.OK || !force && err != nil {
				t.Fatalf("active call result = %v, force=%t", err, force)
			}
		})
	}
}
