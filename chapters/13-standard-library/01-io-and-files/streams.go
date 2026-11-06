package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

const (
	maxDocumentBytes = 1024
	maxScanBytes     = 4 * 1024
)

var ErrDocumentTooLarge = errors.New("document exceeds the 1024-byte limit")

// countBytes demonstrates the Reader contract without retaining the input.
// Use a custom Read loop when each chunk needs processing; use io.Copy for an
// ordinary transfer. The caller owns r and must close it if it needs closing.
func countBytes(r io.Reader) (int64, error) {
	var total int64
	buffer := make([]byte, 32)
	for {
		n, err := r.Read(buffer)
		// Read can return both bytes and an error. Process buffer[:n] first.
		// Short reads and (0, nil) are not EOF; a reader must eventually progress.
		total += int64(n)
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			// The partial count remains useful even when the input fails.
			return total, fmt.Errorf("count input bytes: %w", err)
		}
	}
}

// readDocument accepts a small, complete document and rejects larger input.
// ReadAll is convenient when the entire result fits the application's memory
// budget. Reading one extra byte distinguishes an exact fit from truncation.
// This borrows r; failures return no usable document, even if bytes were read.
func readDocument(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}
	if len(data) > maxDocumentBytes {
		return nil, ErrDocumentTooLarge
	}
	return data, nil
}

// forEachLine uses Scanner when input is line-oriented and tokens have a known
// size bound. It streams callbacks instead of collecting every line in memory.
// Completed callbacks are not rolled back if scanning or a later callback fails.
func forEachLine(r io.Reader, visit func(string) error) error {
	scanner := bufio.NewScanner(r)
	// Configure before Scan. The buffer limit also needs room for delimiters;
	// it is not an exact allowance of maxScanBytes bytes of line content.
	scanner.Buffer(make([]byte, 256), maxScanBytes)
	for scanner.Scan() {
		// Text returns a string that can be retained. Scanner.Bytes instead
		// borrows storage that may be overwritten by the next Scan call.
		if err := visit(scanner.Text()); err != nil {
			return fmt.Errorf("process line: %w", err)
		}
	}
	// Scan returning false means either normal exhaustion or a failure.
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan lines: %w", err)
	}
	return nil
}

// writeLines buffers several small writes, then explicitly flushes them to w.
// The caller owns w. Returning success means Flush succeeded; it does not mean
// that a file was closed, synced to disk, or updated atomically.
func writeLines(w io.Writer, lines []string) error {
	buffered := bufio.NewWriter(w)
	for _, line := range lines {
		if _, err := fmt.Fprintln(buffered, line); err != nil {
			return fmt.Errorf("write line: %w", err)
		}
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("flush lines: %w", err)
	}
	return nil
}
