//go:build headless

package uistatic

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeadlessHandlerReturnsNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if body := recorder.Body.String(); body != "web UI not included in this server build\n" {
		t.Fatalf("body = %q", body)
	}
}
