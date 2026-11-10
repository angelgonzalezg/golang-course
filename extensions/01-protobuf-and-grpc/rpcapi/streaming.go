package rpcapi

import (
	"io"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MaxImportContacts bounds the work and response size of one import RPC.
// It does not make the batch transactional or impose a transport message limit.
const MaxImportContacts = 100

func (s *Service) StreamContacts(request *contactsv1.ListContactsRequest, stream grpc.ServerStreamingServer[contactsv1.Contact]) error {
	ctx := stream.Context()
	// Search returns one bounded snapshot and releases its lock before any Send.
	// Reuse unary validation and paging rather than inventing different rules.
	page, err := s.ListContacts(ctx, request)
	if err != nil {
		return err
	}
	for _, contact := range page.Contacts {
		if err := ctx.Err(); err != nil {
			return s.rpcError(ctx, err)
		}
		// Send can block under flow control. Wait here; no goroutine per message
		// or growing application queue. A successful Send is not a client ack.
		if err := stream.Send(contact); err != nil {
			return err
		}
	}
	return nil // The client must still read to io.EOF to observe successful completion.
}

func (s *Service) ImportContacts(stream grpc.ClientStreamingServer[contactsv1.CreateContactRequest, contactsv1.ImportContactsResponse]) error {
	ctx := stream.Context()
	response := &contactsv1.ImportContactsResponse{}
	for {
		if err := ctx.Err(); err != nil {
			return s.rpcError(ctx, err)
		}
		request, err := stream.Recv()
		if err == io.EOF {
			// EOF here means the client finished sending, not that the whole RPC
			// is done. The client still needs this response and the final status.
			return stream.SendAndClose(response)
		}
		if err != nil {
			return err
		}
		if response.CreatedCount == MaxImportContacts {
			return status.Errorf(codes.ResourceExhausted, "import accepts at most %d contacts", MaxImportContacts)
		}
		contact, err := s.CreateContact(ctx, request)
		if err != nil {
			// Earlier creates remain committed. No transaction or automatic retry
			// is implied by streaming, and a failed RPC has no final summary.
			return err
		}
		response.CreatedCount++
		response.Ids = append(response.Ids, contact.GetId())
	}
}

func (s *Service) LookupContacts(stream grpc.BidiStreamingServer[contactsv1.LookupContactsRequest, contactsv1.LookupContactsResponse]) error {
	ctx := stream.Context()
	for {
		if err := ctx.Err(); err != nil {
			return s.rpcError(ctx, err)
		}
		request, err := stream.Recv()
		if err == io.EOF {
			return nil // All received requests have already had a reply.
		}
		if err != nil {
			return err
		}
		contact, err := s.GetContact(ctx, &contactsv1.GetContactRequest{Id: request.GetId()})
		response := &contactsv1.LookupContactsResponse{RequestId: request.GetRequestId()}
		switch status.Code(err) {
		case codes.OK:
			response.Result = &contactsv1.LookupContactsResponse_Contact{Contact: contact}
		case codes.InvalidArgument, codes.NotFound:
			code := contactsv1.LookupFailureCode_LOOKUP_FAILURE_CODE_INVALID_ARGUMENT
			if status.Code(err) == codes.NotFound {
				code = contactsv1.LookupFailureCode_LOOKUP_FAILURE_CODE_NOT_FOUND
			}
			response.Result = &contactsv1.LookupContactsResponse_Failure{Failure: &contactsv1.LookupFailure{
				Code: code, Message: status.Convert(err).Message(),
			}}
		default:
			return err // Infrastructure/context failures terminate the stream.
		}
		// This handler processes one item at a time. The client receives while
		// sending, so both directions can progress under flow control.
		if err := stream.Send(response); err != nil {
			return err
		}
	}
}
