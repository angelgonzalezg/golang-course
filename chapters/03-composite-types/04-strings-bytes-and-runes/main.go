// Strings, Bytes, and Runes
// Topic: strings contain immutable bytes; UTF-8 encodes code points in those bytes.
// Input: examples include ASCII, accented text, emoji, and invalid UTF-8.
// Usage: choose byte or rune operations deliberately and use the standard library.
package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// prefixRunes returns at most limit code points, preserving their UTF-8 encoding.
// Invalid UTF-8 or a negative limit returns ("", false), with no partial result.
// A code point boundary is not necessarily a visible character boundary.
func prefixRunes(text string, limit int) (string, bool) {
	if limit < 0 || !utf8.ValidString(text) {
		return "", false
	}
	runes := []rune(text)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes), true
}

// splitAssignment splits at the first '=' and trims Unicode whitespace.
// A nonblank key is required; an empty value or more '=' in the value is valid.
func splitAssignment(line string) (string, string, bool) {
	key, value, found := strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	if !found || key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(value), true
}

func showBytesAndRunes() {
	var empty string // A string's zero value is "", not nil.
	fmt.Printf("zero string: %q, len=%d\n", empty, len(empty))

	text := "Goé🙂"
	fmt.Printf("text=%q, bytes=%d, runes=%d\n", text, len(text), utf8.RuneCountInString(text))
	// byte is an alias for uint8. Indexing yields one byte, not a whole code point.
	fmt.Printf("text[2]=0x%02x, type=%T\n", text[2], text[2])
	fmt.Printf("all bytes: % x\n", []byte(text))
	for index := 0; index < len(text); index++ {
		fmt.Printf("byte[%d]=0x%02x\n", index, text[index])
	}

	// rune is an alias for int32. range decodes UTF-8, with byte offsets as indexes.
	for offset, codePoint := range text {
		fmt.Printf("rune at byte %d: %U %q\n", offset, codePoint, codePoint)
	}
}

func showConversions() {
	original := "café"
	// original[0] = 'C' // Does not compile: string contents are immutable.
	bytes := []byte(original)
	bytes[0] = 'C'
	converted := string(bytes)
	bytes[0] = 'X'
	// Changing the slice affects neither the original nor the converted string.
	fmt.Printf("original=%q, converted=%q, bytes=%q\n", original, converted, bytes)

	runes := []rune(original)
	runes[3] = 'e' // Rune index 3 is the whole é, whose UTF-8 encoding uses 2 bytes.
	fmt.Printf("edited runes=%q, original=%q\n", string(runes), original)

	codePoint := rune(65)
	// Integer-to-string conversion encodes a code point; Itoa formats decimal digits.
	fmt.Printf("string(rune(65))=%q, strconv.Itoa(65)=%q\n", string(codePoint), strconv.Itoa(65))
}

func showSlicing() {
	text := "Goé🙂"
	// Both bounds are byte offsets. Index 3 lies inside the encoding of é.
	broken := text[:3]
	fmt.Printf("text[:3]=%q, valid UTF-8=%t\n", broken, utf8.ValidString(broken))
	fmt.Printf("text[:4]=%q, valid UTF-8=%t\n", text[:4], utf8.ValidString(text[:4]))

	prefix, ok := prefixRunes(text, 3)
	if !ok {
		fmt.Println("prefix rejected: invalid UTF-8 or negative limit")
		return
	}
	fmt.Printf("first 3 runes=%q\n", prefix)
}

func showUnicodeBoundaries() {
	composed := "\u00e9"    // é: one code point.
	decomposed := "e\u0301" // e followed by a combining acute accent.
	fmt.Printf("composed: bytes=%d, runes=%d, code points=%U\n",
		len(composed), utf8.RuneCountInString(composed), []rune(composed))
	fmt.Printf("decomposed: bytes=%d, runes=%d, code points=%U\n",
		len(decomposed), utf8.RuneCountInString(decomposed), []rune(decomposed))
	// These can look the same. == compares bytes; Go does not normalize them.
	fmt.Println("equal bytes:", composed == decomposed)
	fmt.Println("equal with EqualFold:", strings.EqualFold(composed, decomposed))
	fmt.Println("EqualFold ignores case:", strings.EqualFold("Go", "gO"))

	sun := "\u2600\ufe0f" // Sun plus variation selector: two code points.
	fmt.Printf("sun: bytes=%d, runes=%d\n", len(sun), utf8.RuneCountInString(sun))
	// Truncating at a rune boundary can still remove part of a visible character.
	prefix, ok := prefixRunes(decomposed, 1)
	if ok {
		fmt.Printf("first rune of decomposed text=%q\n", prefix)
	}
}

