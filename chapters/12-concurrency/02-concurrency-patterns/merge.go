package main

import (
	"context"
	"sync"
)

// fanIn merges distinct streams in unspecified arrival order. Nil inputs are
// ignored. The caller registers all stages before waiting and either consumes
// the output or cancels the shared context before joining those stages.
func fanIn(ctx context.Context, stages *sync.WaitGroup, capacity int, inputs ...<-chan Result) <-chan Result {
	out := make(chan Result, capacity)
	var forwarders sync.WaitGroup
	for _, input := range inputs {
		if input == nil {
			continue
		}
		stages.Add(1)
		forwarders.Add(1)
		go func(input <-chan Result) {
			defer stages.Done()
			defer forwarders.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				var result Result
				select {
				case <-ctx.Done():
					return
				case value, ok := <-input:
					if !ok {
						return
					}
					result = value
				}
				// A slow consumer fills this bounded queue, then blocks forwarding.
				// Cancellation must be observed here as well as on the receive.
				select {
				case <-ctx.Done():
					return
				case out <- result:
				}
			}
		}(input)
	}
	stages.Add(1)
	go func() {
		defer stages.Done()
		forwarders.Wait()
		close(out) // Only this coordinator knows every shared-output sender returned.
	}()
	return out
}
