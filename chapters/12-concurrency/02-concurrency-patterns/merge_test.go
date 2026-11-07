package main

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
)

func TestFanInMergesAndClosesAfterAllInputs(t *testing.T) {
	first := make(chan Result, 2)
	first <- Result{Index: 0, Output: 10}
	first <- Result{Index: 2, Output: 30}
	close(first)
	second := make(chan Result, 1)
	second <- Result{Index: 1, Output: 20}
	close(second)
	var stages sync.WaitGroup
	merged := fanIn(context.Background(), &stages, 0, first, nil, second)
	var values []int
	for result := range merged {
		values = append(values, result.Output)
	}
	stages.Wait()
	slices.Sort(values)
	if !slices.Equal(values, []int{10, 20, 30}) {
		t.Fatalf("merged values=%v", values)
	}
	if _, ok := <-merged; ok {
		t.Fatal("merged output remained open")
	}
}

func TestFanInWithoutActiveInputsCloses(t *testing.T) {
	var stages sync.WaitGroup
	merged := fanIn(context.Background(), &stages, 0, nil)
	stages.Wait()
	if _, ok := <-merged; ok {
		t.Fatal("absent inputs produced a result")
	}
}

func TestFanInCanCancelABlockedReceiveOrSend(t *testing.T) {
	for _, phase := range []string{"receive", "send"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				input := make(chan Result)
				var stages sync.WaitGroup
				merged := fanIn(ctx, &stages, 0, input)
				if phase == "send" {
					// This handshake proves the receive completed, while no receiver
					// exists for the forwarder's unbuffered output.
					input <- Result{Output: 1}
				}
				synctest.Wait()
				cancel()
				stages.Wait()
				if _, ok := <-merged; ok {
					t.Fatal("canceled blocked forwarder unexpectedly sent a result")
				}
				// The producer owns input; fan-in never closes a borrowed input.
				close(input)
			})
		})
	}
}
