//go:build integration

// Tagged tests own a real authenticated Valkey server. No test reads VALKEY_URL.
package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/course/cache-lab/cache"
	"example.com/course/cache-lab/internal/demo"
	"github.com/testcontainers/testcontainers-go"
	valkeycontainer "github.com/testcontainers/testcontainers-go/modules/valkey"
	valkey "github.com/valkey-io/valkey-go"
)

// Run basic contracts serially, then stop only the owned server to prove that
// a real connection failure is different from a normal missing-key response.
func TestValkey(t *testing.T) {
	client, uri, container := newTestCache(t)
	t.Run("strings misses and metadata", func(t *testing.T) { testStrings(t, client) })
	t.Run("real expiration", func(t *testing.T) { testExpiration(t, client) })
	t.Run("typed JSON and server errors", func(t *testing.T) { testJSON(t, client) })
	t.Run("clients contexts and authentication", func(t *testing.T) { testContexts(t, client, uri) })
	t.Run("finite demo cleanup", func(t *testing.T) { testDemo(t, client, uri) })
	t.Run("server unavailable", func(t *testing.T) {
		ctx := operationContext(t)
		if err := container.Stop(ctx, nil); err != nil {
			t.Fatal(err)
		}
		request, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_, err := cache.NewStore(client).Get(request, "course:test:unavailable")
		if err == nil || errors.Is(err, cache.ErrMiss) {
			t.Fatalf("connection failure became a miss: %v", err)
		}
	})
}

// Startup discovers a dynamic port. Cleanup disconnects the client before
// terminating the container, including when startup fails after partial creation.
func newTestCache(t *testing.T) (valkey.Client, string, *valkeycontainer.ValkeyContainer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := valkeycontainer.Run(ctx, "valkey/valkey:9.0.7-alpine3.24",
		testcontainers.WithCmd("--save", "", "--appendonly", "no", "--requirepass", "fixture_password"))
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start Valkey (check docker info): %v", err)
	}
	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Scheme, parsed.Path, parsed.User = "valkey", "/0", url.UserPassword("default", "fixture_password")
	client, err := cache.Open(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client, parsed.String(), container
}

// Each case has its own deadline, separate from the package's overall timeout.
func operationContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// Register exact keys before writes, with cleanup that survives canceled calls.
func ownedKey(t *testing.T, store *cache.Store) string {
	t.Helper()
	key := "course:test:" + rand.Text()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := store.Delete(ctx, key); err != nil {
			t.Error(err)
		}
	})
	return key
}

// Observe actual string, nil, and PTTL replies. Empty/binary strings are valid
// hits, and SET/PX replaces a persistent key with an explicitly expiring value.
func testStrings(t *testing.T, client valkey.Client) {
	ctx := operationContext(t)
	store := cache.NewStore(client)
	key := ownedKey(t, store)
	if _, err := store.Get(ctx, key); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("missing GET = %v", err)
	}
	if info, err := store.TTL(ctx, key); err != nil || info.Exists {
		t.Fatalf("missing TTL = (%+v, %v)", info, err)
	}
	for _, value := range []string{"hello", "", "a\x00b\xff"} {
		if err := store.Set(ctx, key, value, time.Minute); err != nil {
			t.Fatal(err)
		}
		got, err := store.Get(ctx, key)
		if err != nil || got != value {
			t.Fatalf("string round trip = (%q, %v)", got, err)
		}
		info, err := store.TTL(ctx, key)
		if err != nil || !info.Exists || !info.HasTTL || info.Remaining < 0 || info.Remaining > time.Minute {
			t.Fatalf("SET/PX TTL = (%+v, %v)", info, err)
		}
	}
	if err := client.Do(ctx, client.B().Set().Key(key).Value("no TTL").Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if info, err := store.TTL(ctx, key); err != nil || !info.Exists || info.HasTTL {
		t.Fatalf("persistent TTL = (%+v, %v)", info, err)
	}
	if err := store.Set(ctx, key, "expires again", time.Minute); err != nil {
		t.Fatal(err)
	}
	if info, err := store.TTL(ctx, key); err != nil || !info.HasTTL {
		t.Fatalf("persistent overwrite = (%+v, %v)", info, err)
	}
	if removed, err := store.Delete(ctx, key); err != nil || !removed {
		t.Fatalf("first DEL = (%t, %v)", removed, err)
	}
	if removed, err := store.Delete(ctx, key); err != nil || removed {
		t.Fatalf("repeat DEL = (%t, %v)", removed, err)
	}
}

