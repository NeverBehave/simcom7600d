package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/modem"
)

func TestLimitRequestBody(t *testing.T) {
	handler := limitRequestBody(4)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/sms", bytes.NewBufferString("12345"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestLimitConcurrentRequests(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	handler := limitConcurrentRequests(1)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		close(firstDone)
	}()
	<-started
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	close(release)
	<-firstDone
}

func newTestServer(t *testing.T, m modem.Modem) *httptest.Server {
	t.Helper()
	return newTestServerWith(t, m, Config{AuthToken: "secret"})
}

func newTestServerWith(t *testing.T, m modem.Modem, cfg Config) *httptest.Server {
	t.Helper()
	cfg.Modem = m
	srv := httptest.NewServer(NewRouter(cfg))
	t.Cleanup(srv.Close)
	return srv
}

func TestStatus_RequiresAuth(t *testing.T) {
	srv := newTestServer(t, &fakeModem{status: modem.ModemStatus{Model: "X", UpdatedAt: time.Now().UTC()}})
	resp, err := http.Get(srv.URL + "/v1/status")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestStatus_OK(t *testing.T) {
	srv := newTestServer(t, &fakeModem{
		status: modem.ModemStatus{Model: "SIM7600G-H", IMEI: "123", UpdatedAt: time.Now().UTC()},
	})
	req, _ := http.NewRequest("GET", srv.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "SIM7600G-H", body["modem"].(map[string]any)["model"])
	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	require.Contains(t, resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'")
	_ = context.Background
}
