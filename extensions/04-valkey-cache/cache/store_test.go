package cache

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// Model the three PTTL states without conflating persistent keys and misses.
// Check conversion boundaries so a server duration cannot overflow Go's type.
func TestExpirationStates(t *testing.T) {
	for _, test := range []struct {
		ms      int64
		want    Expiration
		invalid bool
	}{
		{-2, Expiration{}, false}, {-1, Expiration{Exists: true}, false},
		{0, Expiration{Exists: true, HasTTL: true}, false},
		{1500, Expiration{Exists: true, HasTTL: true, Remaining: 1500 * time.Millisecond}, false},
		{-3, Expiration{}, true}, {math.MaxInt64, Expiration{}, true},
	} {
		got, err := expiration(test.ms)
		if test.invalid {
			if err == nil {
				t.Errorf("PTTL %d accepted", test.ms)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Errorf("PTTL %d = (%+v, %v)", test.ms, got, err)
		}
	}
}

// Invalid keys and TTLs fail before using a client. Key length is in bytes,
// whereas contact names use runes. No operation below can contact a server.
func TestInputValidation(t *testing.T) {
	store := &Store{}
	for _, key := range []string{"", "a\x00b", "a\xff", strings.Repeat("é", 101)} {
		if _, err := store.Get(t.Context(), key); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("key %q: %v", key, err)
		}
	}
	if err := validateKey(strings.Repeat("é", 100)); err != nil {
		t.Fatal(err)
	}
	for _, ttl := range []time.Duration{0, -time.Second, time.Nanosecond, time.Millisecond + 1} {
		if err := store.Set(t.Context(), "valid-key", "value", ttl); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("TTL %v: %v", ttl, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Get(ctx, "valid-key"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled get = %v", err)
	}
	if err := store.Set(ctx, "valid-key", "value", time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled set = %v", err)
	}
}

// Explicit configuration avoids implicit localhost defaults and URL query
// options that would obscure this lab's standalone connection behavior.
func TestOpenConfiguration(t *testing.T) {
	for _, uri := range []string{"", " ", "http://127.0.0.1:6380", "valkey:///0", "valkey://127.0.0.1:6380/-1", "valkey://127.0.0.1:6380/0?addr=other"} {
		client, err := Open(t.Context(), uri)
		if !errors.Is(err, ErrInvalidInput) || client != nil {
			t.Fatalf("invalid configuration accepted: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if client, err := Open(ctx, "valkey://127.0.0.1:6380/0"); client != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
}

// Context wrapping keeps the underlying wire/server cause available to callers.
func TestCommandErrorCauses(t *testing.T) {
	wire := errors.New("wire failure")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := commandError(ctx, wire)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, wire) {
		t.Fatalf("causes = %v", err)
	}
}