// Expiration belongs to the real server's clock. Poll with a deadline rather
// than using one fixed sleep or assuming Go's virtual time controls a container.
func testExpiration(t *testing.T, client valkey.Client) {
	store := cache.NewStore(client)
	key := ownedKey(t, store)
	ctx, cancel := context.WithTimeout(operationContext(t), 5*time.Second)
	defer cancel()
	if err := store.Set(ctx, key, "short lived", 80*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := store.Get(ctx, key)
		if errors.Is(err, cache.ErrMiss) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if info, err := store.TTL(ctx, key); err != nil || info.Exists {
		t.Fatalf("expired PTTL = (%+v, %v)", info, err)
	}
}

// Decode actual remote JSON, then distinguish corrupt JSON and a wrong server
// value type from missing keys. Domain errors retain their driver causes.
func testJSON(t *testing.T, client valkey.Client) {
	ctx := operationContext(t)
	store := cache.NewStore(client)
	key := ownedKey(t, store)
	if err := store.SetContact(ctx, key, cache.Contact{ID: 42, Name: " Ana "}, time.Minute); err != nil {
		t.Fatal(err)
	}
	contact, err := store.GetContact(ctx, key)
	if err != nil || contact != (cache.Contact{ID: 42, Name: "Ana"}) {
		t.Fatalf("remote JSON = (%+v, %v)", contact, err)
	}
	if err := store.Set(ctx, key, "{", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetContact(ctx, key); !errors.Is(err, cache.ErrInvalidPayload) || errors.Is(err, cache.ErrMiss) {
		t.Fatalf("bad JSON = %v", err)
	}
	if _, err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, client.B().Hset().Key(key).FieldValue().FieldValue("name", "Ana").Build()).Error(); err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(ctx, key)
	var server *valkey.ValkeyError
	if err == nil || errors.Is(err, cache.ErrMiss) || !errors.As(err, &server) {
		t.Fatalf("wrong server type = %v", err)
	}
}

// One reusable client does not own server data. Verify caller cancellation,
// expired deadlines, and authentication failure without turning them into misses.
func testContexts(t *testing.T, client valkey.Client, uri string) {
	ctx := operationContext(t)
	store := cache.NewStore(client)
	key := ownedKey(t, store)
	if err := store.Set(ctx, key, "saved", time.Minute); err != nil {
		t.Fatal(err)
	}
	second, err := cache.Open(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Close)
	if value, err := cache.NewStore(second).Get(ctx, key); err != nil || value != "saved" {
		t.Fatalf("second client = (%q, %v)", value, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Set(canceled, key, "must not replace", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SET = %v", err)
	}
	if _, err := store.Get(canceled, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GET = %v", err)
	}
	if _, err := store.Delete(canceled, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled DEL = %v", err)
	}
	if _, err := store.TTL(canceled, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled PTTL = %v", err)
	}
	expired, finish := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer finish()
	if _, err := store.Get(expired, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired GET = %v", err)
	}
	if value, err := store.Get(ctx, key); err != nil || value != "saved" {
		t.Fatalf("canceled write changed state = (%q, %v)", value, err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword("default", "wrong_password")
	bad, err := cache.Open(ctx, parsed.String())
	if bad != nil {
		bad.Close()
		t.Fatal("client returned after failed authentication")
	}
	if err == nil || errors.Is(err, cache.ErrMiss) {
		t.Fatalf("authentication failure = %v", err)
	}
}

// Repeated demos remove only their own keys. DBSIZE is safe here because this
// entire small server belongs to the test and earlier cases cleaned their keys.
func testDemo(t *testing.T, client valkey.Client, uri string) {
	ctx := operationContext(t)
	store := cache.NewStore(client)
	key := ownedKey(t, store)
	if err := store.Set(ctx, key, "keep neighbor", time.Minute); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var output bytes.Buffer
		if err := demo.Run(ctx, uri, &output); err != nil {
			t.Fatal(err)
		}
		for _, summary := range []string{"Absent key -> ErrMiss", "empty string is a hit", "corrupt JSON -> ErrInvalidPayload", "key expired on Valkey", "repeat DEL -> false"} {
			if !strings.Contains(output.String(), summary) {
				t.Errorf("demo missing %q", summary)
			}
		}
		if value, err := store.Get(ctx, key); err != nil || value != "keep neighbor" {
			t.Fatalf("neighbor cleanup = (%q, %v)", value, err)
		}
		count, err := client.Do(ctx, client.B().Dbsize().Build()).ToInt64()
		if err != nil || count != 1 {
			t.Fatalf("demo keys remained = (%d, %v)", count, err)
		}
	}
}
