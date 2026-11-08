// Package measurement provides a shared counter for race and benchmark exercises.
package measurement

import "sync"

// Counter's zero value is ready to use. Share its pointer and do not copy it
// after first use: both reads and writes must use the same mutex.
type Counter struct {
	mu    sync.Mutex
	value int
}

func (c *Counter) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}

func (c *Counter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}
