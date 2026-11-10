package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

func conditionalRequest(handler http.Handler, method, path, body string, headers http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Content-Type", "application/json")
	if method == "PATCH" {
		req.Header.Set("Content-Type", MergePatchMediaType)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestConditionalReadComparisons(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	created := request(handler, "POST", "/contacts", `{"name":"Ana"}`)
	path := created.Header().Get("Location")
	first := request(handler, "GET", path, "")
	tag := first.Header().Get("ETag")
	if tag == "" || strings.HasPrefix(tag, "W/") || tag != created.Header().Get("ETag") || first.Body.String() != created.Body.String() || first.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("creation/read validators or representation differ: POST %v, GET %v", created.Header(), first.Header())
	}
	for _, tc := range []struct {
		name    string
		headers http.Header
		status  int
	}{
		{"fresh", nil, 200},
		{"matching read", http.Header{"If-None-Match": {tag}}, 304},
		{"weak read comparison", http.Header{"If-None-Match": {"W/" + tag}}, 304},
		{"any representation", http.Header{"If-None-Match": {"*"}}, 304},
		{"different representation", http.Header{"If-None-Match": {`"other"`}}, 200},
		{"commas inside tag", http.Header{"If-None-Match": {`"comma,inside", ` + tag}}, 304},
		{"multiple header lines", http.Header{"If-None-Match": {`"other"`, tag}}, 304},
		{"empty elements tolerated", http.Header{"If-None-Match": {", ," + tag + ","}}, 304},
		{"empty none-match list", http.Header{"If-None-Match": {""}}, 200},
		{"empty match list", http.Header{"If-Match": {""}}, 412},
		{"matching strong precondition", http.Header{"If-Match": {tag}}, 200},
		{"weak cannot satisfy If-Match", http.Header{"If-Match": {"W/" + tag}}, 412},
		{"If-Match evaluated first", http.Header{"If-Match": {`"stale"`}, "If-None-Match": {tag}}, 412},
		{"both satisfied", http.Header{"If-Match": {tag}, "If-None-Match": {`"other"`}}, 200},
		{"unquoted", http.Header{"If-None-Match": {"unquoted"}}, 400},
		{"mixed wildcard", http.Header{"If-Match": {"*, " + tag}}, 400},
		{"unterminated", http.Header{"If-None-Match": {`"missing quote`}}, 400},
		{"missing comma", http.Header{"If-None-Match": {`"a" "b"`}}, 400},
		{"invalid tag byte", http.Header{"If-Match": {"\"space inside\""}}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := conditionalRequest(handler, "GET", path, "", tc.headers)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d; body %s", response.Code, tc.status, response.Body.String())
			}
			if tc.status >= 400 {
				expectProblem(t, response, tc.status)
				if response.Header().Get("ETag") != "" {
					t.Fatal("error response leaked a success validator")
				}
			} else if response.Header().Get("ETag") != tag {
				t.Fatal("read lost current ETag")
			}
			if tc.status == 304 && (response.Body.Len() != 0 || response.Header().Get("Content-Type") != "" || response.Header().Get("Cache-Control") != "private, no-cache") {
				t.Fatalf("304 must carry validators/cache policy and no body: %v, %s", response.Header(), response.Body.String())
			}
		})
	}
	expectProblem(t, conditionalRequest(handler, "GET", "/contacts/999", "", http.Header{"If-None-Match": {"*"}}), 404)
}

