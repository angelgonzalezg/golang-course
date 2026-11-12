//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/labtls"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"google.golang.org/protobuf/proto"
)

type processReply struct {
	Contact  []byte
	Replayed string
}

// Discard a successful child's reply and retry in a different OS process. Each
// process owns fresh pools, server, TLS material, and credentials; the verified
// writer subject remains stable and PostgreSQL supplies the saved result.
func testProcessRestart(t *testing.T, db *sql.DB, dsn string) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := func(output io.Writer) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestPostgresProcessHelper$")
		command.Env = append(os.Environ(), "GRPC_POSTGRES_CHILD=1", "GRPC_POSTGRES_CHILD_DSN="+dsn)
		var stderr bytes.Buffer
		command.Stdout, command.Stderr = output, &stderr
		if err := command.Run(); err != nil {
			t.Fatalf("gRPC child failed: %v\n%s", err, stderr.String())
		}
	}
	// This models a lost response after commit, not actual packet-loss injection.
	run(io.Discard)
	var id int64
	if err := db.QueryRowContext(t.Context(), "SELECT id FROM sql_service_contacts WHERE name=$1", []byte("Process contact")).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	run(&output)
	var reply processReply
	if err := json.Unmarshal(output.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	var contact contactsv1.Contact
	if err := proto.Unmarshal(reply.Contact, &contact); err != nil {
		t.Fatal(err)
	}
	if reply.Replayed != "true" || contact.Id != id || contact.Name != "Process contact" || contact.Email != "original" || contact.Version == "" {
		t.Fatalf("new process recovered wrong result: %v, replay=%s", &contact, reply.Replayed)
	}
	assertCounts(t, db, 1, 1)
}

// The parent invokes this entry only with its child environment. Normal tagged
// runs return without creating another container or opening a database.
func TestPostgresProcessHelper(t *testing.T) {
	if os.Getenv("GRPC_POSTGRES_CHILD") != "1" {
		return
	}
	if err := runProcessChild(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Child-owned defers have completed; omit testing's PASS suffix from JSON.
	os.Exit(0)
}

// The child intentionally retains committed rows, while closing every channel,
// server, provider, and pool before exiting so the second process starts fresh.
func runProcessChild() (result error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	db, err := contactsql.Open(ctx, os.Getenv("GRPC_POSTGRES_CHILD_DSN"))
	if err != nil {
		return err
	}
	defer db.Close()
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
	policy, err := rpcinfra.NewPolicy("child-reader", "child-writer")
	if err != nil {
		return err
	}
	telemetry := rpcobserve.New()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, telemetry.Shutdown(cleanup))
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server, err := rpcpostgres.Start(listener, contactpg.NewStore(db), rpcpostgres.Config{TLS: serverTLS, Policy: policy, Telemetry: telemetry})
	if err != nil {
		listener.Close()
		return err
	}
	defer func() { result = errors.Join(result, rpcpostgres.Shutdown(server)) }()
	connection, err := rpcpostgres.OpenConnection(listener.Addr().String(), clientTLS, "child-writer", telemetry)
	if err != nil {
		return err
	}
	defer connection.Close()
	contact, trailer, err := invokeCreate(ctx, contactsv1.NewContactServiceClient(connection), "process-key", &contactsv1.ContactInput{Name: "Process contact", Email: "original"})
	if err != nil {
		return err
	}
	data, err := proto.Marshal(contact)
	if err != nil {
		return err
	}
	values := trailer.Get(rpcpostgres.ReplayedTrailer)
	if len(values) != 1 {
		return fmt.Errorf("child missing replay metadata")
	}
	encoded, err := json.Marshal(processReply{Contact: data, Replayed: values[0]})
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(encoded)
	return err
}
