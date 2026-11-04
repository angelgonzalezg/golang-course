// Error Handling
// Topic: errors are returned as values and checked explicitly.
// Args/Input: examples use division, parsing, user lookup, validation, and cleanup.
// Usage: return errors, wrap errors, compare errors, and extract custom error data.
// Key idea: errors are normal values; handle expected failures with error, not panic.
package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

/* Helper section 1: Sentinel errors */
// Sentinel errors are package-level values that callers can compare with errors.Is.
var (
	ErrDivisionByZero     = errors.New("division by zero")
	ErrDividendOutOfRange = errors.New("dividend must be between -1000 and 1000")
	ErrInvalidPort        = errors.New("port must be between 1 and 65535")
	ErrNotFound           = errors.New("not found")
	ErrInvalidInput       = errors.New("invalid input")
)

/* Helper section 2: Custom error type */
/* Custom error type with extra fields */
type OperationError struct {
	Op      string
	Code    int
	Message string
}

func (e *OperationError) Error() string {
	// Error makes OperationError satisfy the error interface.
	return fmt.Sprintf("%s failed: code %d: %s", e.Op, e.Code, e.Message)
}

/* Helper section 3: Functions that return errors */
func divide(a, b int) (int, error) {
	if b == 0 {
		return 0, ErrDivisionByZero
	}

	// An exercise-specific input limit gives us a second sentinel to inspect.
	if a < -1000 || a > 1000 {
		return 0, ErrDividendOutOfRange
	}

	return a / b, nil
}

// parsePort reads a configured destination port. This exercise excludes port 0,
// which has a separate use when asking a listener to choose an available port.
func parsePort(text string) (int, error) {
	port, err := strconv.Atoi(text)
	if err != nil {
		// %w wraps the original error so callers can inspect it later.
		return 0, fmt.Errorf("parse port %q: %w", text, err)
	}

	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("parse port %q: %w", text, ErrInvalidPort)
	}

	return port, nil
}

func findUserName(id int) (string, error) {
	users := map[int]string{
		1: "Ana",
		2: "Ben",
	}

	name, ok := users[id]
	if !ok {
		return "", fmt.Errorf("find user %d: %w", id, ErrNotFound)
	}

	return name, nil
}

/* Helper section 4: Function that can return multiple errors */
func validateSignup(name string, age int) error {
	var validationErrors []error

	if strings.TrimSpace(name) == "" {
		validationErrors = append(validationErrors, fmt.Errorf("name must not be blank: %w", ErrInvalidInput))
	}

	if age < 18 {
		validationErrors = append(validationErrors, fmt.Errorf("age must be at least 18: %w", ErrInvalidInput))
	}

	// errors.Join returns nil when there are no errors.
	return errors.Join(validationErrors...)
}

/* Helper section 5: Function that returns a custom error */
func chargePayment(amountCents int) error {
	if amountCents <= 0 {
		return &OperationError{
			Op:      "charge payment",
			Code:    400,
			Message: "amount must be greater than zero",
		}
	}

	return nil
}

/* Helper section 6: Fake file type for cleanup examples */
type fakeFile struct {
	Name       string
	processErr error // Inject failures without depending on the filesystem.
	closeErr   error
	closed     bool
}

func openFakeFile(name string) (*fakeFile, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("open file: %w", ErrInvalidInput)
	}

	return &fakeFile{Name: name}, nil
}

func (f *fakeFile) Close() error {
	f.closed = true
	fmt.Println("closing file:", f.Name)
	return f.closeErr
}

func processFakeFile(name string) error {
	file, err := openFakeFile(name)
	if err != nil {
		return err
	}
	return processAndClose(file)
}

// processAndClose owns cleanup for a successfully opened, non-nil file.
func processAndClose(file *fakeFile) (err error) {
	// Named err lets deferred cleanup change the error returned to the caller.
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Keep both causes if processing and cleanup fail.
			err = errors.Join(err, fmt.Errorf("close file %q: %w", file.Name, closeErr))
		}
	}()

	fmt.Println("processing file:", file.Name)
	if file.processErr != nil {
		return fmt.Errorf("process file %q: %w", file.Name, file.processErr)
	}
	return nil
}

/* Helper section 7: Small panic/recover example */
func demonstrateRecover() {
	defer func() {
		recoveredValue := recover()
		if recoveredValue != nil {
			fmt.Println("recovered from panic:", recoveredValue)
		}
	}()

	// This panic is intentional for learning.
	panic("programmer mistake example")
}

