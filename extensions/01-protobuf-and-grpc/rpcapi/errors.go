package rpcapi

import (
	"context"
	"errors"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func invalid(field, message string) error {
	base := status.New(codes.InvalidArgument, field+": "+message)
	detailed, err := base.WithDetails(&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: field, Description: message}}})
	if err != nil {
		return base.Err()
	}
	return detailed.Err()
}

func (s *Service) rpcError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, contacts.ErrInvalidInput):
		var validation *contacts.ValidationError
		if errors.As(err, &validation) {
			field := validation.Field
			if field == "name" || field == "email" || field == "phone" {
				field = "input." + field
			}
			return invalid(field, validation.Message)
		}
		return status.Error(codes.InvalidArgument, "invalid contact input")
	case errors.Is(err, contacts.ErrNotFound):
		return status.Error(codes.NotFound, "contact not found")
	case errors.Is(err, contacts.ErrNameTaken):
		return status.Error(codes.AlreadyExists, "contact name already exists")
	case errors.Is(err, contacts.ErrPreconditionFailed):
		return status.Error(codes.Aborted, "contact changed; read current state before deciding to retry")
	case errors.Is(err, contacts.ErrIDExhausted):
		return status.Error(codes.ResourceExhausted, "no contact IDs are available")
	default:
		s.logger.ErrorContext(ctx, "contact operation failed", "error", err)
		return status.Error(codes.Internal, "contact operation failed")
	}
}
