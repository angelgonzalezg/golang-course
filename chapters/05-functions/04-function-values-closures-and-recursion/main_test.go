package main

import "testing"

func TestMakeCountersIndependent(t *testing.T) {
	first := makeCounter()
	second := makeCounter()

	steps := []struct {
		next func() int
		want int
	}{
		{next: first, want: 1},
		{next: first, want: 2},
		{next: second, want: 1},
		{next: first, want: 3},
		{next: second, want: 2},
	}

	for index, step := range steps {
		if got := step.next(); got != step.want {
			t.Errorf("step %d: counter returned %d, want %d", index+1, got, step.want)
		}
	}
}
