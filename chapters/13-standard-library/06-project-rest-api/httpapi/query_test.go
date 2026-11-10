package httpapi

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

func queryHandler(t *testing.T) http.Handler {
	t.Helper()
	store := contacts.NewStore()
	for _, name := range []string{"Ana López", "Bob", "ana Ruiz", "Charlie", "ANA", "Ana & +?/界"} {
		if _, err := store.Create(t.Context(), contacts.Input{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	return NewHandler(store, nil)
}

func contactIDs(collection Collection) []int64 {
	ids := make([]int64, 0, len(collection.Contacts))
	for _, contact := range collection.Contacts {
		ids = append(ids, contact.ID)
	}
	return ids
}

func TestCollectionNavigationPreservesFilterAndSort(t *testing.T) {
	handler := queryHandler(t)
	first := decodeResponse[Collection](t, request(handler, "GET", "/contacts?limit=2&sort=name&name_contains=%20ANA%20", ""), 200)
	if !reflect.DeepEqual(contactIDs(first), []int64{5, 6}) || first.Pagination != (Pagination{Limit: 2, Total: 4}) {
		t.Fatalf("first filtered page = %+v", first)
	}
	if first.Links.Prev != "" || first.Links.First != first.Links.Self || first.Links.Next == "" {
		t.Fatalf("first page links = %+v", first.Links)
	}
	for _, link := range []string{first.Links.Self, first.Links.First, first.Links.Next} {
		u, err := url.Parse(link)
		if err != nil || u.IsAbs() || u.Host != "" || u.Path != "/contacts" {
			t.Fatalf("invalid relative page link %q: %v", link, err)
		}
		if u.Query().Get("limit") != "2" || u.Query().Get("sort") != "name" || u.Query().Get("name_contains") != "ANA" {
			t.Fatalf("link lost normalized query values: %s", link)
		}
	}
	// Follow links returned by the server rather than reconstructing offsets.
	last := decodeResponse[Collection](t, request(handler, "GET", first.Links.Next, ""), 200)
	if !reflect.DeepEqual(contactIDs(last), []int64{1, 3}) || last.Pagination != (Pagination{Limit: 2, Offset: 2, Total: 4}) {
		t.Fatalf("last filtered page = %+v", last)
	}
	if last.Links.Next != "" || last.Links.Prev != first.Links.Self || last.Links.First != first.Links.First {
		t.Fatalf("last page links = %+v", last.Links)
	}
	again := decodeResponse[Collection](t, request(handler, "GET", last.Links.Prev, ""), 200)
	if !reflect.DeepEqual(again, first) {
		t.Fatalf("previous page differs: %+v", again)
	}
}

func TestCollectionFilterURLRoundTrip(t *testing.T) {
	handler := queryHandler(t)
	filter := " & +?/界"
	path := "/contacts?" + url.Values{"name_contains": {filter}}.Encode()
	page := decodeResponse[Collection](t, request(handler, "GET", path, ""), 200)
	if !reflect.DeepEqual(contactIDs(page), []int64{6}) || page.Pagination.Total != 1 {
		t.Fatalf("escaped filter = %+v", page)
	}
	u, err := url.Parse(page.Links.Self)
	if err != nil || u.Query().Get("name_contains") != "& +?/界" {
		t.Fatalf("self link changed filter meaning: %q, %v", page.Links.Self, err)
	}
	again := decodeResponse[Collection](t, request(handler, "GET", page.Links.Self, ""), 200)
	if !reflect.DeepEqual(page, again) {
		t.Fatalf("following escaped link changed page: %+v", again)
	}
}

func TestCollectionDefaultsAndEmptyPages(t *testing.T) {
	store := contacts.NewStore()
	for i := range 25 {
		if _, err := store.Create(t.Context(), contacts.Input{Name: fmt.Sprintf("Contact %02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewHandler(store, nil)
	page := decodeResponse[Collection](t, request(handler, "GET", "/contacts", ""), 200)
	if len(page.Contacts) != 20 || page.Pagination != (Pagination{Limit: 20, Total: 25}) || page.Links.Next == "" {
		t.Fatalf("default response must be bounded: %+v", page)
	}
	last := decodeResponse[Collection](t, request(handler, "GET", page.Links.Next, ""), 200)
	if len(last.Contacts) != 5 || last.Contacts[0].ID != 21 || last.Links.Next != "" {
		t.Fatalf("last short page = %+v", last)
	}
	for _, path := range []string{"/contacts?offset=25", "/contacts?offset=" + strconv.Itoa(math.MaxInt), "/contacts?name_contains=Missing"} {
		empty := decodeResponse[Collection](t, request(handler, "GET", path, ""), 200)
		if empty.Contacts == nil || len(empty.Contacts) != 0 || empty.Links.Next != "" || empty.Links.First == "" {
			t.Fatalf("empty page = %+v", empty)
		}
	}
}

func TestCollectionRejectsInvalidQueries(t *testing.T) {
	handler := queryHandler(t)
	for _, query := range []string{
		"limit=0", "limit=-1", "limit=101", "limit=", "limit=1.5", "limit=01", "limit=%2B1",
		"limit=999999999999999999999999", "offset=-1", "offset=01", "offset=", "offset=x",
		"offset=999999999999999999999999", "sort=", "sort=email", "sort=ID", "sort=name,-id",
		"unknown=1", "limit=1&limit=2", "name_contains=Ana&name_contains=Bob", "sort=id&sort=id",
		"limit=1;offset=2", "name_contains=%ZZ", "name_contains=%ff",
		"name_contains=" + url.QueryEscape(strings.Repeat("界", 81)),
	} {
		t.Run(query, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/contacts", nil)
			req.URL.RawQuery = query
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			expectProblem(t, response, 400)
		})
	}
	// Blank filters are valid and mean no filtering.
	all := decodeResponse[Collection](t, request(handler, "GET", "/contacts?name_contains=", ""), 200)
	if all.Pagination.Total != 6 {
		t.Fatalf("blank filter = %+v", all)
	}
}

type searchStore struct {
	ContactStore
	search func(context.Context, contacts.Query) (contacts.Page, error)
}

func (s searchStore) Search(ctx context.Context, query contacts.Query) (contacts.Page, error) {
	return s.search(ctx, query)
}

func TestCollectionPassesContextAndQueryToStore(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := searchStore{search: func(got context.Context, query contacts.Query) (contacts.Page, error) {
		if got != ctx || query != (contacts.Query{Limit: 3, Offset: 2, NameContains: "Ana", Sort: contacts.SortNameDesc}) {
			t.Errorf("unexpected store search: context %v, query %+v", got, query)
		}
		return contacts.Page{}, got.Err()
	}}
	req := httptest.NewRequest("GET", "/contacts?limit=3&offset=2&name_contains=Ana&sort=-name", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	NewHandler(store, nil).ServeHTTP(response, req)
	expectProblem(t, response, 503)
}
