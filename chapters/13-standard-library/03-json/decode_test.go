package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

func requireNoTask(t *testing.T, task Task) {
	t.Helper()
	if task.Title != "" || task.Notifications || task.Tags != nil || task.State != 0 {
		t.Fatalf("failure exposed a partial task: %+v", task)
	}
}

func TestReadTaskDefaultAndExplicitChoices(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		notifications bool
		tags          []string
	}{
		{"missing", `{"title":"  Learn ☀  "}`, true, []string{}},
		{"null", `{"title":"Learn ☀","notifications":null,"tags":null}`, true, []string{}},
		{"false", `{"title":"Learn ☀","notifications":false}`, false, []string{}},
		{"true", `{"title":"Learn ☀","notifications":true,"tags":["Go"]}`, true, []string{"Go"}},
		{"surrounding whitespace", " \n{\"title\":\"Learn ☀\"}\t\r\n", true, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := readTask(iotest.OneByteReader(strings.NewReader(tt.input)))
			if err != nil {
				t.Fatal(err)
			}
			if task.Title != "Learn ☀" || task.Notifications != tt.notifications || task.State != StatePending {
				t.Fatalf("unexpected task: %+v", task)
			}
			if task.Tags == nil || !slices.Equal(task.Tags, tt.tags) {
				t.Fatalf("tags = %#v, want non-nil %v", task.Tags, tt.tags)
			}
		})
	}
}

func TestReadTaskRejectsUnknownNamesIncludingWrongCase(t *testing.T) {
	for _, input := range []string{
		`{"title":"valid","unexpected":true}`,
		`{"Title":"wrong case"}`,
		`{"title":"valid","state":"done"}`,
	} {
		task, err := readTask(strings.NewReader(input))
		if !errors.Is(err, json.ErrUnknownName) {
			t.Fatalf("%s: want unknown member cause, got %v", input, err)
		}
		requireNoTask(t, task)
	}
}

func TestReadTaskRejectsDuplicateNamesAndTrailingDocuments(t *testing.T) {
	for _, input := range []string{
		`{"title":"first","title":"second"}`,
		`{"title":"first","\u0074itle":"second"}`,
		`{"title":"valid","tags":[],"tags":["duplicate"]}`,
		`{"title":"valid"} {"title":"second"}`,
		`{"title":"valid"} null`,
		`{"title":"valid"} garbage`,
		`{"title":"unfinished"`,
		"{\"title\":\"\xff\"}",
	} {
		task, err := readTask(strings.NewReader(input))
		var syntaxErr *jsontext.SyntacticError
		if !errors.As(err, &syntaxErr) {
			t.Fatalf("%s: want wrapped syntax error, got %v", input, err)
		}
		requireNoTask(t, task)
	}
}

func TestReadTaskRequiresAnObjectAndValidDomainData(t *testing.T) {
	tests := []struct {
		input string
		cause error
	}{
		{`null`, ErrObjectRequired},
		{`{}`, ErrInvalidTask},
		{`{"title":"\t  "}`, ErrInvalidTask},
		{`{"title":null}`, ErrInvalidTask},
	}
	for _, tt := range tests {
		task, err := readTask(strings.NewReader(tt.input))
		if !errors.Is(err, tt.cause) {
			t.Fatalf("%s: want %v, got %v", tt.input, tt.cause, err)
		}
		requireNoTask(t, task)
	}
	for _, input := range []string{`[]`, `42`, `true`, `"text"`, "", " \n"} {
		task, err := readTask(strings.NewReader(input))
		if err == nil {
			t.Fatalf("non-object or empty input accepted: %q", input)
		}
		requireNoTask(t, task)
	}
}

func TestReadTaskPreservesTypeErrorDetailsWithoutPartialResults(t *testing.T) {
	for _, input := range []string{
		`{"title":42}`,
		`{"title":"already decoded","notifications":"false"}`,
		`{"title":"already decoded","tags":[1]}`,
	} {
		task, err := readTask(strings.NewReader(input))
		var semanticErr *json.SemanticError
		if !errors.As(err, &semanticErr) || semanticErr.JSONPointer == "" {
			t.Fatalf("%s: want wrapped semantic error with field location, got %v", input, err)
		}
		requireNoTask(t, task)
	}
}

func TestReadTaskEnforcesTheByteLimitIncludingWhitespace(t *testing.T) {
	const document = `{"title":"Learn ☀"}`
	exactFit := document + strings.Repeat(" ", maxJSONBytes-len(document))
	if task, err := readTask(strings.NewReader(exactFit)); err != nil || task.Title != "Learn ☀" {
		t.Fatalf("exact fit rejected: %+v, %v", task, err)
	}
	for _, input := range []string{exactFit + " ", strings.Repeat("x", maxJSONBytes+1)} {
		task, err := readTask(strings.NewReader(input))
		if !errors.Is(err, ErrJSONTooLarge) {
			t.Fatalf("oversized input: want ErrJSONTooLarge, got %v", err)
		}
		requireNoTask(t, task)
	}
	source := strings.NewReader(exactFit + strings.Repeat(" ", maxJSONBytes))
	_, err := readTask(source)
	if !errors.Is(err, ErrJSONTooLarge) || source.Len() != maxJSONBytes-1 {
		t.Fatalf("overflow probe consumed too much: unread=%d, err=%v", source.Len(), err)
	}
}

type borrowedJSONReader struct {
	io.Reader
	closes int
}

func (r *borrowedJSONReader) Close() error {
	r.closes++
	return nil
}

func TestReadTaskPreservesReadFailuresAndBorrowsTheReader(t *testing.T) {
	failure := errors.New("input failed")
	source := &borrowedJSONReader{Reader: io.MultiReader(strings.NewReader(`{"title":"complete-looking prefix"}`), iotest.ErrReader(failure))}
	task, err := readTask(source)
	if !errors.Is(err, failure) {
		t.Fatalf("lost input failure: %v", err)
	}
	requireNoTask(t, task)
	if source.closes != 0 {
		t.Fatal("readTask closed a borrowed reader")
	}

	source = &borrowedJSONReader{Reader: strings.NewReader(`{"title":"valid"}`)}
	if _, err := readTask(source); err != nil || source.closes != 0 {
		t.Fatalf("successful read must also borrow the source: closes=%d, err=%v", source.closes, err)
	}
}
