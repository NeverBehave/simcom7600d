// Package api wires HTTP handlers over modem.Modem. Handlers are thin: validate,
// call facade, serialize. No persistence access here.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	uistatic "sim7600d/web/ui"

	"sim7600d/internal/modem"
	"sim7600d/internal/store"
	"sim7600d/internal/voicemail"
)

type VoicemailSyncer interface {
	Sync(context.Context) (voicemail.SyncResult, error)
}

type Config struct {
	AuthToken          string
	Modem              modem.Modem
	Store              *store.Store // for /v1/events, /v1/admin/queue
	Admin              *Admin
	CallAudio          http.Handler
	Voicemail          VoicemailSyncer
	AllowATPassthrough bool
	AllowModemReset    bool
}

// Server bundles the running router with the OpenAPI 3.1 spec rendered from
// its huma operation registrations. The spec is byte-stable for a given build
// of the binary; cmd/openapi-dump uses it to produce the committed
// internal/api/openapi.json.
type Server struct {
	Handler http.Handler
	Spec    []byte // OpenAPI 3.1 JSON, indented
}

// NewRouter is the legacy entrypoint used by cmd/sim7600d. Returns just the
// http.Handler; equivalent to NewServer(cfg).Handler.
func NewRouter(cfg Config) http.Handler {
	return NewServer(cfg).Handler
}

// NewServer constructs the API and computes the OpenAPI spec it would serve.
// Callers that want the spec (cmd/openapi-dump, the embedded /openapi.json
// handler, the drift test) call this directly.
func NewServer(cfg Config) *Server {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(limitRequestBody(1 << 20))
	r.Use(timeoutExceptLongLived(100 * time.Second))

	r.Get("/openapi.json", handleOpenAPISpec()) // unauthenticated by design
	if cfg.CallAudio != nil {
		r.With(audioAuthMiddleware(cfg.AuthToken)).Get("/v1/calls/{id}/audio", cfg.CallAudio.ServeHTTP)
	}

	authedR := r.With(authMiddleware(cfg.AuthToken))
	registerEventsStream(authedR, cfg.Store, newEventStreamLimiter(4))
	registerVoicemailAudioRoute(authedR, cfg.Store)
	humaAuthedAPI := newHumaAPI(authedR.With(limitConcurrentRequests(16)))

	registerStatus(humaAuthedAPI, cfg.Modem)
	registerSMSSend(humaAuthedAPI, cfg.Modem)
	registerSMSList(humaAuthedAPI, cfg.Modem)
	registerSMSGet(humaAuthedAPI, cfg.Modem)
	registerSMSDelete(humaAuthedAPI, cfg.Modem)
	registerCallsList(humaAuthedAPI, cfg.Modem)
	registerCallsDial(humaAuthedAPI, cfg.Modem)
	registerCallsGet(humaAuthedAPI, cfg.Modem)
	registerCallsAnswer(humaAuthedAPI, cfg.Modem)
	registerCallsReject(humaAuthedAPI, cfg.Modem)
	registerCallsHold(humaAuthedAPI, cfg.Modem)
	registerCallsResume(humaAuthedAPI, cfg.Modem)
	registerCallsMerge(humaAuthedAPI, cfg.Modem)
	registerCallsHangup(humaAuthedAPI, cfg.Modem)
	registerCallsDTMF(humaAuthedAPI, cfg.Modem)
	registerCallForwardingGet(humaAuthedAPI, cfg.Modem)
	registerCallForwardingUpdate(humaAuthedAPI, cfg.Modem)
	registerVoicemails(humaAuthedAPI, cfg.Store, cfg.Voicemail)
	registerEventsList(humaAuthedAPI, cfg.Store)
	registerAdminCapabilities(humaAuthedAPI, cfg)
	registerAdminReconcile(humaAuthedAPI, cfg)
	registerAdminQueue(humaAuthedAPI, cfg)
	if cfg.AllowATPassthrough {
		registerAdminAT(humaAuthedAPI, cfg)
	}
	if cfg.AllowModemReset {
		registerAdminATReset(humaAuthedAPI, cfg)
	}
	registerAdminVacuum(humaAuthedAPI, cfg)

	r.Handle("/*", uistatic.Handler())

	specBytes, err := json.MarshalIndent(humaAuthedAPI.OpenAPI(), "", "  ")
	if err != nil {
		// Spec generation should never fail at startup; panic is appropriate.
		panic("api: failed to render openapi spec: " + err.Error())
	}

	return &Server{Handler: r, Spec: specBytes}
}

func limitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func limitConcurrentRequests(max int) func(http.Handler) http.Handler {
	if max < 1 {
		max = 1
	}
	slots := make(chan struct{}, max)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				next.ServeHTTP(w, r)
			default:
				w.Header().Set("Retry-After", "1")
				http.Error(w, "too many concurrent requests", http.StatusTooManyRequests)
			}
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self)")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.URL.Path == "/v1" || len(r.URL.Path) > len("/v1/") && r.URL.Path[:len("/v1/")] == "/v1/" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
