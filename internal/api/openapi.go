package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var embeddedSpec []byte

// handleOpenAPISpec serves the committed OpenAPI 3.1 spec embedded at build
// time. Unauthenticated by design — the spec describes shape, not data, and
// SDK tooling needs to fetch it without credentials.
func handleOpenAPISpec() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(embeddedSpec)
	}
}
