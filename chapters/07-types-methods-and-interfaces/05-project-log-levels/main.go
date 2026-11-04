// Project: Log Levels
// Topic: enum-like constants, validation, parsing, and a String method for readable output.
// Args/Input: examples use numeric levels and text names, including invalid inputs.
// Usage: run from the repository root with go run ./chapters/07-types-methods-and-interfaces/05-project-log-levels.
// Key idea: a defined type groups values; validate input before using it as a log level.
package main

import (
	"fmt"
	"strings"
)

/* Helper section 1: Log level type and values */
// LogLevel represents the severity of a message, from Trace to Fatal.
type LogLevel int

const (
	LevelTrace LogLevel = iota // The zero value is Trace in this exercise.
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// Key each array entry by its constant so the numeric-to-text mapping is explicit.
var levelNames = [...]string{
	LevelTrace: "Trace",
	LevelDebug: "Debug",
	LevelInfo:  "Info",
	LevelWarn:  "Warn",
	LevelError: "Error",
	LevelFatal: "Fatal",
}

/* Helper section 2: Validation and formatting */
// IsValid reports whether the value is one of this exercise's contiguous iota constants.
func (l LogLevel) IsValid() bool {
	return l >= LevelTrace && l <= LevelFatal
}

// String converts a LogLevel number into a readable name.
// Invalid values retain their number so diagnostics can identify the input.
func (l LogLevel) String() string {
	if !l.IsValid() {
		return fmt.Sprintf("LogLevel(%d)", int(l))
	}
	return levelNames[l]
}

/* Helper section 3: Parse names received as text */
// ParseLogLevel accepts a canonical name, ignoring case and surrounding whitespace.
// On failure it returns the zero value and false; callers must check ok before using the level.
func ParseLogLevel(text string) (LogLevel, bool) {
	name := strings.TrimSpace(text)
	for level := LevelTrace; level <= LevelFatal; level++ {
		if strings.EqualFold(name, levelNames[level]) {
			return level, true
		}
	}
	return 0, false
}

/* Helper section 4: Presentation belongs at the call site */
func printLogLevel(level LogLevel) {
	// %d prints the number; %s uses LogLevel.String through fmt.Stringer.
	fmt.Printf("Log level: %d %s (valid: %t)\n", level, level, level.IsValid())
}

func main() {
	/* Section 1: Display every supported level */
	fmt.Println("== 1. Supported levels ==")
	for level := LevelTrace; level <= LevelFatal; level++ {
		printLogLevel(level)
	}

	/* Section 2: The zero value is a deliberate choice */
	fmt.Println("\n== 2. Zero value ==")
	var defaultLevel LogLevel
	fmt.Println("An uninitialized LogLevel is:", defaultLevel)

	/* Section 3: A defined type still permits invalid numeric values */
	fmt.Println("\n== 3. Invalid numeric levels ==")
	for _, level := range []LogLevel{-1, LevelFatal + 1, 99} {
		printLogLevel(level)
	}

	/* Section 4: Validate parsed text before using the result */
	fmt.Println("\n== 4. Parse text names ==")
	for _, text := range []string{"info", " WARN ", "DeBuG", "", "verbose", "2"} {
		level, ok := ParseLogLevel(text)
		if !ok {
			fmt.Printf("Cannot parse log level %q\n", text)
			continue
		}
		fmt.Printf("Parsed %q: ", text)
		printLogLevel(level)
	}
}
