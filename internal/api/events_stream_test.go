package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/store"
)

func TestEventsStream_RequiresAuth(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	defer st.Close()
	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret", Store: st})

	resp, err := http.Get(srv.URL + eventStreamPath)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestEventsStream_SkipsHistoryAndStreamsNewEvents(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	defer st.Close()
	_, err = st.AppendEvent(context.Background(), store.Event{Kind: "sms.arrived", Detail: `{"body":"old"}`})
	require.NoError(t, err)
	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret", Store: st})

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+eventStreamPath, nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	_, err = st.AppendEvent(context.Background(), store.Event{
		Kind: "call.ringing", RefKind: "call", RefID: "call-1", Detail: `{"from":"+15551234567"}`,
	})
	require.NoError(t, err)

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.Contains(line, `"ref_id":"call-1"`) {
			break
		}
	}
	joined := strings.Join(lines, "\n")
	require.Contains(t, joined, "event: ready")
	require.Contains(t, joined, "event: sim7600")
	require.Contains(t, joined, `"kind":"call.ringing"`)
	require.NotContains(t, joined, `"body":"old"`)
}

func TestTimeoutMiddlewarePreservesCallAudioWebSocketUpgrade(t *testing.T) {
	handler := timeoutExceptLongLived(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, canHijack := w.(http.Hijacker)
		_, hasDeadline := req.Context().Deadline()
		if !canHijack || hasDeadline {
			http.Error(w, "websocket capabilities lost", http.StatusNotImplemented)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	srv := httptest.NewServer(handler)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/calls/call-1/audio")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestTimeoutMiddlewareStillLimitsOrdinaryAPIRequests(t *testing.T) {
	handler := timeoutExceptLongLived(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, hasDeadline := req.Context().Deadline()
		require.True(t, hasDeadline)
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestEventStreamLimiter(t *testing.T) {
	limiter := newEventStreamLimiter(1)
	require.True(t, limiter.acquire())
	require.False(t, limiter.acquire())
	limiter.release()
	require.True(t, limiter.acquire())
	limiter.release()
}