func main() {
	/* Section 1: Errors are values */
	fmt.Println("== 1. Errors are values ==")

	value, err := divide(10, 2)
	if err != nil {
		fmt.Println("divide error:", err)
	} else {
		fmt.Println("divide result:", value)
	}

	_, err = divide(10, 0)
	if err != nil {
		fmt.Println("expected error:", err)
	}

	/* Section 2: Return early when there is an error */
	fmt.Println("\n== 2. Return early when there is an error ==")

	port, err := parsePort("8080")
	if err != nil {
		fmt.Println("parse error:", err)
		return
	}
	fmt.Println("parsed port:", port)

	/* Section 3: Sentinel errors and errors.Is */
	fmt.Println("\n== 3. Sentinel errors and errors.Is ==")

	_, err = divide(1001, 1)
	if errors.Is(err, ErrDividendOutOfRange) {
		fmt.Println("dividend is outside this exercise's allowed range")
	}

	_, err = findUserName(99)
	if errors.Is(err, ErrNotFound) {
		fmt.Println("user was not found")
	}

	/* Section 4: Wrapping errors */
	fmt.Println("\n== 4. Wrapping errors ==")

	_, err = parsePort("not-a-number")
	if err != nil {
		fmt.Println("wrapped error:", err)
		fmt.Println("contains strconv.ErrSyntax:", errors.Is(err, strconv.ErrSyntax))
	}
	_, err = parsePort("70000")
	if errors.Is(err, ErrInvalidPort) {
		fmt.Println("port outside configured range:", err)
	}

	// Wrapped errors keep context plus the original cause.
	_, err = findUserName(99)
	fmt.Println("lookup error with context:", err)

	/* Section 5: Custom errors and errors.As */
	fmt.Println("\n== 5. Custom errors and errors.As ==")

	err = chargePayment(0)
	if err != nil {
		err = fmt.Errorf("checkout: %w", err)
		fmt.Println("payment error:", err)

		var opErr *OperationError
		if errors.As(err, &opErr) {
			fmt.Println("operation:", opErr.Op)
			fmt.Println("code:", opErr.Code)
			fmt.Println("message:", opErr.Message)
		}
	}

	/* Section 6: Multiple errors with errors.Join */
	fmt.Println("\n== 6. Multiple errors with errors.Join ==")

	err = validateSignup("", 15)
	if err != nil {
		fmt.Println("validation error:", err)
		fmt.Println("contains invalid input:", errors.Is(err, ErrInvalidInput))
	}

	/* Section 7: Cleanup with defer */
	fmt.Println("\n== 7. Cleanup with defer ==")

	err = processFakeFile("config.json")
	if err != nil {
		fmt.Println("file error:", err)
	}

	err = processFakeFile("")
	if err != nil {
		fmt.Println("file error:", err)
	}

	processErr := errors.New("processing failed")
	closeErr := errors.New("cleanup failed")
	err = processAndClose(&fakeFile{Name: "report.txt", processErr: processErr, closeErr: closeErr})
	if err != nil {
		fmt.Println("processing and cleanup errors:", err)
		fmt.Println("preserved processing failure:", errors.Is(err, processErr))
		fmt.Println("preserved cleanup failure:", errors.Is(err, closeErr))
	}

	/* Section 8: Do not ignore errors */
	fmt.Println("\n== 8. Do not ignore errors ==")

	// Avoid using _ when the error matters.
	// Good: check err right after the call that returned it.
	_, err = divide(8, 0)
	if err != nil {
		fmt.Println("handled instead of ignored:", err)
	}

	/* Section 9: Error messages */
	fmt.Println("\n== 9. Error messages ==")

	// Error strings are usually lowercase and do not end with punctuation.
	// Add context when returning the error, not when creating every sentinel.
	fmt.Println("good error style:", ErrDivisionByZero)

	/* Section 10: A nil pointer inside an error is not a nil error */
	fmt.Println("\n== 10. Nil errors and typed nil pointers ==")

	var nilOperation *OperationError
	var typedNil error = nilOperation
	fmt.Println("pointer is nil:", nilOperation == nil)
	// An interface is nil only when both its dynamic type and value are absent.
	fmt.Println("error containing the pointer is nil:", typedNil == nil)
	// Do not return a typed nil pointer as error; return nil explicitly.
	fmt.Println("successful payment returns nil:", chargePayment(100) == nil)

	/* Section 11: Error, panic, and recover */
	fmt.Println("\n== 11. Error, panic, and recover ==")

	// Use error for expected problems: bad input, missing files, failed requests.
	// Use panic for programmer mistakes or impossible states.
	fmt.Println("Expected failures should usually return error.")

	// recover must be called directly by a deferred function in the panicking goroutine.
	demonstrateRecover()

	/* Section 12: Final error rules */
	fmt.Println("\n== 12. Final error rules ==")

	// Check errors explicitly.
	// Wrap errors with %w when adding context.
	// Use errors.Is for known sentinel errors.
	// Use errors.As for custom error types.
	fmt.Println("Errors are values; return them, check them, and add useful context.")
}
