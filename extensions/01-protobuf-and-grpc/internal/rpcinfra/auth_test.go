package rpcinfra

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestPolicyRejectsAmbiguousOrMissingConfiguration(t *testing.T) {
	for _, pair := range [][2]string{{"", "writer"}, {"reader", ""}, {"same", "same"}, {"has space", "writer"}, {"reader", "non-ASCII-é"}} {
		if _, err := NewPolicy(pair[0], pair[1]); err == nil {
			t.Fatal("invalid credential configuration accepted")
		}
	}
}

func TestNewMethodsRequireExplicitAuthorization(t *testing.T) {
	policy, err := NewPolicy("reader", "writer")
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer writer"))
	called := false
	_, err = policy.Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/contacts.v1.ContactService/FutureMethod"}, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if called || status.Code(err) != codes.PermissionDenied {
		t.Fatalf("future method was not denied before dispatch: %v, called=%t", err, called)
	}
}
