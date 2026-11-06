package main

import (
	"errors"
	"testing"
	"time"
)

func TestParsePositiveDuration(t *testing.T) {
	tests := []struct {
		text string
		want time.Duration
	}{
		{"1ns", time.Nanosecond},
		{"250ms", 250 * time.Millisecond},
		{"1.5s", 1500 * time.Millisecond},
		{"1h30m", 90 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := parsePositiveDuration(tt.text)
			if err != nil || got != tt.want {
				t.Fatalf("parsePositiveDuration = (%v, %v), want %v", got, err, tt.want)
			}
		})
	}
}

func TestParsePositiveDurationRejectsUnusableConfiguration(t *testing.T) {
	for _, text := range []string{"0", "0s", "-1s", "0.1ns"} {
		got, err := parsePositiveDuration(text)
		if got != 0 || !errors.Is(err, ErrNonPositiveDuration) {
			t.Fatalf("%q: got (%v, %v), want zero and ErrNonPositiveDuration", text, got, err)
		}
	}
	// Missing units, unsupported calendar units, and overflow are parse failures.
	for _, text := range []string{"", "5", "1d", "bad", "9223372036854775808ns"} {
		got, err := parsePositiveDuration(text)
		if got != 0 || err == nil || errors.Is(err, ErrNonPositiveDuration) {
			t.Fatalf("%q: got (%v, %v), want a parsing failure", text, got, err)
		}
	}
}

func TestParseEventTimeNormalizesTheInstantToUTC(t *testing.T) {
	want := time.Date(2024, time.July, 1, 15, 30, 0, 123456789, time.UTC)
	for _, text := range []string{
		"2024-07-01T15:30:00.123456789Z",
		"2024-07-01T09:30:00.123456789-06:00",
		"2024-07-01T17:30:00.123456789+02:00",
	} {
		got, err := parseEventTime(text)
		if err != nil || !got.Equal(want) || got.Location() != time.UTC {
			t.Fatalf("%q: got (%v, %v), want UTC instant %v", text, got, err, want)
		}
	}
}

func TestParseEventTimeKeepsParseErrors(t *testing.T) {
	for _, text := range []string{
		"",
		"2024-07-01",
		"2024-07-01T09:30:00",
		"2024-02-30T09:30:00Z",
	} {
		got, err := parseEventTime(text)
		var parseErr *time.ParseError
		if !got.IsZero() || !errors.As(err, &parseErr) || parseErr.Value != text {
			t.Fatalf("%q: got (%v, %v), want zero Time and underlying ParseError", text, got, err)
		}
	}
}

func TestExplicitOffsetsDisambiguateRepeatedLocalClockTimes(t *testing.T) {
	// During the historical fall transition, the same displayed hour occurred
	// twice. Explicit offsets select distinct instants without guessing a region.
	first, err := parseEventTime("2024-11-03T01:30:00-04:00")
	if err != nil {
		t.Fatal(err)
	}
	second, err := parseEventTime("2024-11-03T01:30:00-05:00")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Before(second) || second.Sub(first) != time.Hour {
		t.Fatalf("offsets must select instants one hour apart: %v and %v", first, second)
	}
}

func TestRemainingLifetimeAtExpiryBoundary(t *testing.T) {
	expires := time.Date(2024, time.July, 1, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{"before", expires.Add(-10 * time.Minute), 10 * time.Minute},
		{"one nanosecond before", expires.Add(-time.Nanosecond), time.Nanosecond},
		{"exactly at expiry", expires, 0},
		{"same instant different location", expires.In(time.FixedZone("display", -6*60*60)), 0},
		{"one nanosecond after", expires.Add(time.Nanosecond), 0},
		{"long expired", expires.Add(time.Hour), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remainingLifetime(expires, tt.now); got != tt.want {
				t.Fatalf("remainingLifetime = %v, want %v", got, tt.want)
			}
		})
	}
}
