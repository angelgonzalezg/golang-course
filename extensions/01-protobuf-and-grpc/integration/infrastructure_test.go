//go:build integration

package integration_test

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/postgresapp"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/sql-lab/contactpg"
	"go.opentelemetry.io/otel/trace"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

// Check the new composition, not just isolated helpers: TLS 1.3, public health,
// authorized reflection, size limits, and owned metrics/client-server spans.
func testInfrastructure(t *testing.T, db *sql.DB, _ string) {
	store := contactpg.NewStore(db)
	h := newEndpoint(t, store, false, nil)
	anonymous, _ := h.client(t, "")
	var remote peer.Peer
	check, err := healthv1.NewHealthClient(anonymous).Check(t.Context(), &healthv1.HealthCheckRequest{Service: rpcpostgres.ServiceName}, grpc.Peer(&remote))
	if err != nil || check.GetStatus() != healthv1.HealthCheckResponse_SERVING {
		t.Fatalf("health=%v,%v", check, err)
	}
	info, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || info.State.Version != tls.VersionTLS13 || len(info.State.VerifiedChains) == 0 {
		t.Fatal("connection did not verify TLS 1.3")
	}
	_, reader := h.client(t, h.reader)
	if _, err := reader.ListContacts(t.Context(), &contactsv1.ListContactsRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		connection *grpc.ClientConn
		allowed    bool
	}{{"writer", h.connection, true}, {"reader", func() *grpc.ClientConn { connection, _ := h.client(t, h.reader); return connection }(), false}} {
		t.Run(test.name+" reflection", func(t *testing.T) {
			stream, err := reflectionv1.NewServerReflectionClient(test.connection).ServerReflectionInfo(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if err := stream.Send(&reflectionv1.ServerReflectionRequest{MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{ListServices: ""}}); err != nil && !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			reply, err := stream.Recv()
			if !test.allowed {
				if status.Code(err) != codes.PermissionDenied {
					t.Fatalf("reader reflection=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, service := range reply.GetListServicesResponse().GetService() {
				found = found || service.GetName() == rpcpostgres.ServiceName
			}
			if !found {
				t.Fatal("contacts absent from reflection")
			}
			if err := stream.CloseSend(); err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
				t.Fatalf("reflection completion=%v", err)
			}
		})
	}
	// A payload may fail the client or server byte budget before domain validation.
	oversized := &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: strings.Repeat("x", 2048)}}
	for _, options := range [][]grpc.CallOption{nil, {grpc.MaxCallSendMsgSize(4096)}} {
		if _, err := h.api.CreateContact(keyed(t.Context(), "too-large"), oversized, options...); status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("message boundary=%v", err)
		}
	}
	if _, _, err := invokeCreate(t.Context(), h.api, "telemetry", &contactsv1.ContactInput{Name: "Telemetry contact"}); err != nil {
		t.Fatal(err)
	}
	if err := h.close(); err != nil {
		t.Fatal(err)
	}
	metrics, err := h.telemetry.Collect(t.Context())
	if err != nil || len(metrics.ScopeMetrics) == 0 {
		t.Fatalf("metrics missing: %v", err)
	}
	spans, _ := h.telemetry.Spans.Snapshot()
	client, server := false, false
	for _, span := range spans {
		client = client || span.Kind == trace.SpanKindClient
		server = server || span.Kind == trace.SpanKindServer
	}
	if !client || !server {
		t.Fatalf("missing client/server spans: %v", spans)
	}
}

// Run the complete finite lesson beside an unrelated retained operation. Demo
// cleanup must remove only its own random keys/names and leave this replay intact.
func testDemo(t *testing.T, db *sql.DB, dsn string) {
	store := contactpg.NewStore(db)
	before, err := store.CreateOnce(t.Context(), "writer-client", "retained", contacts.Input{Name: "Keep this row"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := postgresapp.RunDemo(t.Context(), dsn, &output); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"two attempts, one committed creation", "New server/pool", "partial import", "Deleted contact stays absent", "OpenTelemetry spans="} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("demo output missing %q", text)
		}
	}
	again, err := store.CreateOnce(t.Context(), "writer-client", "retained", contacts.Input{Name: "Keep this row"})
	if err != nil || !again.Replayed || again.Contact != before.Contact {
		t.Fatalf("unrelated result changed: %+v,%v", again, err)
	}
	assertCounts(t, db, 1, 1)
}