func TestConditionalWritesAndPUTValidatorRule(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	created := request(handler, "POST", "/contacts", `{"name":"Ana","email":"ana@example.com"}`)
	path, oldTag := created.Header().Get("Location"), created.Header().Get("ETag")
	patched := conditionalRequest(handler, "PATCH", path, `{"name":"Ana Torres"}`, http.Header{"If-Match": {oldTag}})
	decodeResponse[ContactResource](t, patched, 200)
	newTag := patched.Header().Get("ETag")
	if newTag == "" || newTag == oldTag {
		t.Fatal("PATCH failed to return its new validator")
	}
	for _, method := range []string{"PUT", "PATCH", "DELETE"} {
		expectProblem(t, conditionalRequest(handler, method, path, `{"name":"Stale"}`, http.Header{"If-Match": {oldTag}}), 412)
	}
	current := decodeResponse[ContactResource](t, request(handler, "GET", path, ""), 200)
	if current.Name != "Ana Torres" || current.Email != "ana@example.com" {
		t.Fatalf("rejected write changed contact: %+v", current)
	}
	expectProblem(t, conditionalRequest(handler, "PATCH", path, `{}`, http.Header{"If-Match": {"W/" + newTag}}), 412)
	expectProblem(t, conditionalRequest(handler, "PATCH", path, `{}`, http.Header{"If-None-Match": {"W/" + newTag}}), 412)
	expectProblem(t, conditionalRequest(handler, "DELETE", path, "", http.Header{"If-None-Match": {"*"}}), 412)
	put := conditionalRequest(handler, "PUT", path, `{"name":" Ana Updated "}`, http.Header{"If-Match": {`"unknown,tag", ` + newTag}})
	decodeResponse[ContactResource](t, put, 200)
	if put.Header().Get("ETag") != "" {
		t.Fatal("transformed PUT representation must not send a success validator")
	}
	fresh := request(handler, "GET", path, "")
	if fresh.Header().Get("ETag") == newTag || fresh.Header().Get("ETag") == "" {
		t.Fatal("GET must supply the new validator after PUT")
	}
	deleted := conditionalRequest(handler, "DELETE", path, "", http.Header{"If-Match": {"*"}})
	if deleted.Code != 204 || deleted.Body.Len() != 0 {
		t.Fatalf("conditional DELETE = %d, %s", deleted.Code, deleted.Body.String())
	}
	expectProblem(t, conditionalRequest(handler, "DELETE", path, "", http.Header{"If-Match": {"*"}}), 404)
	for _, target := range []struct{ method, path, body string }{{"POST", "/contacts", `{"name":"Bob"}`}, {"OPTIONS", "/contacts/1", ""}} {
		expectProblem(t, conditionalRequest(handler, target.method, target.path, target.body, http.Header{"If-Match": {"*"}}), 400)
	}
}

func TestCollectionValidatorsTrackSelectedRepresentation(t *testing.T) {
	handler := NewHandler(contacts.NewStore(), nil)
	request(handler, "POST", "/contacts", `{"name":"Ana"}`)
	path := "/contacts?limit=1&name_contains=Ana"
	first := request(handler, "GET", path, "")
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("collection has no ETag")
	}
	unchanged := conditionalRequest(handler, "GET", path, "", http.Header{"If-None-Match": {tag}})
	if unchanged.Code != 304 {
		t.Fatalf("unchanged page = %d", unchanged.Code)
	}
	request(handler, "POST", "/contacts", `{"name":"Bob"}`)
	if got := conditionalRequest(handler, "GET", path, "", http.Header{"If-None-Match": {tag}}); got.Code != 304 {
		t.Fatalf("unselected contact changed page bytes: %d", got.Code)
	}
	request(handler, "POST", "/contacts", `{"name":"Ana Torres"}`)
	changed := conditionalRequest(handler, "GET", path, "", http.Header{"If-None-Match": {tag}})
	page := decodeResponse[Collection](t, changed, 200)
	if page.Pagination.Total != 2 || changed.Header().Get("ETag") == tag {
		t.Fatalf("changed total must change validator: %+v", page)
	}
}

func TestConditionalHEADAndCompetingHTTPWriters(t *testing.T) {
	server := httptest.NewServer(NewHandler(contacts.NewStore(), nil))
	defer server.Close()
	client := server.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Post(server.URL+"/contacts", "application/json", strings.NewReader(`{"name":"Ana"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, resp.Body)
	if err := errors.Join(readErr, resp.Body.Close()); err != nil {
		t.Fatal(err)
	}
	path, tag := resp.Header.Get("Location"), resp.Header.Get("ETag")
	for _, conditional := range []bool{false, true} {
		req, err := http.NewRequestWithContext(t.Context(), "HEAD", server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		wantStatus := 200
		if conditional {
			req.Header.Set("If-None-Match", tag)
			wantStatus = 304
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(resp.Body)
		if err := errors.Join(readErr, resp.Body.Close()); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != wantStatus || len(data) != 0 || resp.Header.Get("ETag") != tag {
			t.Fatalf("HEAD: status %d, body %q, headers %v", resp.StatusCode, data, resp.Header)
		}
	}
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, body := range []string{`{"name":"Writer A"}`, `{"name":"Writer B"}`} {
		wg.Go(func() {
			<-start
			req, err := http.NewRequestWithContext(t.Context(), "PATCH", server.URL+path, strings.NewReader(body))
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Content-Type", MergePatchMediaType)
			req.Header.Set("If-Match", tag)
			resp, err := client.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			_, readErr := io.Copy(io.Discard, resp.Body)
			if err := errors.Join(readErr, resp.Body.Close()); err != nil {
				t.Error(err)
			}
			statuses <- resp.StatusCode
		})
	}
	close(start)
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[412] != 1 {
		t.Fatalf("competing HTTP writers: %v", counts)
	}
}
