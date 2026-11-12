// Package demo runs a finite sequence of real remote cache operations.
package demo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/course/cache-lab/cache"
)

// Run owns its clients and exact random keys. Cleanup never scans/flushes the
// database, and it gets fresh time before client Close even after caller expiry.
func Run(ctx context.Context, uri string, output io.Writer) (result error) {
	client, err := cache.Open(ctx, uri)
	if err != nil {
		return err
	}
	defer client.Close()
	store := cache.NewStore(client)
	prefix := "course:fundamentals:" + rand.Text()[:12] + ":"
	keys := []string{prefix + "text", prefix + "empty", prefix + "persistent", prefix + "contact", prefix + "bad", prefix + "expiry"}
	// Register keys BEFORE writes: a canceled/failed write may still have reached
	// the server, so cleanup also attempts every key whose result was uncertain.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, key := range keys {
			if _, err := store.Delete(cleanup, key); err != nil {
				result = errors.Join(result, fmt.Errorf("clean up demo key: %w", err))
			}
		}
	}()
	fmt.Fprintln(output, "== 1. A missing key, SET with TTL, then a hit ==")
	if _, err := store.Get(ctx, keys[0]); !errors.Is(err, cache.ErrMiss) {
		return fmt.Errorf("expected initial miss, got %v", err)
	}
	if err := store.Set(ctx, keys[0], "hello", time.Minute); err != nil {
		return err
	}
	value, err := store.Get(ctx, keys[0])
	if err != nil || value != "hello" {
		return fmt.Errorf("expected greeting hit, got %q (%v)", value, err)
	}
	fmt.Fprintln(output, "Absent key -> ErrMiss; SET/PX then GET -> hit with hello.")

	fmt.Fprintln(output, "\n== 2. Empty hits and expiration metadata ==")
	if err := store.Set(ctx, keys[1], "", time.Minute); err != nil {
		return err
	}
	if value, err := store.Get(ctx, keys[1]); err != nil || value != "" {
		return errors.New("empty-string hit was confused with a miss")
	}
	info, err := store.TTL(ctx, keys[0])
	if err != nil || !info.Exists || !info.HasTTL {
		return fmt.Errorf("expected expiring key: %+v (%v)", info, err)
	}
	// Raw SET without PX illustrates PTTL=-1. The cache wrapper itself always
	// requires a TTL; persistent keys can still be created by other clients.
	if err := client.Do(ctx, client.B().Set().Key(keys[2]).Value("persistent example").Build()).Error(); err != nil {
		return err
	}
	persistent, err := store.TTL(ctx, keys[2])
	if err != nil || !persistent.Exists || persistent.HasTTL {
		return errors.New("persistent PTTL state was not preserved")
	}
	fmt.Fprintln(output, "An empty string is a hit; PTTL distinguishes expiring and persistent keys.")

	fmt.Fprintln(output, "\n== 3. Typed JSON and a corrupt payload ==")
	if err := store.SetContact(ctx, keys[3], cache.Contact{ID: 42, Name: " Ana "}, time.Minute); err != nil {
		return err
	}
	contact, err := store.GetContact(ctx, keys[3])
	if err != nil || contact != (cache.Contact{ID: 42, Name: "Ana"}) {
		return fmt.Errorf("contact round trip = (%+v, %v)", contact, err)
	}
	if err := store.Set(ctx, keys[4], "{", time.Minute); err != nil {
		return err
	}
	if _, err := store.GetContact(ctx, keys[4]); !errors.Is(err, cache.ErrInvalidPayload) || errors.Is(err, cache.ErrMiss) {
		return fmt.Errorf("expected distinct payload error, got %v", err)
	}
	fmt.Fprintln(output, "Versioned JSON -> Contact 42/Ana; corrupt JSON -> ErrInvalidPayload, not ErrMiss.")

	fmt.Fprintln(output, "\n== 4. Observe real server expiration ==")
	if err := store.Set(ctx, keys[5], "short lived", 120*time.Millisecond); err != nil {
		return err
	}
	if err := waitForMiss(ctx, store, keys[5]); err != nil {
		return err
	}
	fmt.Fprintln(output, "The short-lived key expired on Valkey; GET now reports ErrMiss.")

	fmt.Fprintln(output, "\n== 5. Shared server state, cancellation, and deletion ==")
	second, err := cache.Open(ctx, uri)
	if err != nil {
		return err
	}
	defer second.Close()
	if value, err := cache.NewStore(second).Get(ctx, keys[0]); err != nil || value != "hello" {
		return errors.New("second client did not read server-owned state")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Get(canceled, keys[1]); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("expected cancellation, got %v", err)
	}
	removed, err := store.Delete(ctx, keys[0])
	if err != nil || !removed {
		return fmt.Errorf("first delete = (%t, %v)", removed, err)
	}
	removed, err = store.Delete(ctx, keys[0])
	if err != nil || removed {
		return fmt.Errorf("repeat delete = (%t, %v)", removed, err)
	}
	fmt.Fprintln(output, "A second client reads the same cache; canceled GET stays a context error.")
	fmt.Fprintln(output, "DEL -> true, repeat DEL -> false; cleanup removes only this run's exact keys.")
	return nil
}

// Poll observable server state with a bound, rather than assuming one fixed
// sleep proves expiration. The ticker and child context always have owners.
func waitForMiss(ctx context.Context, store *cache.Store, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := store.Get(ctx, key)
		if errors.Is(err, cache.ErrMiss) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