func showInvalidUTF8() {
	// A string may contain arbitrary bytes, including invalid UTF-8.
	raw := "A\xffB"
	fmt.Printf("raw=%q, valid UTF-8=%t, decoded runes=%d\n",
		raw, utf8.ValidString(raw), utf8.RuneCountInString(raw))
	for offset, codePoint := range raw {
		fmt.Printf("rune at byte %d: %U\n", offset, codePoint)
	}

	// Decoding substitutes U+FFFD for each invalid byte. Re-encoding changes data.
	decoded := string([]rune(raw))
	fmt.Printf("re-encoded bytes: % x, same bytes=%t\n", []byte(decoded), decoded == raw)
	// RuneError can also be a legitimate, correctly encoded replacement character.
	badRune, badWidth := utf8.DecodeRuneInString(raw[1:])
	replacement, replacementWidth := utf8.DecodeRuneInString("\ufffd")
	fmt.Printf("invalid byte: rune=%U, width=%d\n", badRune, badWidth)
	fmt.Printf("literal replacement: rune=%U, width=%d\n", replacement, replacementWidth)
}

func showStringHelpers() {
	text := " \tGo\n is\u2003fun  "
	words := strings.Fields(text) // Unicode whitespace is collapsed; no empty fields.
	fmt.Printf("Fields: %q\n", words)
	fmt.Printf("Join: %q\n", strings.Join(words, " "))
	// Split uses an exact separator and keeps these empty fields.
	fmt.Printf("Split: %q\n", strings.Split("go,,rust,", ","))
	fmt.Printf("ReplaceAll: %q\n", strings.ReplaceAll("go-go", "go", "Go"))
	fmt.Println("HasPrefix:", strings.HasPrefix("gopher", "go"))
	fmt.Println("Contains:", strings.Contains("café", "fé"))
	// Index, like range, reports a byte offset rather than a rune index.
	fmt.Println("Index of emoji in café🙂:", strings.Index("café🙂", "🙂"))

	for _, line := range []string{" language = Go ", "token=a=b", "empty=", "missing separator", " = Go"} {
		key, value, ok := splitAssignment(line)
		if !ok {
			fmt.Printf("invalid assignment: %q\n", line)
			continue
		}
		fmt.Printf("assignment: key=%q, value=%q\n", key, value)
	}
}

func showNumberConversions() {
	// Atoi parses decimal text into int; callers must check the returned error.
	// This previews the full error-handling lesson in chapter 9.
	for _, text := range []string{"42", "-7", "4two", " 42 "} {
		number, err := strconv.Atoi(text)
		if err != nil {
			fmt.Printf("Atoi(%q) failed: %v\n", text, err)
			continue
		}
		fmt.Printf("Atoi(%q)=%d; Itoa=%q\n", text, number, strconv.Itoa(number))
	}

	// ParseInt selects the base and signed bit width; here the result is int64.
	number, err := strconv.ParseInt("ff", 16, 64)
	if err != nil {
		fmt.Println("parse hexadecimal number:", err)
		return
	}
	fmt.Printf("hexadecimal ff=%d; FormatInt base 16=%q\n", number, strconv.FormatInt(number, 16))
}

func main() {
	fmt.Println("== 1. Bytes and runes ==")
	showBytesAndRunes()
	fmt.Println("\n== 2. Immutability and conversions ==")
	showConversions()
	fmt.Println("\n== 3. Byte slicing and rune prefixes ==")
	showSlicing()
	fmt.Println("\n== 4. Code points and visible characters ==")
	showUnicodeBoundaries()
	fmt.Println("\n== 5. Invalid UTF-8 ==")
	showInvalidUTF8()
	fmt.Println("\n== 6. Standard string helpers ==")
	showStringHelpers()
	fmt.Println("\n== 7. Numeric text conversions ==")
	showNumberConversions()
}
