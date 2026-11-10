// Package rpcapi translates unary and streaming RPCs into shared contact operations.
package rpcapi

import (
	"context"
	"io"
	"log/slog"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Store is the behavior this adapter needs. Both REST and gRPC can receive the
// same *contacts.Store; neither transport defines the contact business rules.
type Store interface {
	Create(context.Context, contacts.Input) (contacts.Contact, error)
	Get(context.Context, int64) (contacts.Contact, error)
	Search(context.Context, contacts.Query) (contacts.Page, error)
	ReplaceIf(context.Context, int64, contacts.Input, contacts.Conditions) (contacts.Contact, error)
	PatchIf(context.Context, int64, contacts.Changes, contacts.Conditions) (contacts.Contact, error)
	DeleteIf(context.Context, int64, contacts.Conditions) error
}

type Service struct {
	// Embed by value, as required by the generated registration code. Future
	// methods retain a forward-compatible default until explicitly implemented.
	contactsv1.UnimplementedContactServiceServer
	store  Store
	logger *slog.Logger
}

func New(store Store, logger *slog.Logger) *Service {
	if store == nil {
		panic("rpcapi: a usable store is required")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{store: store, logger: logger}
}

var _ contactsv1.ContactServiceServer = (*Service)(nil)

func (s *Service) CreateContact(ctx context.Context, request *contactsv1.CreateContactRequest) (*contactsv1.Contact, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.rpcError(ctx, err)
	}
	if request == nil || request.Input == nil {
		return nil, invalid("input", "is required")
	}
	value, err := s.store.Create(ctx, inputValue(request.Input))
	if err != nil {
		return nil, s.rpcError(ctx, err)
	}
	return resource(value), nil
}

func (s *Service) GetContact(ctx context.Context, request *contactsv1.GetContactRequest) (*contactsv1.Contact, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.rpcError(ctx, err)
	}
	if request == nil {
		return nil, invalid("request", "is required")
	}
	value, err := s.store.Get(ctx, request.GetId())
	if err != nil {
		return nil, s.rpcError(ctx, err)
	}
	return resource(value), nil
}

func (s *Service) ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.rpcError(ctx, err)
	}
	if request == nil {
		return nil, invalid("request", "is required")
	}
	query, err := listQuery(request)
	if err != nil {
		return nil, err
	}
	page, err := s.store.Search(ctx, query)
	if err != nil {
		return nil, s.rpcError(ctx, err)
	}
	response := &contactsv1.ListContactsResponse{
		Contacts: make([]*contactsv1.Contact, 0, len(page.Contacts)), Total: int64(page.Total),
		Limit: int32(page.Query.Limit), Offset: int64(page.Query.Offset),
	}
	for _, value := range page.Contacts {
		response.Contacts = append(response.Contacts, resource(value))
	}
	if page.Query.Offset < page.Total && page.Query.Limit < page.Total-page.Query.Offset {
		next := int64(page.Query.Offset + page.Query.Limit)
		response.NextOffset = &next
	}
	return response, nil
}

func (s *Service) ReplaceContact(ctx context.Context, request *contactsv1.ReplaceContactRequest) (*contactsv1.Contact, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.rpcError(ctx, err)
	}
	if request == nil || request.Input == nil {
		return nil, invalid("input", "is required")
	}
	conditions, err := versionCondition(request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	// All editable fields are supplied, including zero values that clear data.
	value, err := s.store.ReplaceIf(ctx, request.GetId(), inputValue(request.Input), conditions)
	if err != nil {
		return nil, s.rpcError(ctx, err)
	}
	return resource(value), nil
}

func (s *Service) UpdateContact(ctx context.Context, request *contactsv1.UpdateContactRequest) (*contactsv1.Contact, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.rpcError(ctx, err)
	}
	if request == nil || request.Input == nil {
		return nil, invalid("input", "is required")
	}
	changes, err := selectedChanges(request)
	if err != nil {
		return nil, err
	}
	conditions, err := versionCondition(request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	// Comparison and mutation happen inside PatchIf's single store lock.
	value, err := s.store.PatchIf(ctx, request.GetId(), changes, conditions)
	if err != nil {
		return nil, s.rpcError(ctx, err)
	}
	return resource(value), nil
}

func (s *Service) DeleteContact(ctx context.Context, request *contactsv1.DeleteContactRequest) (*contactsv1.DeleteContactResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.rpcError(ctx, err)
	}
	if request == nil {
		return nil, invalid("request", "is required")
	}
	conditions, err := versionCondition(request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if err := s.store.DeleteIf(ctx, request.GetId(), conditions); err != nil {
		return nil, s.rpcError(ctx, err)
	}
	return &contactsv1.DeleteContactResponse{}, nil
}

func inputValue(input *contactsv1.ContactInput) contacts.Input {
	return contacts.Input{Name: input.GetName(), Email: input.GetEmail(), Phone: input.GetPhone()}
}

func resource(value contacts.Contact) *contactsv1.Contact {
	return &contactsv1.Contact{Id: value.ID, Name: value.Name, Email: value.Email, Phone: value.Phone, Version: contacts.Version(value)}
}
