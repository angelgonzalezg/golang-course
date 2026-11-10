package httpapi

import (
	"bytes"
	json "encoding/json/v2"
	"net/http"
	"testing"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

func TestEmbeddedOpenAPIIsServedOutsideSourceDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	handler := NewHandler(contacts.NewStore(), nil)
	response := request(handler, "GET", "/openapi.json", "")
	if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), openAPIDocument) || response.Header().Get("ETag") == "" {
		t.Fatalf("embedded contract response: status %d, headers %v", response.Code, response.Header())
	}
	var document struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.OpenAPI != "3.1.1" {
		t.Fatalf("unexpected contract version: %q", document.OpenAPI)
	}
	for path, methods := range map[string][]string{
		"/": {"get", "head"}, "/openapi.json": {"get", "head"},
		"/contacts":      {"get", "head", "post"},
		"/contacts/{id}": {"get", "head", "put", "patch", "delete", "options"},
	} {
		for _, method := range methods {
			if _, ok := document.Paths[path][method]; !ok {
				t.Errorf("public operation missing from contract: %s %s", method, path)
			}
		}
	}
	notModified := conditionalRequest(handler, "GET", "/openapi.json", "", http.Header{"If-None-Match": {response.Header().Get("ETag")}})
	if notModified.Code != 304 || notModified.Body.Len() != 0 {
		t.Fatalf("contract revalidation = %d, %s", notModified.Code, notModified.Body.String())
	}
	expectProblem(t, request(handler, "POST", "/openapi.json", ""), 405)
}
