// Project: Log Levels
// Topic: enum-like constants with iota and a String method for readable output.
// Args/Input: printLoglevel receives a LogLevel value.
// Usage: map numeric log levels to labels like Trace, Info, Error, etc.
// Key idea: iota plus String() turns numeric levels into readable labels.
package main

import "fmt"

/* Helper section 1: Log level type and values */
type LogLevel int

const (
	LevelTrace LogLevel = iota // iota auto-increments each constant
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = []string{"Trace", "Debug", "Info", "Warn", "Error", "Fatal"} // Index matches LogLevel

// String converts a LogLevel number into a readable name.
func (l LogLevel) String() string {
	if l < LevelTrace || l > LevelFatal {
		return "Unknown"
	}
	return levelNames[l]
}

/* Helper section 2: Log level printing helper */
func printLoglevel(level LogLevel) {
	fmt.Printf("Log level: %d %s\n", level, level.String())
}

func main() {
	printLoglevel(LevelTrace)
	printLoglevel(LevelInfo)
}
