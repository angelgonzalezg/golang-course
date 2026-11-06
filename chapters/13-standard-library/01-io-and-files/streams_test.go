package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

func TestCountBytesHandlesShortReadsAndDataWithErrors(t *testing.T) {
	failure := errors.New("source failed")
	tests := []struct {
		name string
		src  io.Reader
		want int64
		err  error
	}{
		{"empty", strings.NewReader(""), 0, nil},
		{"one byte per read", iotest.OneByteReader(strings.NewReader("Go ☀")), 6, nil},
		{"data and EOF together", iotest.DataErrReader(strings.NewReader("final")), 5, nil},
		{"data and failure together", iotest.DataErrReader(io.MultiReader(strings.NewReader("part"), iotest.ErrReader(failure))), 4, failure},
		{"failure without data", iotest.ErrReader(failure), 0, failure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := countBytes(tt.src)
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Fatalf("countBytes = (%d, %v), want (%d, %v)", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestReadDocumentSizeBoundary(t *testing.T) {
	for _, size := range []int{0, 1, maxDocumentBytes - 1, maxDocumentBytes, maxDocumentBytes + 1} {
		input := strings.Repeat("x", size)
		got, err := readDocument(iotest.OneByteReader(strings.NewReader(input)))
		if size > maxDocumentBytes {
			if !errors.Is(err, ErrDocumentTooLarge) || got != nil {
				t.Fatalf("size %d: got (%q, %v), want nil and ErrDocumentTooLarge", size, got, err)
			}
		} else if err != nil || string(got) != input {
			t.Fatalf("size %d: result length %d, err=%v", size, len(got), err)
		}
	}
}

func TestReadDocumentStopsAfterOverflowProbe(t *testing.T) {
	src := strings.NewReader(strings.Repeat("x", maxDocumentBytes*10))
	_, err := readDocument(src)
	if !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("expected ErrDocumentTooLarge, got %v", err)
	}
	if want := maxDocumentBytes*10 - (maxDocumentBytes + 1); src.Len() != want {
		t.Fatalf("unread bytes = %d, want %d", src.Len(), want)
	}
}

func TestReadDocumentDiscardsPartialDataOnFailure(t *testing.T) {
	failure := errors.New("read failed")
	src := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(failure))
	data, err := readDocument(src)
	if data != nil || !errors.Is(err, failure) {
		t.Fatalf("readDocument = (%q, %v), want nil and original failure", data, err)
	}
}

func TestForEachLinePreservesContent(t *testing.T) {
	var got []string
	err := forEachLine(iotest.OneByteReader(strings.NewReader("Go\r\n\n☀\nlast")), func(line string) error {
		got = append(got, line)
		return nil
	})
	if want := []string{"Go", "", "☀", "last"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("lines = %q, err=%v, want %q", got, err, want)
	}
	if err := forEachLine(strings.NewReader(""), func(string) error {
		t.Fatal("empty input must not produce a line")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestForEachLineReportsOversizedTokens(t *testing.T) {
	var got []string
	err := forEachLine(strings.NewReader("ok\n"+strings.Repeat("x", maxScanBytes+1)), func(line string) error {
		got = append(got, line)
		return nil
	})
	if !errors.Is(err, bufio.ErrTooLong) || !slices.Equal(got, []string{"ok"}) {
		t.Fatalf("lines = %q, err=%v; want prior line and ErrTooLong", got, err)
	}
}

func TestForEachLineReportsInputAndCallbackFailures(t *testing.T) {
	failure := errors.New("input failed")
	var got []string
	err := forEachLine(io.MultiReader(strings.NewReader("first\n"), iotest.ErrReader(failure)), func(line string) error {
		got = append(got, line)
		return nil
	})
	if !errors.Is(err, failure) || !slices.Equal(got, []string{"first"}) {
		t.Fatalf("lines = %q, err=%v; want first line and input failure", got, err)
	}

	failure = errors.New("callback failed")
	got = nil
	err = forEachLine(strings.NewReader("first\nsecond\n"), func(line string) error {
		got = append(got, line)
		return failure
	})
	if !errors.Is(err, failure) || !slices.Equal(got, []string{"first"}) {
		t.Fatalf("lines = %q, err=%v; processing must stop after callback failure", got, err)
	}
}

func TestWriteLinesFlushesToBorrowedWriter(t *testing.T) {
	var output bytes.Buffer
	if err := writeLines(&output, []string{"first", "", "☀"}); err != nil {
		t.Fatal(err)
	}
	if want := "first\n\n☀\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	if err := writeLines(&output, nil); err != nil {
		t.Fatal(err)
	}
	if output.String() != "first\n\n☀\n" {
		t.Fatal("empty input changed existing output")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWriteLinesReportsWriteAndFlushFailures(t *testing.T) {
	failure := errors.New("output failed")
	// Small output stays buffered until Flush; large output reaches Write earlier.
	for _, line := range []string{"small", strings.Repeat("x", 8192)} {
		if err := writeLines(failingWriter{failure}, []string{line}); !errors.Is(err, failure) {
			t.Fatalf("line length %d: want output failure, got %v", len(line), err)
		}
	}
}
