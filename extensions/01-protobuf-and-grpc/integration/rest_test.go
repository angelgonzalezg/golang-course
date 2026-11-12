//go:build integration

package integration_test

import (
	"database/sql"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"example.com/course/sql-lab/contactpg"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/httpapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// Share both current rows and the creation ledger across transports. When an
// application deliberately maps the same trusted subject, the same known input
// fields/key can replay through either adapter without a second creation.
func testRESTInterop(t *testing.T, db *sql.DB, _ string) {
	store := contactpg.NewStore(db)
	rpc := newEndpoint(t, store, false, nil)
	rest := httptest.NewServer(httpapi.NewHandlerWithIdempotency(store, func(*http.Request) string { return "writer-client" }, nil))
	defer rest.Close()
	original, _, err := invokeCreate(t.Context(), rpc.api, "shared-protocol", &contactsv1.ContactInput{Name: " Ana ", Phone: "123"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, media string, headers http.Header) (http.Header, []byte, int) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), method, rest.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if media != "" {
			request.Header.Set("Content-Type", media)
		}
		for key, values := range headers {
			request.Header[key] = values
		}
		response, err := rest.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.Header.Clone(), data, response.StatusCode
	}
	headers, body, code := call("POST", "/contacts", `{"name":" Ana ","phone":"123"}`, "application/json", http.Header{"Idempotency-Key": {"shared-protocol"}})
	var contact httpapi.ContactResource
	if err := json.Unmarshal(body, &contact); err != nil || code != 201 || contact.ID != original.Id || headers.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("HTTP did not replay RPC result: %d,%s,%v", code, body, err)
	}
	path := "/contacts/" + strconv.FormatInt(original.Id, 10)
	if _, _, code := call("PATCH", path, `{"email":"from-rest"}`, httpapi.MergePatchMediaType, http.Header{"If-Match": {headers.Get("ETag")}}); code != 200 {
		t.Fatalf("HTTP PATCH=%d", code)
	}
	current, err := rpc.api.GetContact(t.Context(), &contactsv1.GetContactRequest{Id: original.Id})
	if err != nil || current.Email != "from-rest" || current.Version == original.Version {
		t.Fatalf("RPC missed HTTP change: %v,%v", current, err)
	}
	_, err = rpc.api.UpdateContact(t.Context(), &contactsv1.UpdateContactRequest{
		Id: original.Id, Input: &contactsv1.ContactInput{Phone: "stale"},
		UpdateMask:      &fieldmaskpb.FieldMask{Paths: []string{"phone"}},
		ExpectedVersion: proto.String(original.Version),
	})
	if status.Code(err) != codes.Aborted {
		t.Fatalf("cross-transport stale write=%v", err)
	}
	if _, _, code := call("DELETE", path, "", "", nil); code != 204 {
		t.Fatalf("HTTP delete=%d", code)
	}
	replayed, md, err := invokeCreate(t.Context(), rpc.api, "shared-protocol", &contactsv1.ContactInput{Name: " Ana ", Phone: "123"})
	assertReplay(t, replayed, original, md, err)
	assertCounts(t, db, 0, 1)
}
