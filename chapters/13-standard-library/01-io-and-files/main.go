// I/O and files
// Topic: compose byte streams, buffer deliberately, and own resource cleanup.
// Usage: go run ./chapters/13-standard-library/01-io-and-files
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func showStreams() error {
	fmt.Println("== 1. Readers and writers compose ==")
	// Use an in-memory reader and buffer for small constructed input and output.
	// The same io.Copy call can accept files because it needs only the interfaces.
	var output bytes.Buffer
	written, err := io.Copy(&output, strings.NewReader("Go composes streams.\n"))
	if err != nil {
		return fmt.Errorf("copy in memory: %w", err)
	}
	fmt.Printf("copied %d bytes: %q\n", written, output.String())

	fmt.Println("\n== 2. Read does not promise to fill the buffer ==")
	buffer := make([]byte, 4)
	reader := strings.NewReader("Go")
	n, readErr := reader.Read(buffer)
	fmt.Printf("first Read: n=%d data=%q err=%v\n", n, buffer[:n], readErr)
	n, readErr = reader.Read(buffer)
	fmt.Printf("next Read: n=%d EOF=%t\n", n, readErr == io.EOF)
	total, err := countBytes(strings.NewReader("Go ☀"))
	if err != nil {
		return err
	}
	fmt.Println("byte count, not rune count:", total)

	fmt.Println("\n== 3. ReadFull for fixed-size fields ==")
	// Use ReadFull for a fixed-size header; ordinary Read can return fewer bytes.
	header := make([]byte, 4)
	n, err = io.ReadFull(strings.NewReader("GO01"), header)
	if err != nil {
		return fmt.Errorf("read complete header: %w", err)
	}
	fmt.Printf("complete header: n=%d data=%q\n", n, header)
	n, err = io.ReadFull(strings.NewReader("GO"), header)
	if err != io.ErrUnexpectedEOF {
		return fmt.Errorf("expected an incomplete header, got %v", err)
	}
	fmt.Printf("incomplete header: n=%d data=%q unexpected EOF=%t\n", n, header[:n], err == io.ErrUnexpectedEOF)

	fmt.Println("\n== 4. ReadAll with an application size limit ==")
	document, err := readDocument(strings.NewReader("small document"))
	if err != nil {
		return err
	}
	fmt.Printf("document: %q\n", document)
	_, err = readDocument(strings.NewReader(strings.Repeat("x", maxDocumentBytes+1)))
	if !errors.Is(err, ErrDocumentTooLarge) {
		return fmt.Errorf("expected an oversized document, got %v", err)
	}
	fmt.Println("oversized document rejected:", errors.Is(err, ErrDocumentTooLarge))

	fmt.Println("\n== 5. Scan bounded lines, including the last unterminated line ==")
	err = forEachLine(strings.NewReader("Go\r\n\n☀\nlast"), func(line string) error {
		fmt.Printf("line: %q\n", line)
		return nil
	})
	if err != nil {
		return err
	}
	err = forEachLine(strings.NewReader(strings.Repeat("x", maxScanBytes+1)), func(string) error {
		return nil
	})
	if !errors.Is(err, bufio.ErrTooLong) {
		return fmt.Errorf("expected a scanner size failure, got %v", err)
	}
	fmt.Println("oversized token rejected:", errors.Is(err, bufio.ErrTooLong))

	fmt.Println("\n== 6. ReadString preserves delimiters and the final fragment ==")
	// Use Reader for delimiter control. ReadString allocates a whole fragment;
	// for large or untrusted records, process ReadSlice fragments with a bound.
	lineReader := bufio.NewReader(strings.NewReader("first\nlast"))
	for {
		fragment, readErr := lineReader.ReadString('\n')
		if len(fragment) > 0 {
			fmt.Printf("fragment: %q\n", fragment)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read delimited text: %w", readErr)
		}
	}

	fmt.Println("\n== 7. Buffered writes need Flush ==")
	var destination bytes.Buffer
	buffered := bufio.NewWriter(&destination)
	if _, err := io.WriteString(buffered, "queued"); err != nil {
		return fmt.Errorf("buffer output: %w", err)
	}
	fmt.Println("underlying bytes before Flush:", destination.Len())
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("flush output: %w", err)
	}
	fmt.Printf("underlying output after Flush: %q\n", destination.String())
	return nil
}

func showFiles(directory string) (err error) {
	fmt.Println("\n== 8. Small files, append mode, and streaming copies ==")
	notesDirectory := filepath.Join(directory, "notes")
	if err := os.Mkdir(notesDirectory, 0o700); err != nil {
		return fmt.Errorf("create notes directory: %w", err)
	}
	name := filepath.Join(notesDirectory, "source.txt")
	// Use ReadFile/WriteFile for known-small files. WriteFile creates or truncates;
	// use the append helper when old contents must be kept. It is not an atomic save.
	if err := os.WriteFile(name, []byte("first\n"), 0o600); err != nil {
		return fmt.Errorf("write initial note: %w", err)
	}
	if err := appendLines(name, []string{"second", "☀"}); err != nil {
		return err
	}
	content, err := os.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read small note: %w", err)
	}
	fmt.Printf("after append: %q\n", content)

	input, err := os.Open(name)
	if err != nil {
		return fmt.Errorf("open copy source: %w", err)
	}
	// This function opened input and owns closing it. copyToNewFile only borrows it.
	defer func() {
		if closeErr := input.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close copy source: %w", closeErr))
		}
	}()
	written, err := copyToNewFile(filepath.Join(directory, "copy.txt"), input)
	if err != nil {
		return err
	}
	fmt.Println("bytes streamed to a new file:", written)

	fmt.Println("\n== 9. A filesystem view and portable names ==")
	// Use filepath.Join for OS paths. Within fs.FS use slash-separated relative
	// names such as notes/source.txt. DirFS is a view, not a symlink sandbox.
	view := os.DirFS(directory)
	names, err := listFiles(view)
	if err != nil {
		return err
	}
	fmt.Println("filesystem entries:", names)
	data, err := fs.ReadFile(view, "notes/source.txt")
	if err != nil {
		return fmt.Errorf("read filesystem entry: %w", err)
	}
	fmt.Println("filesystem content matches:", bytes.Equal(content, data))

	fmt.Println("\n== 10. Inspect file errors by cause ==")
	_, err = os.ReadFile(filepath.Join(directory, "missing.txt"))
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("expected a missing file, got %v", err)
	}
	fmt.Println("missing file:", errors.Is(err, fs.ErrNotExist))
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		fmt.Println("failed operation:", pathErr.Op)
	}
	return nil
}

func run() (err error) {
	if err := showStreams(); err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "golang-course-io-*")
	if err != nil {
		return fmt.Errorf("create lesson directory: %w", err)
	}
	// Remove only the fresh directory this run owns, after showFiles closes its
	// handles. Keeping cleanup inside run lets it finish before main calls Exit.
	defer func() {
		if cleanupErr := os.RemoveAll(directory); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove lesson directory: %w", cleanupErr))
		}
	}()
	if err := showFiles(directory); err != nil {
		return err
	}

	fmt.Println("\n== 11. I/O rules ==")
	fmt.Println("Use ReadAll/ReadFile for bounded small data; stream larger input with io.Copy or chunk processing.")
	fmt.Println("Use Scanner for bounded tokens; use bufio.Reader when you need explicit delimiter and size handling.")
	fmt.Println("Flush buffered output, check failures, and close resources according to ownership.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "I/O lesson:", err)
		os.Exit(1)
	}
	fmt.Println("Temporary lesson directory removed.")
}
