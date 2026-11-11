package main

import (
	"context"
	"sync/atomic"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// lessonFaults is controlled by the local demo/test, never by incoming metadata.
// The post-create failure simulates losing the reply after the handler committed;
// it does not simulate dropping packets or every possible transport failure.
type lessonFaults struct {
	readFailures atomic.Int64
	dropCreate   atomic.Bool
	readCalls    atomic.Int64
	createCalls  atomic.Int64
}

// Unary injects a failure before read work or after a successful create result.
// Atomics keep the teaching counters/one-shot flag safe across concurrent calls.
func (f *lessonFaults) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		switch info.FullMethod {
		case contactsv1.ContactService_GetContact_FullMethodName:
			f.readCalls.Add(1)
			// Starting from 2, decrements return 1, 0, then -1: exactly the first
			// two reads fail before the actual handler is called.
			if f.readFailures.Add(-1) >= 0 {
				return nil, status.Error(codes.Unavailable, "simulated temporary read failure")
			}
		case contactsv1.ContactService_CreateContact_FullMethodName:
			f.createCalls.Add(1)
			response, err := next(ctx, request)
			// Exactly one successful create reply is hidden. The handler/cache
			// already finished, so the next same-key call can replay its result.
			if err == nil && f.dropCreate.CompareAndSwap(true, false) {
				return nil, status.Error(codes.Unavailable, "simulated lost response after successful create")
			}
			return response, err
		}
		return next(ctx, request)
	}
}
