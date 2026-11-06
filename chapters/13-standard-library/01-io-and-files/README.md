# I/O and files

Practice byte streams, bounded input, buffering, files, filesystem interfaces,
and cleanup. This starts chapter 13's standard library lessons and builds on
interfaces, error wrapping, and `defer`.

## Run and validate

Run from the repository root:

```sh
go run ./chapters/13-standard-library/01-io-and-files
go test ./chapters/13-standard-library/01-io-and-files
go vet ./chapters/13-standard-library/01-io-and-files
```

The program creates a fresh temporary directory, closes its files, and removes
that directory before returning successfully. It does not need input files or a
particular working directory. All example data is constructed in the program.

## Read the example

- [main.go](main.go) runs eleven sections, including normal results and expected
  failures. Unexpected errors reach `main` after deferred cleanup finishes.
- [streams.go](streams.go) counts bytes in chunks, reads a bounded document,
  processes lines, and writes buffered output.
- [files.go](files.go) streams into new files, appends lines, closes owned handles,
  and walks an `fs.FS`.
- [streams_test.go](streams_test.go) checks short reads, data returned with an
  error, size boundaries, line content, and input/output failures.
- [files_test.go](files_test.go) checks ownership, combined copy/close failures,
  existing destinations, partial files, append behavior, and filesystem errors.

## Choose the operation

| Operation | When to use it | What to account for |
| --- | --- | --- |
| `ReadAll` / `os.ReadFile` | A whole small result is needed and its size is acceptable. | Result memory grows with the input. `ReadFile` has no application size limit; this lesson wraps `ReadAll` with one. |
| `io.Copy` | Transfer a stream without collecting it all first. | Check its count and error; it does not close either endpoint. A `bytes.Buffer` destination still retains the entire output. |
| A `Read` loop | Each chunk requires processing, such as byte counting. | Process `p[:n]` before handling the error, and handle partial progress. |
| `io.ReadFull` | A protocol requires an exact number of bytes, such as a fixed header. | Distinguish no bytes at EOF from an incomplete field. |
| `bufio.Scanner` | Process bounded lines or other tokens conveniently. | Configure the token buffer, check `Err`, and account for read-ahead. |
| `bufio.Reader` | Control delimiters or handle fragments explicitly. | `ReadString` retains delimiters and may allocate a whole line; `ReadSlice` allows fragment processing with explicit bounds. |
| `bufio.Writer` | Combine small writes to an underlying destination. | Check write errors and explicitly check `Flush` before closing the destination. |

The interfaces let the same code work with strings, memory buffers, and files.
`countBytes`, `readDocument`, and `forEachLine` receive only the reading capability
they need. `writeLines` needs only writing. These helpers borrow their inputs;
they neither assume a concrete file nor close the caller's resources.

## Bytes, progress, and exhaustion

`Read` can return fewer bytes than the buffer's capacity without reaching EOF.
It may also return data together with an error. `countBytes` processes that data
first, including when the error is EOF. A non-EOF failure returns the count of
bytes already observed plus the wrapped cause. A zero-length successful read
does not by itself indicate exhaustion.

These are byte operations. `Go ☀` contains six bytes, although it has four runes.
Chunk boundaries can split a UTF-8 encoding; counting bytes is unaffected, while
text decoding would need to preserve incomplete encodings across chunks.

`io.Copy` treats ordinary EOF as successful completion. `ReadFull` returns EOF
when it gets no bytes, or `io.ErrUnexpectedEOF` after an incomplete field. The
example's four-byte header accepts `GO01` and rejects the two-byte `GO` input.

## Bound whole documents and line tokens deliberately

`readDocument` permits at most 1,024 bytes. It reads through a `LimitReader` with
an allowance of 1,025 bytes so an exact fit succeeds and an oversized input is
rejected. Reading only the allowed size would silently accept a truncated prefix.
The extra byte is consumed on rejection. This is a complete-document API: any
failure returns a nil document, and a large source is not drained to exhaustion.
It is not a byte-framing API for several documents on the same stream.

