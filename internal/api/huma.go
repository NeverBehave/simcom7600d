package api

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// newHumaAPI builds the huma.API used to register typed operations on top of
// the supplied chi router. The returned API is what huma.Register(api, op, h)
// is called against. Bearer auth is declared as a security scheme so it
// appears in the spec; actual auth enforcement stays in the chi middleware
// (auth.go) — huma.SecurityScheme is metadata, not behaviour.
func newHumaAPI(r chi.Router) huma.API {
	cfg := huma.DefaultConfig("SIM7600D Control API", "1.0.0")
	cfg.Info.Description = "Cellular modem control plane: status, SMS, calls, events, admin."
	// Disable huma's built-in spec routes; we serve the committed, embedded
	// openapi.json ourselves on the outer (unauthenticated) router.
	cfg.OpenAPIPath = ""
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "opaque",
		},
	}
	return humachi.New(r, cfg)
}

func init() {
	// huma.NewError is the global function used by every huma.ErrorXxx helper
	// (Error400BadRequest, Error503ServiceUnavailable, etc.). Replace it so
	// all 4xx/5xx errors come out in our envelope shape instead of huma's
	// RFC-7807 default. Operations remain free to return errors with custom
	// status/code by constructing *apiError directly.
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		var details any
		if len(errs) > 0 {
			msgs := make([]string, 0, len(errs))
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			details = map[string]any{"errors": msgs}
		}
		return newAPIError(status, codeForStatus(status), message, details)
	}
}
