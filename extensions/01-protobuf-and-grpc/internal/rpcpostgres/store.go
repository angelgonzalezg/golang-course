// Package rpcpostgres connects the existing gRPC service to durable creation.
// PostgreSQL owns the transaction; this adapter supplies verified caller/key
// metadata without making the SQL store depend on gRPC or authentication types.
package rpcpostgres

import (
	"context"
	"strconv"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/grpc-lab/internal/rpcinfra"
	"example.com/course/grpc-lab/internal/rpcresilience"
	"example.com/course/grpc-lab/rpcapi"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ReplayedTrailer reports how this invocation obtained its successful result.
// It is transport metadata, not a field in the saved Contact response.
const ReplayedTrailer = "x-idempotency-replayed"

// Backend adds atomic keyed creation to the ordinary RPC storage interface.
// The SQL implementation also serves REST with the same domain rules.
type Backend interface {
	rpcapi.Store
	CreateOnce(context.Context, string, string, contacts.Input) (contacts.CreationResult, error)
}

type creationScope struct{ subject, key string }
type scopeKey struct{}

// Store delegates CRUD and paging to Backend. Its Create override chooses
// durable unary creation only when the server supplied a verified scope.
type Store struct{ Backend }

// NewStore borrows its backend and owns no pool, connection, or cache.
func NewStore(backend Backend) *Store { return &Store{Backend: backend} }

// Create runs inside rpcapi's normal input-validation/status-mapping flow.
// Streaming imports have no unary scope and retain their per-item commits.
func (s *Store) Create(ctx context.Context, input contacts.Input) (contacts.Contact, error) {
	scope, ok := ctx.Value(scopeKey{}).(creationScope)
	if !ok {
		return s.Backend.Create(ctx, input)
	}
	result, err := s.Backend.CreateOnce(ctx, scope.subject, scope.key, input)
	if err != nil {
		return contacts.Contact{}, err
	}
	// The result is already committed. A later transport failure cannot undo it;
	// retrying the same key/data recovers that saved success from the database.
	if err := grpc.SetTrailer(ctx, metadata.Pairs(ReplayedTrailer, strconv.FormatBool(result.Replayed))); err != nil {
		return contacts.Contact{}, err
	}
	return result.Contact, nil
}

// Unary requires exactly one key on CreateContact, rejects keys on other unary
// methods, and derives identity ONLY from the preceding authorization policy.
// It reserves nothing in memory and invokes the ordinary RPC handler once.
func Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		keys := md.Get(rpcresilience.KeyHeader)
		if info.FullMethod != contactsv1.ContactService_CreateContact_FullMethodName {
			if len(keys) != 0 {
				return nil, status.Error(codes.InvalidArgument, "idempotency keys apply only to unary CreateContact")
			}
			return next(ctx, request)
		}
		if len(keys) != 1 || contacts.ValidateIdempotencyKey(keys[0]) != nil {
			return nil, status.Error(codes.InvalidArgument, "one 1-64 character ASCII idempotency key is required")
		}
		identity, ok := rpcinfra.IdentityFromContext(ctx)
		if !ok || identity.Subject == "" {
			return nil, status.Error(codes.Unauthenticated, "authenticate before applying idempotency")
		}
		// The private context key cannot be selected by incoming metadata. The
		// SQL store validates this application's subject/key shape independently.
		ctx = context.WithValue(ctx, scopeKey{}, creationScope{identity.Subject, keys[0]})
		return next(ctx, request)
	}
}
