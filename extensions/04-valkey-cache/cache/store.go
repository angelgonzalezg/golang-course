package cache

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	valkey "github.com/valkey-io/valkey-go"
)

// Store borrows its client's connections. Its owner closes the shared client.
type Store struct{ client valkey.Client }

// NewStore borrows an already opened client; it does not open another connection.
func NewStore(client valkey.Client) *Store { return &Store{client: client} }

// Set atomically assigns the value AND its expiration with SET ... PX. A
// separate SET then EXPIRE could leave an immortal key if the second command failed.
// This teaching API requires positive, whole-millisecond TTLs to avoid truncation.
func (s *Store) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if ttl < time.Millisecond || ttl%time.Millisecond != 0 {
		return fmt.Errorf("TTL must be positive whole milliseconds: %w", ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := s.client.Do(ctx, s.client.B().Set().Key(key).Value(value).Px(ttl).Build()).Error()
	if err != nil {
		return fmt.Errorf("set cached value: %w", commandError(ctx, err))
	}
	return nil
}

// Get distinguishes a real empty-string hit from a protocol nil reply (miss).
// Only that nil reply maps to ErrMiss; authentication/type/network errors do not.
func (s *Store) Get(ctx context.Context, key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	value, err := s.client.Do(ctx, s.client.B().Get().Key(key).Build()).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return "", fmt.Errorf("%w: %w", ErrMiss, err)
		}
		return "", fmt.Errorf("get cached value: %w", commandError(ctx, err))
	}
	return value, nil
}

// Delete reports whether a key was removed. Repeating it leaves the same absent
// state but returns false; this is not request-key replay or a saved response.
func (s *Store) Delete(ctx context.Context, key string) (bool, error) {
	if err := validateKey(key); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	count, err := s.client.Do(ctx, s.client.B().Del().Key(key).Build()).ToInt64()
	if err != nil {
		return false, fmt.Errorf("delete cached value: %w", commandError(ctx, err))
	}
	return count != 0, nil
}

// Expiration describes normal PTTL states without treating a missing key or a
// persistent key as a transport error. Remaining is meaningful only with HasTTL.
type Expiration struct {
	Exists    bool
	HasTTL    bool
	Remaining time.Duration
}

// TTL inspects server expiration metadata. GET/PTTL are separate observations:
// a key may expire, be replaced, or be deleted between those calls.
func (s *Store) TTL(ctx context.Context, key string) (Expiration, error) {
	if err := validateKey(key); err != nil {
		return Expiration{}, err
	}
	if err := ctx.Err(); err != nil {
		return Expiration{}, err
	}
	ms, err := s.client.Do(ctx, s.client.B().Pttl().Key(key).Build()).ToInt64()
	if err != nil {
		return Expiration{}, fmt.Errorf("inspect cached TTL: %w", commandError(ctx, err))
	}
	return expiration(ms)
}

func expiration(ms int64) (Expiration, error) {
	switch {
	case ms == -2:
		return Expiration{}, nil
	case ms == -1:
		return Expiration{Exists: true}, nil
	case ms >= 0 && ms <= math.MaxInt64/int64(time.Millisecond):
		return Expiration{Exists: true, HasTTL: true, Remaining: time.Duration(ms) * time.Millisecond}, nil
	default:
		return Expiration{}, fmt.Errorf("PTTL reply outside supported duration range: %d", ms)
	}
}

// Valkey keys can be binary. This lab chooses readable UTF-8 names of 1–200
// bytes without NUL; values can still be empty or contain arbitrary bytes.
func validateKey(key string) error {
	if key == "" || len(key) > 200 || !utf8.ValidString(key) || strings.ContainsRune(key, 0) {
		return fmt.Errorf("key requires 1–200 UTF-8 bytes without NUL: %w", ErrInvalidInput)
	}
	return nil
}
