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

func (f *lessonFaults) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		switch info.FullMethod {
		case contactsv1.ContactService_GetContact_FullMethodName:
			f.readCalls.Add(1)
			if f.readFailures.Add(-1) >= 0 {
				return nil, status.Error(codes.Unavailable, "simulated temporary read failure")
			}
		case contactsv1.ContactService_CreateContact_FullMethodName:
			f.createCalls.Add(1)
			response, err := next(ctx, request)
			if err == nil && f.dropCreate.CompareAndSwap(true, false) {
				return nil, status.Error(codes.Unavailable, "simulated lost response after successful create")
			}
			return response, err
		}
		return next(ctx, request)
	}
}
