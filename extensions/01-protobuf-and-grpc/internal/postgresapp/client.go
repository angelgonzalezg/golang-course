package postgresapp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcobserve"
	"example.com/course/grpc-lab/internal/rpcpostgres"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// CreateClient labels one intended operation with a caller-supplied stable key.
// Running it again after restarting the server recovers the same creation.
// It does not delete the contact or inspect the database from the client.
func CreateClient(ctx context.Context, target string, transport *tls.Config, token, key string, input *contactsv1.ContactInput, out io.Writer) (result error) {
	telemetry := rpcobserve.New()
	defer func() { result = errors.Join(result, shutdownTelemetry(telemetry)) }()
	connection, err := rpcpostgres.OpenConnection(target, transport, token, telemetry)
	if err != nil {
		return err
	}
	defer connection.Close()
	client := rpcresilience.NewClient(contactsv1.NewContactServiceClient(connection))
	var trailer metadata.MD
	contact, err := client.CreateOnce(ctx, key, &contactsv1.CreateContactRequest{Input: input}, grpc.Trailer(&trailer))
	if err != nil {
		return err
	}
	replayed := trailer.Get(rpcpostgres.ReplayedTrailer)
	if len(replayed) != 1 {
		return errors.New("server did not report the durable creation outcome")
	}
	fmt.Fprintf(out, "CreateOnce -> id=%d, name=%q, version=%s, replay=%s\n", contact.GetId(), contact.GetName(), contact.GetVersion(), replayed[0])
	return nil
}
