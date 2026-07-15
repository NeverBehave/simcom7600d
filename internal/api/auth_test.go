package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAudioAuthMiddleware(t *testing.T) {
	var called bool
	handler := audioAuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	bad := httptest.NewRequest(http.MethodGet, "/v1/calls/id/audio", nil)
	bad.Header.Set("Sec-WebSocket-Protocol", audioProtocol+", "+audioTokenProtocolPrefix+"wrong")
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, bad)
	require.Equal(t, http.StatusUnauthorized, badRec.Code)
	require.False(t, called)

	good := httptest.NewRequest(http.MethodGet, "/v1/calls/id/audio", nil)
	good.Header.Set("Sec-WebSocket-Protocol", audioProtocol+", "+audioTokenProtocolPrefix+"secret")
	goodRec := httptest.NewRecorder()
	handler.ServeHTTP(goodRec, good)
	require.Equal(t, http.StatusNoContent, goodRec.Code)
	require.True(t, called)
	require.Equal(t, "no-store", goodRec.Header().Get("Cache-Control"))
}
