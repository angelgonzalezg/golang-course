package httpapi

import (
	_ "embed"
	"net/http"
)

// Keep the checked-in contract and the served document identical. Embedding
// also makes the endpoint work when the binary runs outside this directory.
//
//go:embed openapi.json
var openAPIDocument []byte

func (a *api) openAPI(w http.ResponseWriter, r *http.Request) {
	a.writeReadBytes(w, r, openAPIDocument, "")
}
