package main

import (
	"context"
	"testing"
)

func TestRequestIDAccessorsPreserveScopeAndKeyTypes(t *testing.T) {
	root := context.Background()
	if id, ok := requestIDFrom(root); ok || id != "" {
		t.Fatalf("missing metadata returned (%q, %t)", id, ok)
	}
	parent := withRequestID(root, "req-parent")
	child := withRequestID(parent, "req-child")
	if id, ok := requestIDFrom(parent); !ok || id != "req-parent" {
		t.Fatal("child metadata changed the parent")
	}
	if id, ok := requestIDFrom(child); !ok || id != "req-child" {
		t.Fatal("child metadata was not selected")
	}
	type unrelatedKey struct{}
	other := context.WithValue(parent, unrelatedKey{}, "unrelated value")
	if id, ok := requestIDFrom(other); !ok || id != "req-parent" {
		t.Fatal("unrelated key type collided with request metadata")
	}
	for _, ctx := range []context.Context{
		withRequestID(root, ""),
		context.WithValue(root, requestIDKey{}, 42),
	} {
		if id, ok := requestIDFrom(ctx); ok || id != "" {
			t.Fatalf("empty or wrong-typed metadata returned (%q, %t)", id, ok)
		}
	}
}
