package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"
)

// These doubles make cleanup failures reproducible without relying on disk faults.
type recordingWriteCloser struct {
	output   bytes.Buffer
	writeErr error
	closeErr error
	closes   int
}

func (w *recordingWriteCloser) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.output.Write(data)
}

func (w *recordingWriteCloser) Close() error {
	w.closes++
	return w.closeErr
}

type borrowedReadCloser struct {
	io.Reader
	closes int
}

func (r *borrowedReadCloser) Close() error {
	r.closes++
	return nil
}

func TestCopyAndCloseHonorsOwnershipAndKeepsBothErrors(t *testing.T) {
	readFailure := errors.New("read failed")
	writeFailure := errors.New("write failed")
	closeFailure := errors.New("close failed")
	tests := []struct {
		name     string
		src      io.Reader
		writeErr error
		closeErr error
		want     string
		cause    error
	}{
		{"success", strings.NewReader("Go ☀"), nil, nil, "Go ☀", nil},
		{"read failure", io.MultiReader(strings.NewReader("part"), iotest.ErrReader(readFailure)), nil, nil, "part", readFailure},
		{"read and close failure", io.MultiReader(strings.NewReader("part"), iotest.ErrReader(readFailure)), nil, closeFailure, "part", readFailure},
		{"write and close failure", strings.NewReader("Go"), writeFailure, closeFailure, "", writeFailure},
		{"close failure", strings.NewReader("Go"), nil, closeFailure, "Go", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &borrowedReadCloser{Reader: tt.src}
			dst := &recordingWriteCloser{writeErr: tt.writeErr, closeErr: tt.closeErr}
			written, err := copyAndClose(dst, src)
			if written != int64(len(tt.want)) || dst.output.String() != tt.want {
				t.Fatalf("copied (%d, %q), want (%d, %q)", written, dst.output.String(), len(tt.want), tt.want)
			}
			if dst.closes != 1 || src.closes != 0 {
				t.Fatalf("close calls: destination=%d source=%d, want 1 and 0", dst.closes, src.closes)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Fatalf("lost copy cause %v in %v", tt.cause, err)
			}
			if tt.closeErr != nil && !errors.Is(err, tt.closeErr) {
				t.Fatalf("lost close cause %v in %v", tt.closeErr, err)
			}
			if tt.cause == nil && tt.closeErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCopyToNewFilePreservesExistingDestination(t *testing.T) {
	name := filepath.Join(t.TempDir(), "copy.txt")
	written, err := copyToNewFile(name, strings.NewReader("original"))
	if err != nil || written != 8 {
		t.Fatalf("initial copy = (%d, %v)", written, err)
	}
	written, err = copyToNewFile(name, strings.NewReader("replacement"))
	if written != 0 || !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing destination = (%d, %v), want 0 and ErrExist", written, err)
	}
	data, err := os.ReadFile(name)
	if err != nil || string(data) != "original" {
		t.Fatalf("destination = (%q, %v), want original data", data, err)
	}
}

func TestCopyToNewFileReportsPartialOutput(t *testing.T) {
	name := filepath.Join(t.TempDir(), "partial.txt")
	failure := errors.New("source failed")
	written, err := copyToNewFile(name, io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(failure)))
	if written != 7 || !errors.Is(err, failure) {
		t.Fatalf("copy = (%d, %v), want 7 and original failure", written, err)
	}
	data, err := os.ReadFile(name)
	if err != nil || string(data) != "partial" {
		t.Fatalf("partial destination = (%q, %v)", data, err)
	}
}

func TestAppendLinesCreatesAndPreservesData(t *testing.T) {
	name := filepath.Join(t.TempDir(), "notes.txt")
	if err := appendLines(name, []string{"first"}); err != nil {
		t.Fatal(err)
	}
	if err := appendLines(name, []string{"second", "☀"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil || string(data) != "first\nsecond\n☀\n" {
		t.Fatalf("appended file = (%q, %v)", data, err)
	}

	missingParent := filepath.Join(t.TempDir(), "missing", "notes.txt")
	err = appendLines(missingParent, []string{"line"})
	var pathErr *fs.PathError
	if !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pathErr) {
		t.Fatalf("open failure must preserve cause and PathError, got %v", err)
	}
}

func TestListFilesWorksWithFilesystemInterface(t *testing.T) {
	view := fstest.MapFS{
		"root.txt": {Data: []byte("root")},
		"a/z.txt":  {Data: []byte("z")},
		"a/b.txt":  {Data: []byte("b")},
		"empty":    {Mode: fs.ModeDir | 0o700},
		"link.txt": {Mode: fs.ModeSymlink, Data: []byte("root.txt")},
	}
	got, err := listFiles(view)
	want := []string{"a/b.txt", "a/z.txt", "link.txt", "root.txt"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("entries = (%q, %v), want %q", got, err, want)
	}
	got, err = listFiles(fstest.MapFS{})
	if err != nil || len(got) != 0 {
		t.Fatalf("empty filesystem = (%q, %v)", got, err)
	}
}

type failingFS struct{ err error }

func (f failingFS) Open(string) (fs.File, error) { return nil, f.err }

func TestListFilesReportsWalkErrors(t *testing.T) {
	failure := errors.New("filesystem unavailable")
	names, err := listFiles(failingFS{failure})
	if names != nil || !errors.Is(err, failure) {
		t.Fatalf("listFiles = (%q, %v), want nil and original failure", names, err)
	}
}
