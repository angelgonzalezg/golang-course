// Package cache teaches remote Valkey operations and an explicit JSON payload.
package cache

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	valkey "github.com/valkey-io/valkey-go"
)

var (
	ErrInvalidInput   = errors.New("invalid cache input")
	ErrMiss           = errors.New("cache key not found")
	ErrInvalidPayload = errors.New("invalid cached contact payload")
)

// Open creates a reusable, concurrent-safe client and verifies it with PING.
// The caller owns Close. Command contexts do not become client lifetime contexts.
func Open(ctx context.Context, uri string) (valkey.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(uri)
	if err != nil || strings.TrimSpace(uri) == "" || parsed.Hostname() == "" || parsed.RawQuery != "" {
		// Do not echo a URI containing credentials into an error message.
		return nil, fmt.Errorf("supply a standalone Valkey URL with a host and no query options: %w", ErrInvalidInput)
	}
	if parsed.Scheme != "valkey" && parsed.Scheme != "valkeys" && parsed.Scheme != "redis" && parsed.Scheme != "rediss" {
		return nil, fmt.Errorf("unsupported cache URL scheme: %w", ErrInvalidInput)
	}
	options, err := valkey.ParseURL(uri)
	if err != nil || options.SelectDB < 0 {
		return nil, fmt.Errorf("invalid cache URL settings: %w", ErrInvalidInput)
	}
	// NewClient has no context argument. Initial connections have separate dial/
	// network bounds; PING and subsequent operations use their caller contexts.
	options.Dialer.Timeout = 3 * time.Second
	options.ConnWriteTimeout = 3 * time.Second
	options.ForceSingleClient = true
	options.DisableCache = true
	options.DisableRetry = true
	if options.TLSConfig != nil {
		options.TLSConfig.MinVersion = tls.VersionTLS13
	}
	// DisableCache keeps these lessons about server-side keys/TTL. Ordinary Do
	// calls perform remote commands; client-side caching is a separate feature.
	client, err := valkey.NewClient(options)
	if err != nil {
		return nil, fmt.Errorf("connect to Valkey: %w", commandError(ctx, err))
	}
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping Valkey: %w", commandError(ctx, err))
	}
	return client, nil
}

// Keep driver and context causes inspectable; a failure is never silently a miss.
func commandError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	return err
}
