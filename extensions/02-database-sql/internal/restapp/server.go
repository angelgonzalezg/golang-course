// Package restapp runs the persistent REST lesson. PostgreSQL stays in this
// extension module; the chapter's HTTP adapter remains standard-library-only.
package restapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"example.com/course/sql-lab/contactpg"
	"example.com/course/sql-lab/contactsql"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/httpapi"
)

// Serve owns the migrated database pool, listener, and Serve goroutine. subject
// is an operator-configured namespace for this single-user, loopback lab; it
// supplies no authentication or per-contact access control for a deployed API.
func Serve(ctx context.Context, dsn, addr, subject string, out io.Writer) error {
	if err := contacts.ValidateRequestIdentity(subject, "configuration"); err != nil {
		return err
	}
	if _, err := contactpg.Migrate(dsn); err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	db, err := contactsql.Open(startup, dsn)
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := httpapi.NewHandlerWithIdempotency(contactpg.NewStore(db), func(*http.Request) string { return subject }, logger)
	server, served := start(listener, handler)
	fmt.Fprintf(out, "PostgreSQL contacts API: http://%s\nKey namespace: %s; stop with Ctrl+C. Committed data is retained.\n", listener.Addr(), subject)
	select {
	case <-ctx.Done():
		return stop(server, served)
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return errors.Join(err, server.Close())
	}
}

// start bounds database work before the write timeout. The request context is
// passed through the shared handler into SQL, including pool and lock waits.
func start(listener net.Listener, handler http.Handler) (*http.Server, <-chan error) {
	bounded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
	server := &http.Server{Handler: bounded, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 * 1024}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	return server, served
}

// stop has its own budget because the signal/request context may already be
// canceled. Join Serve before its caller closes the pool or exits the process.
func stop(server *http.Server, served <-chan error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := server.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, server.Close())
	}
	if serveErr := <-served; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
	}
	return err
}
