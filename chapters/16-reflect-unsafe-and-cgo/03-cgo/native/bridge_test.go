// Test the ordinary Go API; cgo compiles the actual C implementation with it.
// _test.go files do not need or import the pseudo-package C.
package native

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"testing"
)

func TestAdd(t *testing.T) {
	for _, tt := range []struct {
		name string
		a, b int32
		want int32
		err  error
	}{
		{name: "positive", a: 20, b: 22, want: 42},
		{name: "negative", a: -20, b: -22, want: -42},
		{name: "zero"},
		{name: "maximum", a: math.MaxInt32, want: math.MaxInt32},
		{name: "minimum", a: math.MinInt32, want: math.MinInt32},
		{name: "cancellation", a: math.MaxInt32, b: math.MinInt32, want: -1},
		{name: "positive overflow", a: math.MaxInt32, b: 1, err: ErrOverflow},
		{name: "negative overflow", a: math.MinInt32, b: -1, err: ErrOverflow},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // Calls share no mutable C state.
			got, err := Add(tt.a, tt.b)
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Fatalf("Add(%d, %d)=(%d, %v), want (%d, %v)", tt.a, tt.b, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestSumBytes(t *testing.T) {
	backing := []byte{100, 1, 0, 2, 255, 100}
	for _, tt := range []struct {
		name string
		data []byte
		want uint64
	}{
		{name: "nil"},
		{name: "empty", data: []byte{}},
		{name: "binary", data: []byte{1, 0, 2, 255}, want: 258},
		{name: "subslice", data: backing[1:5], want: 258},
		{name: "empty with capacity", data: backing[:0]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := slices.Clone(tt.data)
			if got := SumBytes(tt.data); got != tt.want {
				t.Fatalf("sum=%d, want %d", got, tt.want)
			}
			if !slices.Equal(tt.data, before) {
				t.Fatal("C changed the borrowed input")
			}
		})
	}
	if !slices.Equal(backing, []byte{100, 1, 0, 2, 255, 100}) {
		t.Fatal("C changed storage outside the visible slice")
	}
}

func TestGreet(t *testing.T) {
	for _, tt := range []struct {
		name string
		want string
		err  error
	}{
		{name: "", want: "Hello, "},
		{name: "Ana", want: "Hello, Ana"},
		{name: "niño 🐹", want: "Hello, niño 🐹"},
		{name: "\xff", want: "Hello, \xff"}, // Byte copying does not validate UTF-8.
		{name: "\x00Ana", err: ErrEmbeddedNUL},
		{name: "Ana\x00Hidden", err: ErrEmbeddedNUL},
		{name: "Ana\x00", err: ErrEmbeddedNUL},
	} {
		got, err := Greet(tt.name)
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("Greet(%q)=(%q, %v), want (%q, %v)", tt.name, got, err, tt.want, tt.err)
		}
	}
}

func TestGreetResultsSurviveNativeCleanupAndLaterCalls(t *testing.T) {
	results := make([]string, 50)
	for index := range results {
		result, err := Greet(strconv.Itoa(index))
		if err != nil {
			t.Fatal(err)
		}
		results[index] = result
	}
	for index, result := range results {
		if want := "Hello, " + strconv.Itoa(index); result != want {
			t.Fatalf("retained result=%q, want %q", result, want)
		}
	}
}

func TestReverseCopy(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		want []byte
	}{
		{name: "nil"},
		{name: "empty", data: []byte{}},
		{name: "single", data: []byte{7}, want: []byte{7}},
		{name: "odd", data: []byte{1, 2, 3}, want: []byte{3, 2, 1}},
		{name: "binary", data: []byte{1, 0, 2, 255}, want: []byte{255, 2, 0, 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := slices.Clone(tt.data)
			got, err := ReverseCopy(tt.data)
			if err != nil || !slices.Equal(got, tt.want) {
				t.Fatalf("reversed=(%v, %v), want %v", got, err, tt.want)
			}
			if len(tt.data) == 0 && got != nil {
				t.Fatal("empty input must return nil")
			}
			if len(got) > 0 {
				got[0] ^= 255
			}
			if !slices.Equal(tt.data, before) {
				t.Fatal("reversing or changing the returned copy changed input")
			}
		})
	}
}

func TestCopyLengthLimitWithoutLargeAllocations(t *testing.T) {
	for _, tt := range []struct {
		length int64
		err    error
	}{
		{length: -1, err: ErrCopyLength},
		{length: 0},
		{length: 1},
		{length: math.MaxInt32},
		{length: int64(math.MaxInt32) + 1, err: ErrCopyLength},
	} {
		if err := validateCopyLength(tt.length); !errors.Is(err, tt.err) {
			t.Errorf("length %d: error=%v, want %v", tt.length, err, tt.err)
		}
	}
}