`forEachLine` configures a 4 KiB scanner buffer limit before scanning. That limit
includes room needed to recognize a delimiter, so it is not an exact allowance
of 4 KiB of line payload. The small initial buffer can grow up to that limit.
Oversized tokens fail with `bufio.ErrTooLong`; checking only `Scan` would miss the
failure. Line splitting removes LF and an optional preceding CR, preserves empty
lines, and returns a final nonempty line even without a terminating newline.

The callback receives `Scanner.Text`, which can be retained. A `Scanner.Bytes`
slice would need copying if it must survive a subsequent scan. Completed callback
effects remain when later input or processing fails. Scanner can read ahead, so
stopping a callback does not leave the source positioned exactly after that line.

The separate `ReadString` example preserves `first\n` and processes `last` before
its accompanying EOF. Choose it when delimiter control is useful and a complete
fragment fits memory. For larger records, `ReadSlice` can report
`bufio.ErrBufferFull`; process each borrowed fragment and enforce a total bound.
Simply replacing Scanner with `ReadString` would remove the scanner's size bound.

## Buffering, cleanup, and ownership

Writing `queued` to a new buffered writer leaves the underlying buffer empty
until `Flush`. File closure cannot flush bytes still held by that separate writer.
`appendLines` therefore waits for `writeLines` to flush, then closes its handle.
A successful flush passes bytes to the underlying writer; disk durability would
require a separate persistence policy, including `File.Sync` where appropriate.

| Helper | Ownership and failure contract |
| --- | --- |
| `countBytes` | Borrows its reader; returns a partial count on failure. |
| `readDocument` | Borrows its reader; returns no usable document on failure. |
| `forEachLine` | Borrows its reader; prior callback effects remain on failure. |
| `writeLines` | Borrows its writer; flushes on success; partial output can remain on failure. |
| `copyAndClose` | Borrows the source and takes ownership of the destination; always closes the destination once. |
| `copyToNewFile` | Opens a destination and transfers its handle to `copyAndClose`; borrows the source. |
| `appendLines` | Opens, flushes, and closes its own file. |

Named error returns let deferred cleanup report failures. `errors.Join` retains
both an earlier operation error and a close error; tests inspect both with
`errors.Is`. The lesson uses explicit ownership comments because accepting a
`Reader` or `Writer` does not itself specify resource ownership.

Keep `defer Close` near a successful open in a function with a bounded lifetime.
For many files in a loop, process each file in a helper so cleanup runs per file
rather than accumulating open handles until the outer function returns. `run`
owns directory cleanup; `main` calls `os.Exit` only after `run` returns, because
`os.Exit` does not run deferred calls.

## File modes, paths, and filesystem views

`os.WriteFile` creates a missing file or truncates an existing one. The append
helper uses `O_WRONLY | O_CREATE | O_APPEND` to keep old contents. The streaming
copy uses `O_WRONLY | O_CREATE | O_EXCL` to reject an existing destination. `O_EXCL`
avoids a separate existence check followed by creation; errors remain inspectable
with `errors.Is(err, fs.ErrExist)`.

These examples do not provide an atomic file replacement. An interrupted or
failed write can leave partial content, which one test deliberately verifies.
Append mode also does not make a multi-write logical record a transaction.
Creation requests `0o600` for files and `0o700` for the notes directory; OS rules
and the process umask govern effective permissions. A creation mode does not
change an existing file's permissions.

Use `filepath.Join` for host filesystem paths. Within `fs.FS`, use relative names
with `/`, such as `notes/source.txt`; `.` denotes its root. `listFiles` accepts
`os.DirFS` in the runnable example and `fstest.MapFS` in tests, while chapter 11's
`embed.FS` implements the same interface. It lists non-directory entries in
lexical traversal order and includes symbolic links without following them.
`os.DirFS` does not prevent symlinks from resolving outside its directory; the
lesson uses only its own newly created files.

Use `errors.Is(err, fs.ErrNotExist)` to identify a missing file independently of
the platform's message. `errors.As` exposes the underlying `*fs.PathError` when
its operation and path are needed. Avoid branching on error strings.

References: [`io`](https://pkg.go.dev/io), [`bufio`](https://pkg.go.dev/bufio),
[`os`](https://pkg.go.dev/os), [`io/fs`](https://pkg.go.dev/io/fs),
and [`path/filepath`](https://pkg.go.dev/path/filepath).
