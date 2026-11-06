package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// copyAndClose takes ownership of dst and always closes it, including on failure.
// It borrows src. io.Copy returns the bytes written and a nil error on normal EOF;
// the returned count may describe partial output when a read, write, or close fails.
func copyAndClose(dst io.WriteCloser, src io.Reader) (written int64, err error) {
	defer func() {
		if closeErr := dst.Close(); closeErr != nil {
			// Deferred cleanup must not hide an earlier copy error.
			err = errors.Join(err, fmt.Errorf("close destination: %w", closeErr))
		}
	}()

	written, err = io.Copy(dst, src)
	if err != nil {
		return written, fmt.Errorf("copy stream: %w", err)
	}
	return written, nil
}

// copyToNewFile streams into a new file without retaining the whole source.
// Use O_EXCL when an existing destination must be preserved. The caller owns src.
// A failure after creation can leave a partial file; this is not an atomic save.
func copyToNewFile(name string, src io.Reader) (int64, error) {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create destination %q: %w", name, err)
	}
	// Ownership of this handle moves to copyAndClose; do not close it twice.
	written, err := copyAndClose(file, src)
	if err != nil {
		return written, fmt.Errorf("save %q: %w", name, err)
	}
	return written, nil
}

// appendLines creates a file if needed and appends without truncating old data.
// It owns the opened handle. No concurrent writers are used in this exercise;
// one logical record may require several writes and is not a transaction.
func appendLines(name string, lines []string) (err error) {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open append file %q: %w", name, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close append file %q: %w", name, closeErr))
		}
	}()

	// writeLines flushes before the deferred Close runs. Closing the file alone
	// would not flush bytes still held by a bufio.Writer.
	return writeLines(file, lines)
}

// listFiles accepts any fs.FS, including os.DirFS, embed.FS, or fstest.MapFS.
// It lists non-directory entries in traversal order; WalkDir does not follow
// symbolic links. Names use '/' within the filesystem, independent of the OS.
func listFiles(fsys fs.FS) ([]string, error) {
	var names []string
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		// On failure entry may be nil; inspect the error before using entry.
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk files: %w", err)
	}
	return names, nil
}
