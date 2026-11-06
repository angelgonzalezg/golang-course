package main

import (
	"fmt"
	"testing"
)

func TestGeneratedStatusNames(t *testing.T) {
	for _, tt := range []struct {
		status Status
		want   string
	}{{StatusQueued, "Queued"}, {StatusRunning, "Running"}, {StatusComplete, "Complete"}, {Status(-1), "Status(-1)"}} {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("status %d = %q, want %q", int(tt.status), got, tt.want)
		}
	}
	unknown := Status(len(statusNames))
	if got, want := unknown.String(), fmt.Sprintf("Status(%d)", int(unknown)); got != want {
		t.Errorf("out-of-range status = %q, want %q", got, want)
	}
}
