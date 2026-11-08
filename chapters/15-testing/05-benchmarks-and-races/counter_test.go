package measurement

import (
	"sync"
	"testing"
)

func TestCounterConcurrent(t *testing.T) {
	const workers, increments = 8, 100
	var counter Counter
	if got := counter.Value(); got != 0 {
		t.Fatalf("zero-value counter = %d, want 0", got)
	}
	start := make(chan struct{})
	var finished sync.WaitGroup
	for range workers {
		finished.Add(1)
		go func() {
			defer finished.Done()
			<-start
			for range increments {
				counter.Increment()
				_ = counter.Value() // Exercise concurrent reads as well as writes.
			}
		}()
	}
	close(start)
	finished.Wait() // Join all workers before inspecting the final result.
	if got, want := counter.Value(), workers*increments; got != want {
		t.Fatalf("counter = %d, want %d", got, want)
	}
}
