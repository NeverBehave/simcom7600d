//go:build headless

// Package uistatic supplies the optional web UI handler.
package uistatic

import "net/http"

// Handler reports that the web interface is unavailable in a server-only
// build. API and WebSocket routes are registered before this fallback handler.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "web UI not included in this server build", http.StatusNotFound)
	})
}
