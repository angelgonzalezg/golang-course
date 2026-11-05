package main

import (
	"math"
	"testing"
)

func TestPrefixRunes(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
		ok    bool
	}{
		{"empty text", "", 3, "", true},
		{"empty text and zero limit", "", 0, "", true},
		{"zero prefix", "Goé🙂", 0, "", true},
		{"ASCII", "gopher", 2, "go", true},
		{"before multibyte rune", "Goé🙂", 2, "Go", true},
		{"accented rune", "Goé🙂", 3, "Goé", true},
		{"emoji", "Goé🙂", 4, "Goé🙂", true},
		{"limit exceeds length", "Goé🙂", 10, "Goé🙂", true},
		{"maximum limit", "Goé🙂", math.MaxInt, "Goé🙂", true},
		{"non-Latin text", "日本語", 2, "日本", true},
		{"embedded zero byte", "A\x00B", 2, "A\x00", true},
		{"valid replacement character", "\ufffd!", 1, "\ufffd", true},
		{"whole combining sequence", "e\u0301!", 2, "e\u0301", true},
		{"rune boundary inside visible character", "e\u0301!", 1, "e", true},
		{"variation selector omitted", "\u2600\ufe0f", 1, "\u2600", true},
		{"negative limit", "Go", -1, "", false},
		{"negative limit on empty text", "", -1, "", false},
		{"invalid byte", "A\xffB", 2, "", false},
		{"incomplete encoding", "Go\xc3", 3, "", false},
		{"invalid byte beyond prefix", "Go\xff", 1, "", false},
		{"zero limit still validates input", "\xff", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := prefixRunes(tt.text, tt.limit)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("prefixRunes(%q, %d) = (%q, %t), want (%q, %t)",
					tt.text, tt.limit, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSplitAssignment(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		key   string
		value string
		ok    bool
	}{
		{"ordinary assignment", "language=Go", "language", "Go", true},
		{"trim whitespace", " language = Go ", "language", "Go", true},
		{"Unicode whitespace", "\t name\u2003= café🙂 \u2003", "name", "café🙂", true},
		{"first separator only", "token=a=b", "token", "a=b", true},
		{"separator as value", "token==", "token", "=", true},
		{"empty value", "empty=", "empty", "", true},
		{"blank value", "empty= \t\u2003", "empty", "", true},
		{"preserve internal spaces", "name=Ana  María", "name", "Ana  María", true},
		{"preserve key case", "Language=Go", "Language", "Go", true},
		{"empty input", "", "", "", false},
		{"missing separator", "language Go", "", "", false},
		{"empty key", "=Go", "", "", false},
		{"blank key", " \t\u2003=Go", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, ok := splitAssignment(tt.line)
			if key != tt.key || value != tt.value || ok != tt.ok {
				t.Fatalf("splitAssignment(%q) = (%q, %q, %t), want (%q, %q, %t)",
					tt.line, key, value, ok, tt.key, tt.value, tt.ok)
			}
		})
	}
}

// Executable examples protect the byte counts, offsets, and decoding behavior
// taught by the walkthrough, without duplicating its implementations.
func Example_byteOffsets() {
	showBytesAndRunes()
	// Output:
	// zero string: "", len=0
	// text="Goé🙂", bytes=8, runes=4
	// text[2]=0xc3, type=uint8
	// all bytes: 47 6f c3 a9 f0 9f 99 82
	// byte[0]=0x47
	// byte[1]=0x6f
	// byte[2]=0xc3
	// byte[3]=0xa9
	// byte[4]=0xf0
	// byte[5]=0x9f
	// byte[6]=0x99
	// byte[7]=0x82
	// rune at byte 0: U+0047 'G'
	// rune at byte 1: U+006F 'o'
	// rune at byte 2: U+00E9 'é'
	// rune at byte 4: U+1F642 '🙂'
}

func Example_conversions() {
	showConversions()
	// Output:
	// original="café", converted="Café", bytes="Xafé"
	// edited runes="cafe", original="café"
	// string(rune(65))="A", strconv.Itoa(65)="65"
}

func Example_slicing() {
	showSlicing()
	// Output:
	// text[:3]="Go\xc3", valid UTF-8=false
	// text[:4]="Goé", valid UTF-8=true
	// first 3 runes="Goé"
}

func Example_unicodeBoundaries() {
	showUnicodeBoundaries()
	// Output:
	// composed: bytes=2, runes=1, code points=[U+00E9]
	// decomposed: bytes=3, runes=2, code points=[U+0065 U+0301]
	// equal bytes: false
	// equal with EqualFold: false
	// EqualFold ignores case: true
	// sun: bytes=6, runes=2
	// first rune of decomposed text="e"
}

func Example_invalidUTF8() {
	showInvalidUTF8()
	// Output:
	// raw="A\xffB", valid UTF-8=false, decoded runes=3
	// rune at byte 0: U+0041
	// rune at byte 1: U+FFFD
	// rune at byte 2: U+0042
	// re-encoded bytes: 41 ef bf bd 42, same bytes=false
	// invalid byte: rune=U+FFFD, width=1
	// literal replacement: rune=U+FFFD, width=3
}
