package postgresapp

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"example.com/course/grpc-lab/internal/rpcserver"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// session owns one server/pool/channel generation. Closing it before opening a
// new session proves that recovery is independent of those Go objects' lifetimes.
type session struct {
	db         *sql.DB
	server     *rpcserver.Running
	connection *grpc.ClientConn
	api        contactsv1.ContactServiceClient
}

func openSession(ctx context.Context, dsn string, serverTLS, clientTLS *tls.Config, token string, policy *rpcinfra.Policy, telemetry *rpcobserve.Telemetry, faults grpc.UnaryServerInterceptor) (*session, error) {
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		db.Close()
		return nil, err
	}
	server, err := rpcpostgres.Start(listener, contactpg.NewStore(db), rpcpostgres.Config{TLS: serverTLS, Policy: policy, Telemetry: telemetry, Reflection: true, Faults: faults})
	if err != nil {
		listener.Close()
		db.Close()
		return nil, err
	}
	connection, err := rpcpostgres.OpenConnection(listener.Addr().String(), clientTLS, token, telemetry)
	if err != nil {
		rpcpostgres.Shutdown(server)
		db.Close()
		return nil, err
	}
	return &session{db: db, server: server, connection: connection, api: contactsv1.NewContactServiceClient(connection)}, nil
}

// close first finishes channel/RPC work, then stops the server before closing SQL.
func (s *session) close() error {
	return errors.Join(s.connection.Close(), rpcpostgres.Shutdown(s.server), s.db.Close())
}

