package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

const audioProtocol = "sim7600.audio.v1"
const audioTokenProtocolPrefix = "sim7600.token."

func authMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				writeAuthError(w, "missing bearer token")
				return
			}
			given := strings.TrimPrefix(h, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
				writeAuthError(w, "invalid token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Browser WebSocket clients cannot set an Authorization header. Authenticate
// call audio with a secondary Sec-WebSocket-Protocol value, keeping the bearer
// token out of the URL, browser history, and ordinary access logs.
func audioAuthMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var offeredAudio bool
			var given string
			for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
				for _, part := range strings.Split(header, ",") {
					protocol := strings.TrimSpace(part)
					switch {
					case protocol == audioProtocol:
						offeredAudio = true
					case strings.HasPrefix(protocol, audioTokenProtocolPrefix):
						given = strings.TrimPrefix(protocol, audioTokenProtocolPrefix)
					}
				}
			}
			if !offeredAudio || subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
				writeAuthError(w, "invalid call audio credentials")
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
}

// writeAuthError emits the same envelope shape as huma errors (Task 7). Lives
// in auth.go because it's the only chi-middleware path that bypasses huma.
func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "unauthorized",
			"message": message,
		},
	})
}
