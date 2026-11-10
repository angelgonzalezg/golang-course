package rpcserver

import (
	"context"
	"errors"
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

func (s *blockedService) GetContact(ctx context.Context, request *contactsv1.GetContactRequest) (*contactsv1.Contact, error) {
	close(s.entered)
	defer close(s.exited)
	select {
	case <-s.release:
		return &contactsv1.Contact{Id: request.GetId(), Name: "Completed"}, nil
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

func TestGracefulShutdownWaitsForAnActiveRPC(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "graceful"
		if force {
			name = "forced fallback"
		}
		t.Run(name, func(t *testing.T) {
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
				result, err := contactsv1.NewContactServiceClient(connection).GetContact(ctx, &contactsv1.GetContactRequest{Id: 1})
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
