package main

import (
	"sync"
	"sync/atomic"
)

// runWorkers starts count finite callbacks and waits for them to return.
// Counts in this lesson are nonnegative, and callbacks must eventually return.
func runWorkers(count int, work func(id int)) {
	var workers sync.WaitGroup
	for id := 0; id < count; id++ {
		// Register before starting the goroutine, so Wait cannot miss this task.
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			work(id)
		}(id)
	}
	workers.Wait()
}

// Counter keeps the protected value and its lock together. Its zero value works.
// Use one *Counter across callers; do not copy a Counter after the mutex is used.
type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Increment() {
	c.mu.Lock()
	c.value++
	c.mu.Unlock()
}

func (c *Counter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func incrementWithMutex(counter *Counter, times int) {
	for i := 0; i < times; i++ {
		counter.Increment()
	}
}

// A typed atomic fits this independent counter. Related multi-field invariants
// need a broader synchronization protocol, usually a mutex. Keep all accesses atomic.
func incrementAtomic(counter *atomic.Int64, times int) {
	for i := 0; i < times; i++ {
		counter.Add(1)
	}
}

// ConfigLoader publishes one initialized value to all Load callers. Its zero value works.
// Do not copy it after first use; all calls must coordinate through the same Once.
type ConfigLoader struct {
	once  sync.Once
	value string
}

// Load runs only the first supplied initializer. Other callers wait for it and
// receive the same value. Use another design for repeated updates or retrying loads.
func (l *ConfigLoader) Load(initialize func() string) string {
	l.once.Do(func() {
		l.value = initialize()
	})
	return l.value
}
