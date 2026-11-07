// Context
// Topic: propagate cancellation, deadlines, and request-scoped values across APIs.
// Usage: go run ./chapters/14-context/01-context
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"time"
)

func showRootsAndValues() {
	fmt.Println("== 1. Background and TODO are non-nil roots ==")
	root := context.Background()
	_, hasDeadline := root.Deadline()
	fmt.Println("Background has a deadline:", hasDeadline)
	fmt.Println("Background Done is nil:", root.Done() == nil)
	fmt.Println("TODO is also uncancelable:", context.TODO().Done() == nil)
	// Use Background at an application boundary. TODO marks an unresolved
	// propagation decision; passing nil is not the alternative.

	fmt.Println("\n== 2. Values use typed accessors and preserve parent scope ==")
	parent := withRequestID(root, "req-parent")
	child := withRequestID(parent, "req-child")
	parentID, _ := requestIDFrom(parent)
	childID, _ := requestIDFrom(child)
	fmt.Println("parent request ID:", parentID)
	fmt.Println("child request ID:", childID)
	_, present := requestIDFrom(root)
	fmt.Println("root has request metadata:", present)
}

func showCancellationTree() {
	fmt.Println("\n== 3. Cancellation flows from parents to descendants ==")
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	child, cancelChild := context.WithCancel(parent)
	defer cancelChild()
	sibling, cancelSibling := context.WithCancel(parent)
	defer cancelSibling()
	cancelChild()
	<-child.Done()
	fmt.Println("child canceled:", errors.Is(child.Err(), context.Canceled))
	fmt.Println("parent still active:", parent.Err() == nil)
	fmt.Println("sibling still active:", sibling.Err() == nil)
	cancelParent()
	<-sibling.Done()
	fmt.Println("parent cancellation reached sibling:", errors.Is(sibling.Err(), context.Canceled))
}

func showWorkAndBudgets() error {
	fmt.Println("\n== 4. Pass a context through service and dependency ==")
	ready := make(chan struct{})
	close(ready)
	service := reportService{store: gatedReports{ready: ready}}
	report, err := service.Generate(context.Background(), "sales", time.Second)
	if err != nil {
		return err
	}
	fmt.Println("completed report:", report.Title)

	fmt.Println("\n== 5. Request cancellation and then join the work ==")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocked := reportService{store: gatedReports{}}
	var workErr error
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, workErr = blocked.Generate(ctx, "sales", time.Hour)
	}()
	cancel()
	<-finished // Closing Done requests cancellation; this acknowledges completion.
	if !errors.Is(workErr, context.Canceled) {
		return fmt.Errorf("expected canceled work, got %v", workErr)
	}
	fmt.Println("worker returned after cancellation:", errors.Is(workErr, context.Canceled))

	fmt.Println("\n== 6. Timeouts bound a layer; earlier parent deadlines win ==")
	_, err = blocked.Generate(context.Background(), "sales", 5*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("expected deadline exhaustion, got %v", err)
	}
	fmt.Println("blocked dependency exceeded its budget:", errors.Is(err, context.DeadlineExceeded))
	deadline := time.Now().Add(time.Hour)
	parent, cancelParent := context.WithDeadline(context.Background(), deadline)
	defer cancelParent()
	child, cancelChild := context.WithTimeout(parent, 2*time.Hour)
	defer cancelChild()
	childDeadline, _ := child.Deadline()
	fmt.Println("longer child timeout kept parent deadline:", childDeadline.Equal(deadline))
	return nil
}

func showCausesAndLifecycle() error {
	fmt.Println("\n== 7. Err classifies cancellation; Cause explains it ==")
	shutdown := errors.New("application is shutting down")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	cancel(shutdown)
	fmt.Println("Err is Canceled:", errors.Is(ctx.Err(), context.Canceled))
	fmt.Println("Cause is the supplied reason:", errors.Is(context.Cause(ctx), shutdown))
	budgetReason := errors.New("report budget exhausted")
	timed, cancelTimed := context.WithTimeoutCause(context.Background(), 5*time.Millisecond, budgetReason)
	defer cancelTimed()
	<-timed.Done()
	fmt.Println("timed Err is DeadlineExceeded:", errors.Is(timed.Err(), context.DeadlineExceeded))
	fmt.Println("timed Cause preserves the reason:", errors.Is(context.Cause(timed), budgetReason))

	fmt.Println("\n== 8. Cancellation callbacks need their own completion acknowledgment ==")
	callbackCtx, cancelCallback := context.WithCancel(context.Background())
	defer cancelCallback()
	callbackDone := make(chan struct{})
	stop := context.AfterFunc(callbackCtx, func() { close(callbackDone) })
	defer stop()
	cancelCallback()
	<-callbackDone
	fmt.Println("cancellation callback completed")

	fmt.Println("\n== 9. Detached work needs a new lifetime and an owner ==")
	requestCtx, cancelRequest := context.WithCancel(withRequestID(context.Background(), "req-detached"))
	cancelRequest()
	// WithoutCancel retains values but removes the parent's cancellation and
	// deadline. Only use it for deliberately separately owned work, such as cleanup.
	detached := context.WithoutCancel(requestCtx)
	id, _ := requestIDFrom(detached)
	fmt.Println("retained request metadata:", id)
	fmt.Println("detached Done is nil:", detached.Done() == nil)
	cleanupCtx, cancelCleanup := context.WithTimeout(detached, time.Second)
	defer cancelCleanup()
	ready := make(chan struct{})
	close(ready)
	service := reportService{store: gatedReports{ready: ready}}
	if _, err := service.Generate(cleanupCtx, "sales", time.Second); err != nil {
		return err
	}
	// Synchronous completion here is the owner's join; nothing is left running.
	fmt.Println("separately bounded work finished")
	return nil
}

func showHTTPPropagation() error {
	fmt.Println("\n== 10. Carry request metadata and context across handler layers ==")
	ready := make(chan struct{})
	close(ready)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := newReportHandler(reportService{store: gatedReports{ready: ready}}, time.Second, logger)
	// This constructs a server-side request for an in-process handler demo.
	// Across real HTTP, a client's context values are not automatically sent;
	// metadata requires an explicit wire representation such as this header.
	request, err := http.NewRequestWithContext(context.Background(), "GET", "http://example.test/reports/sales", nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Request-ID", "req-http")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return fmt.Errorf("unexpected report response: %d", recorder.Code)
	}
	fmt.Println("response request ID:", recorder.Header().Get("X-Request-ID"))
	fmt.Print("response body: ", recorder.Body.String())
	return nil
}

func run() error {
	showRootsAndValues()
	showCancellationTree()
	if err := showWorkAndBudgets(); err != nil {
		return err
	}
	if err := showCausesAndLifecycle(); err != nil {
		return err
	}
	if err := showHTTPPropagation(); err != nil {
		return err
	}
	fmt.Println("\n== 11. Context rules ==")
	fmt.Println("Pass ctx first, derive from the caller, and release every owned cancel function.")
	fmt.Println("Use values for optional request metadata; pass required data and dependencies explicitly.")
	fmt.Println("Observe cancellation at blocking points and confirm completion before the owner returns.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "context lesson:", err)
		os.Exit(1)
	}
}
