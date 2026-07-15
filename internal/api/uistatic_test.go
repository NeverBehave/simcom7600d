//go:build !headless

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServerNoModem(t *testing.T) *httptest.Server {
	t.Helper()
	srv := NewServer(Config{
		AuthToken:          "test-token",
		Modem:              nil,
		Store:              nil,
		Admin:              nil,
		AllowATPassthrough: false,
		AllowModemReset:    false,
	})
	return httptest.NewServer(srv.Handler)
}

func TestRoot_ServesSPA(t *testing.T) {
	ts := newTestServerNoModem(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type: got %q want text/html", ct)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "<html") {
		t.Fatalf("body does not look like HTML: %q", string(body[:min(80, len(body))]))
	}
	if got := res.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options: got %q want DENY", got)
	}
	if got := res.Header.Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("CSP does not prevent framing: %q", got)
	}
}

func TestRoot_DeepLinkFallsBackToIndex(t *testing.T) {
	ts := newTestServerNoModem(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/sms/some-id")
	if err != nil {
		t.Fatalf("GET /sms/some-id: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "<html") {
		t.Fatalf("deep-link fallback did not return HTML")
	}
}

func TestV1_StillRequiresAuth(t *testing.T) {
	ts := newTestServerNoModem(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/v1/status")
	if err != nil {
		t.Fatalf("GET /v1/status: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", res.StatusCode)
	}
}

func TestOpenAPI_StillServed(t *testing.T) {
	ts := newTestServerNoModem(t)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", res.StatusCode)
	}
}
