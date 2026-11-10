// Package rpcinfra supplies metadata, logging, and access control for stages 4 and 5.
package rpcinfra

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	reflectionalpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
)

// Role selects this lab's reader/writer method permissions.
type Role string

const (
	Reader Role = "reader"
	Writer Role = "writer"
)

// Identity describes the client whose token the server checked. The server
// assigns its name (Subject) and allowed operations (Role); request metadata
// cannot choose a different identity or give itself more permissions.
type Identity struct {
	Subject string // Which teaching client authenticated.
	Role    Role   // Which operations that client may perform.
}
type identityKey struct{}

// IdentityFromContext reports the verified caller. It returns false before
// authentication and on the deliberately public health Check path.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}

// Policy is a small teaching credential store. Real deployments need managed
// identities, expiry, revocation, and rotation. Roles come from verified tokens,
// never from a client-supplied role header or request field.
type Policy struct{ readerHash, writerHash [sha256.Size]byte }

// NewPolicy binds two distinct tokens to the lab identities and stores hashes.
// Each token must contain 1-256 visible ASCII characters without whitespace.
func NewPolicy(readerToken, writerToken string) (*Policy, error) {
	if !validToken(readerToken) || !validToken(writerToken) || readerToken == writerToken {
		return nil, errors.New("two distinct nonempty ASCII bearer tokens without whitespace are required")
	}
	return &Policy{readerHash: sha256.Sum256([]byte(readerToken)), writerHash: sha256.Sum256([]byte(writerToken))}, nil
}

func validToken(token string) bool {
	if len(token) == 0 || len(token) > 256 {
		return false
	}
	for _, c := range token {
		if c < '!' || c > '~' {
			return false
		}
	}
	return true
}

func (p *Policy) authorize(ctx context.Context, method string) (context.Context, error) {
	// Only unary Check is public, over TLS. Watch and List require a token.
	if method == healthv1.Health_Check_FullMethodName {
		return ctx, nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 {
		return nil, status.Error(codes.Unauthenticated, "one bearer credential is required")
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || !validToken(token) {
		return nil, status.Error(codes.Unauthenticated, "invalid bearer credential")
	}
	hash := sha256.Sum256([]byte(token))
	// Hash first so comparisons have a fixed length; retain neither raw token
	// nor authorization metadata in the identity placed in the context.
	isReader := subtle.ConstantTimeCompare(hash[:], p.readerHash[:])
	isWriter := subtle.ConstantTimeCompare(hash[:], p.writerHash[:])
	var identity Identity
	switch {
	case isWriter == 1:
		identity = Identity{Subject: "writer-client", Role: Writer}
	case isReader == 1:
		identity = Identity{Subject: "reader-client", Role: Reader}
	default:
		return nil, status.Error(codes.Unauthenticated, "invalid bearer credential")
	}
	allowed := false
	switch method {
	case contactsv1.ContactService_GetContact_FullMethodName,
		contactsv1.ContactService_ListContacts_FullMethodName,
		contactsv1.ContactService_StreamContacts_FullMethodName,
		contactsv1.ContactService_LookupContacts_FullMethodName,
		healthv1.Health_List_FullMethodName, healthv1.Health_Watch_FullMethodName:
		allowed = true
	case contactsv1.ContactService_CreateContact_FullMethodName,
		contactsv1.ContactService_ReplaceContact_FullMethodName,
		contactsv1.ContactService_UpdateContact_FullMethodName,
		contactsv1.ContactService_DeleteContact_FullMethodName,
		contactsv1.ContactService_ImportContacts_FullMethodName,
		reflectionv1.ServerReflection_ServerReflectionInfo_FullMethodName,
		reflectionalpha.ServerReflection_ServerReflectionInfo_FullMethodName:
		allowed = identity.Role == Writer
	}
	// A future RPC must be assigned explicitly. An authenticated writer does
	// not automatically gain access to an unclassified method.
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "role does not allow this RPC")
	}
	return context.WithValue(ctx, identityKey{}, identity), nil
}

// Unary checks who is calling (authentication), then whether that caller may
// use this method (authorization). Only an allowed call reaches next.
func (p *Policy) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		ctx, err := p.authorize(ctx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		// Returning above stops the request. Calling next lets it reach the
		// following interceptor or the actual RPC handler.
		return next(ctx, request)
	}
}

type contextStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s contextStream) Context() context.Context { return s.ctx }

// Stream checks identity/permissions once when the RPC starts. Its handler then
// receives the verified identity through the stream's context. Individual
// messages do not repeat this check, and an open stream is not revoked here.
func (p *Policy) Stream() grpc.StreamServerInterceptor {
	return func(service any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		ctx, err := p.authorize(stream.Context(), info.FullMethod)
		if err != nil {
			return err
		}
		// The wrapper replaces only Context(), so the handler can read the
		// verified identity while Send/Recv still use the original stream.
		return next(service, contextStream{ServerStream: stream, ctx: ctx})
	}
}

// BearerToken uses gRPC call credentials, rather than manually attaching a
// secret in every example. gRPC refuses to use these credentials over plaintext.
type BearerToken string

func (token BearerToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	if !validToken(string(token)) {
		return nil, status.Error(codes.Unauthenticated, "a usable bearer token is required")
	}
	return map[string]string{"authorization": "Bearer " + string(token)}, nil
}

func (BearerToken) RequireTransportSecurity() bool { return true }
