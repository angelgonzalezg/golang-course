package main

import (
	"math"
	"strconv"
	"testing"
)

func TestBuildLabels(t *testing.T) {
	for _, build := range []struct {
		name string
		fn   func(string, int) string
	}{
		{name: "ordinary conversion", fn: buildLabel},
		{name: "private buffer view", fn: buildLabelUnsafe},
	} {
		t.Run(build.name, func(t *testing.T) {
			for _, tt := range []struct {
				prefix string
				number int
				want   string
			}{
				{prefix: "task", number: 42, want: "task:42"},
				{prefix: "task", number: -7, want: "task:-7"},
				{want: ":0"},
				{prefix: "niño", number: 1, want: "niño:1"},
				{prefix: "a\x00b", number: 2, want: "a\x00b:2"},
				{prefix: "min", number: math.MinInt, want: "min:" + strconv.Itoa(math.MinInt)},
				{prefix: "max", number: math.MaxInt, want: "max:" + strconv.Itoa(math.MaxInt)},
			} {
				if got := build.fn(tt.prefix, tt.number); got != tt.want {
					t.Errorf("build(%q, %d)=%q, want %q", tt.prefix, tt.number, got, tt.want)
				}
			}
		})
	}
}

func TestPrivateBufferResultsRemainIndependent(t *testing.T) {
	// Retain earlier labels while constructing later ones. Reusing mutable
	// storage for multiple returned strings would violate this contract.
	labels := make([]string, 100)
	for index := range labels {
		labels[index] = buildLabelUnsafe("task", index)
	}
	for index, label := range labels {
		if want := "task:" + strconv.Itoa(index); label != want {
			t.Fatalf("retained label %d=%q, want %q", index, label, want)
		}
	}
}
