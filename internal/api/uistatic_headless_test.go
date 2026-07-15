//go:build headless

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newHeadlessTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := NewServer(Config{AuthToken: "test-token"})
	return httptest.NewServer(srv.Handler)
}

func TestHeadlessRootReturnsNotFound(t *testing.T) {
	ts := newHeadlessTestServer(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNotFound)
	}
}

func TestHeadlessAPIRoutesRemainAvailable(t *testing.T) {
	ts := newHeadlessTestServer(t)
	defer ts.Close()

	statusResponse, err := http.Get(ts.URL + "/v1/status")
	if err != nil {
		t.Fatalf("GET /v1/status: %v", err)
	}
	defer statusResponse.Body.Close()
	if statusResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status API = %d, want %d", statusResponse.StatusCode, http.StatusUnauthorized)
	}

	openAPIResponse, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer openAPIResponse.Body.Close()
	if openAPIResponse.StatusCode != http.StatusOK {
		t.Fatalf("OpenAPI status = %d, want %d", openAPIResponse.StatusCode, http.StatusOK)
	}
}