// RunDemo follows durable creation through a controlled lost reply, a new
// server/pool, mutation, and deletion. Random names/keys isolate finite cleanup.
func RunDemo(parent context.Context, dsn string, out io.Writer) (result error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if _, err := contactpg.Migrate(dsn); err != nil {
		return err
	}
	telemetry := rpcobserve.New()
	defer func() { result = errors.Join(result, shutdownTelemetry(telemetry)) }()
	prefix := "grpc-demo-" + rand.Text()[:12] + "-"
	key := prefix + "create"
	defer func() { result = errors.Join(result, cleanupDemo(dsn, prefix, key)) }()
	material, err := labtls.Generate()
	if err != nil {
		return err
	}
	serverTLS, err := material.ServerConfig()
	if err != nil {
		return err
	}
	clientTLS, err := labtls.ClientConfig(material.CA)
	if err != nil {
		return err
	}
	reader, writer := rand.Text(), rand.Text()
	policy, err := rpcinfra.NewPolicy(reader, writer)
	if err != nil {
		return err
	}
	request := &contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: " " + prefix + "Contact ", Email: " original@example.com ", Phone: " 123 "}}
	var calls atomic.Int64
	var drop atomic.Bool
	drop.Store(true)
	// This hook is local configuration. It hides exactly one successful response
	// AFTER the database commit; no client metadata can enable the injection.
	faults := func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod != contactsv1.ContactService_CreateContact_FullMethodName {
			return next(ctx, request)
		}
		calls.Add(1)
		response, err := next(ctx, request)
		if err == nil && drop.CompareAndSwap(true, false) {
			return nil, status.Error(codes.Unavailable, "controlled lost reply after commit")
		}
		return response, err
	}
	var original *contactsv1.Contact
	err = func() (result error) {
		first, err := openSession(ctx, dsn, serverTLS, clientTLS, writer, policy, telemetry, faults)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, first.close()) }()
		fmt.Fprintln(out, "== 1. Commit, lose the reply, and retry the same operation ==")
		var trailer metadata.MD
		original, err = rpcresilience.NewClient(first.api).CreateOnce(ctx, key, request, grpc.Trailer(&trailer))
		if err != nil {
			return err
		}
		if calls.Load() != 2 || len(trailer.Get(rpcpostgres.ReplayedTrailer)) != 1 || trailer.Get(rpcpostgres.ReplayedTrailer)[0] != "true" {
			return errors.New("lost reply was not recovered by one replay")
		}
		var count int
		if err := first.db.QueryRowContext(ctx, "SELECT count(*) FROM sql_service_create_requests WHERE subject = $1 AND request_key = $2", "writer-client", key).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("retry did not retain exactly one result")
		}
		fmt.Fprintln(out, "Lost reply -> two attempts, one committed creation; replay=true.")
		return nil
	}()
	if err != nil {
		return err
	}
	// Both the original connection pool and server are now closed. This session
	// builds new Go objects and reads the same committed PostgreSQL records.
	err = func() (result error) {
		second, err := openSession(ctx, dsn, serverTLS, clientTLS, writer, policy, telemetry, nil)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, second.close()) }()
		fmt.Fprintln(out, "\n== 2. Recover through a new server and pool ==")
		client := rpcresilience.NewClient(second.api)
		recovered, err := client.CreateOnce(ctx, key, request)
		if err != nil || !proto.Equal(recovered, original) {
			return fmt.Errorf("new server did not recover the original result: %v", err)
		}
		fmt.Fprintln(out, "New server/pool -> original ID, fields, and version from PostgreSQL.")
		fmt.Fprintln(out, "\n== 3. Keep field masks, versions, and stream semantics ==")
		updated, err := second.api.UpdateContact(ctx, &contactsv1.UpdateContactRequest{
			Id: original.Id, Input: &contactsv1.ContactInput{Email: "changed"},
			UpdateMask:      &fieldmaskpb.FieldMask{Paths: []string{"email"}},
			ExpectedVersion: proto.String(original.Version),
		})
		if err != nil {
			return err
		}
		_, err = second.api.ReplaceContact(ctx, &contactsv1.ReplaceContactRequest{Id: original.Id, Input: &contactsv1.ContactInput{Name: prefix + "Stale"}, ExpectedVersion: proto.String(original.Version)})
		if status.Code(err) != codes.Aborted {
			return fmt.Errorf("stale version was not rejected: %v", err)
		}
		// Imports are not replayable batches. A valid first item stays committed
		// when the invalid second item ends the stream without a final summary.
		importer, err := second.api.ImportContacts(ctx)
		if err != nil {
			return err
		}
		if err := importer.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{Name: prefix + "Imported"}}); err != nil {
			return err
		}
		if err := importer.Send(&contactsv1.CreateContactRequest{Input: &contactsv1.ContactInput{}}); err != nil {
			return err
		}
		if _, err := importer.CloseAndRecv(); status.Code(err) != codes.InvalidArgument {
			return fmt.Errorf("invalid import ended with %v", err)
		}
		stream, err := second.api.StreamContacts(ctx, &contactsv1.ListContactsRequest{NameContains: prefix, Limit: 100})
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
		if count != 2 {
			return fmt.Errorf("stream has %d demo rows, want two after partial import", count)
		}
		fmt.Fprintln(out, "Stale version -> Aborted; partial import keeps its first item; stream ends at EOF.")
		fmt.Fprintln(out, "\n== 4. Replay the historical result after deletion ==")
		if _, err := second.api.DeleteContact(ctx, &contactsv1.DeleteContactRequest{Id: original.Id, ExpectedVersion: proto.String(updated.Version)}); err != nil {
			return err
		}
		replayed, err := client.CreateOnce(ctx, key, request)
		if err != nil || !proto.Equal(replayed, original) {
			return fmt.Errorf("historical creation changed: %v", err)
		}
		if _, err := second.api.GetContact(ctx, &contactsv1.GetContactRequest{Id: original.Id}); status.Code(err) != codes.NotFound {
			return fmt.Errorf("replay recreated the deleted contact: %v", err)
		}
		fmt.Fprintln(out, "Deleted contact stays absent; its original creation/version still replays.")
		return nil
	}()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\n== Owned OpenTelemetry metrics and bounded trace summaries ==")
	return telemetry.Report(ctx, out)
}

// cleanupDemo removes only this random run's exact key and prefixed names. It is
// finite-example housekeeping; standalone services retain their replay records.
func cleanupDemo(dsn, prefix, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	db, err := contactsql.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sql_service_contacts WHERE id IN
		(SELECT response_id FROM sql_service_create_requests WHERE subject = $1 AND request_key = $2)`, "writer-client", key); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sql_service_contacts WHERE substring(name FROM 1 FOR $1) = $2", len(prefix), []byte(prefix)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sql_service_create_requests WHERE subject = $1 AND request_key = $2", "writer-client", key); err != nil {
		return err
	}
	return tx.Commit()
}
