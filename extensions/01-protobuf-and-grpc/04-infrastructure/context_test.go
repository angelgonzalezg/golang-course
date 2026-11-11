package main

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type observation struct {
	identity    rpcinfra.Identity
	identified  bool
	id          string
	deadline    bool
	verifiedTLS bool
}

// observeContext captures what the handler actually receives after middleware,
// including TLS peer state rather than a client-supplied claim about security.
func observeContext(ctx context.Context) observation {
	identity, identified := rpcinfra.IdentityFromContext(ctx)
	_, deadline := ctx.Deadline()
	verifiedTLS := false
	if remote, ok := peer.FromContext(ctx); ok {
		if info, ok := remote.AuthInfo.(credentials.TLSInfo); ok {
			verifiedTLS = info.State.Version == tls.VersionTLS13
		}
	}
	return observation{identity, identified, rpcinfra.RequestID(ctx), deadline, verifiedTLS}
}

type contextService struct {
	contactsv1.ContactServiceServer
	observed chan observation
	finished chan error
}

func (s contextService) GetContact(ctx context.Context, request *contactsv1.GetContactRequest) (*contactsv1.Contact, error) {
	s.observed <- observeContext(ctx)
	return s.ContactServiceServer.GetContact(ctx, request)
}

func (s contextService) LookupContacts(stream grpc.BidiStreamingServer[contactsv1.LookupContactsRequest, contactsv1.LookupContactsResponse]) error {
	s.observed <- observeContext(stream.Context())
	<-stream.Context().Done()
	s.finished <- stream.Context().Err()
	return status.FromContextError(stream.Context().Err()).Err()
}

// Observe unary and streaming handler contexts after the interceptor chains.
// Verified identity, request ID, deadline, TLS 1.3, and stream cancellation must survive.
func TestInterceptorsPreserveIdentityMetadataTLSAndCancellation(t *testing.T) {
	observed := make(chan observation, 2)
	finished := make(chan error, 1)
	env := setup(t, false, func(base contactsv1.ContactServiceServer) contactsv1.ContactServiceServer {
		return contextService{ContactServiceServer: base, observed: observed, finished: finished}
	})
	client := contactsv1.NewContactServiceClient(env.connection(t, env.secrets.reader))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := client.GetContact(metadata.AppendToOutgoingContext(ctx, "x-request-id", "ctx-unary"), &contactsv1.GetContactRequest{Id: 1}); err != nil {
		t.Fatal(err)
	}
	streamContext, stop := context.WithCancel(metadata.AppendToOutgoingContext(ctx, "x-request-id", "ctx-stream"))
	defer stop()
	stream, err := client.LookupContacts(streamContext)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&contactsv1.LookupContactsRequest{RequestId: 1, Id: 1}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ctx-unary", "ctx-stream"} {
		select {
		case got := <-observed:
			if !got.identified || got.identity.Subject != "reader-client" || got.identity.Role != rpcinfra.Reader || got.id != id || !got.deadline || !got.verifiedTLS {
				t.Fatalf("context lost through interceptor chain: %+v", got)
			}
		case <-ctx.Done():
			t.Fatal("handler did not observe its context")
		}
	}
	stop()
	if _, err := stream.Recv(); status.Code(err) != codes.Canceled {
		t.Fatalf("client cancellation = %v", err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("server context = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("wrapped context did not propagate cancellation to the handler")
	}
}
