package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"sim7600d/internal/store"
)

const eventStreamPath = "/v1/events/stream"

type eventStreamLimiter chan struct{}

func newEventStreamLimiter(max int) eventStreamLimiter {
	if max < 1 {
		max = 1
	}
	return make(eventStreamLimiter, max)
}

func (l eventStreamLimiter) acquire() bool {
	select {
	case l <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l eventStreamLimiter) release() { <-l }

// timeoutExceptLongLived preserves the normal API timeout without forcing the
// event stream to reconnect or breaking WebSocket upgrades. In particular,
// http.TimeoutHandler must not wrap call audio: its response writer does not
// implement http.Hijacker, which makes the WebSocket handshake fail with 501.
func timeoutExceptLongLived(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		timedNext := middleware.Timeout(timeout)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == eventStreamPath || isCallAudioPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			timedNext.ServeHTTP(w, r)
		})
	}
}

func isCallAudioPath(requestPath string) bool {
	const prefix = "/v1/calls/"
	const suffix = "/audio"
	return len(requestPath) > len(prefix)+len(suffix) &&
		requestPath[:len(prefix)] == prefix &&
		requestPath[len(requestPath)-len(suffix):] == suffix
}

func registerEventsStream(r chi.Router, st *store.Store, limiter eventStreamLimiter) {
	r.Get(eventStreamPath, func(w http.ResponseWriter, req *http.Request) {
		if !limiter.acquire() {
			http.Error(w, "too many event streams", http.StatusTooManyRequests)
			return
		}
		defer limiter.release()
		if st == nil {
			http.Error(w, "event store unavailable", http.StatusServiceUnavailable)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		cursor, _ := strconv.ParseInt(req.URL.Query().Get("since"), 10, 64)
		if cursor <= 0 {
			latest, err := st.ListEvents(req.Context(), store.EventFilter{Limit: 1})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(latest) > 0 {
				cursor = latest[0].ID
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		fmt.Fprintf(w, "retry: 2000\nevent: ready\nid: %d\ndata: {}\n\n", cursor)
		flusher.Flush()

		poll := time.NewTicker(750 * time.Millisecond)
		keepalive := time.NewTicker(15 * time.Second)
		defer poll.Stop()
		defer keepalive.Stop()

		for {
			select {
			case <-req.Context().Done():
				return
			case <-keepalive.C:
				fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			case <-poll.C:
				events, err := st.ListEvents(req.Context(), store.EventFilter{SinceID: cursor, Limit: 200})
				if err != nil {
					return
				}
				for _, event := range events {
					payload, err := json.Marshal(eventToJSON(event))
					if err != nil {
						continue
					}
					fmt.Fprintf(w, "event: sim7600\nid: %d\ndata: %s\n\n", event.ID, payload)
					cursor = event.ID
				}
				if len(events) > 0 {
					flusher.Flush()
				}
			}
		}
	})
}

func eventToJSON(e store.Event) eventJSON {
	return eventJSON{
		ID: e.ID, TS: e.TS.UTC().Format(time.RFC3339Nano),
		Kind: e.Kind, RefKind: e.RefKind, RefID: e.RefID,
		Raw: e.Raw, Detail: e.Detail,
	}
}
