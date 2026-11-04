package main

import (
	"strconv"
	"testing"
)

func TestLogLevelStringAndValidity(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	tests := []struct {
		name      string
		level     LogLevel
		wantText  string
		wantValid bool
	}{
		{name: "trace is the zero value", level: 0, wantText: "Trace", wantValid: true},
		{name: "debug", level: LevelDebug, wantText: "Debug", wantValid: true},
		{name: "info", level: LevelInfo, wantText: "Info", wantValid: true},
		{name: "warn", level: LevelWarn, wantText: "Warn", wantValid: true},
		{name: "error", level: LevelError, wantText: "Error", wantValid: true},
		{name: "fatal", level: LevelFatal, wantText: "Fatal", wantValid: true},
		{name: "below range", level: -1, wantText: "LogLevel(-1)"},
		{name: "above range", level: LevelFatal + 1, wantText: "LogLevel(6)"},
		{name: "unknown value", level: 99, wantText: "LogLevel(99)"},
		{name: "minimum int", level: LogLevel(minInt), wantText: "LogLevel(" + strconv.Itoa(minInt) + ")"},
		{name: "maximum int", level: LogLevel(maxInt), wantText: "LogLevel(" + strconv.Itoa(maxInt) + ")"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.level.IsValid(); got != tt.wantValid {
				t.Errorf("LogLevel(%d).IsValid() = %t, want %t", int(tt.level), got, tt.wantValid)
			}
			if got := tt.level.String(); got != tt.wantText {
				t.Errorf("LogLevel(%d).String() = %q, want %q", int(tt.level), got, tt.wantText)
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{input: "Trace", want: LevelTrace},
		{input: "Debug", want: LevelDebug},
		{input: "Info", want: LevelInfo},
		{input: "Warn", want: LevelWarn},
		{input: "Error", want: LevelError},
		{input: "Fatal", want: LevelFatal},
		{input: "info", want: LevelInfo},
		{input: "WARN", want: LevelWarn},
		{input: "DeBuG", want: LevelDebug},
		{input: " \t error\n", want: LevelError},
		{input: "\u00a0fatal\u00a0", want: LevelFatal},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, ok := ParseLogLevel(tt.input)
			if !ok || level != tt.want {
				t.Fatalf("ParseLogLevel(%q) = (%d, %t), want (%d, true)", tt.input, int(level), ok, int(tt.want))
			}
		})
	}
}

func TestParseLogLevelRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", " \t\n", "verbose", "warning", "0", "2", "6", "-1", "LogLevel(99)", "info debug", "in fo"} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			if level, ok := ParseLogLevel(input); ok {
				t.Errorf("ParseLogLevel(%q) accepted invalid input as %d", input, int(level))
			}
		})
	}
}

func TestLogLevelNamesRoundTrip(t *testing.T) {
	for level := LevelTrace; level <= LevelFatal; level++ {
		parsed, ok := ParseLogLevel(level.String())
		if !ok || parsed != level {
			t.Errorf("round trip for %d = (%d, %t)", int(level), int(parsed), ok)
		}
	}
}
