package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"sim7600d/internal/atexec"
)

// Admin holds optional back-end hooks for the admin endpoints.
type Admin struct {
	Reconcile func(ctx context.Context) error
	QueueInfo func() map[string]any
	Exec      *atexec.Executor // for /admin/at and /admin/at-reset
}

// --- GET /v1/admin/capabilities --------------------------------------------

type adminCapabilitiesInput struct {
	Authorization string `header:"Authorization" required:"true"`
}

type adminCapabilitiesOutput struct {
	Body struct {
		ATPassthrough bool `json:"at_passthrough"`
		ModemReset    bool `json:"modem_reset"`
	}
}

func registerAdminCapabilities(api huma.API, cfg Config) {
	huma.Register(api, huma.Operation{
		OperationID: "adminCapabilities",
		Method:      http.MethodGet,
		Path:        "/v1/admin/capabilities",
		Summary:     "List enabled admin capabilities",
		Tags:        []string{"admin"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, _ *adminCapabilitiesInput) (*adminCapabilitiesOutput, error) {
		out := adminCapabilitiesOutput{}
		out.Body.ATPassthrough = cfg.AllowATPassthrough
		out.Body.ModemReset = cfg.AllowModemReset
		return &out, nil
	})
}

// --- POST /v1/admin/reconcile -----------------------------------------------

type adminReconcileInput struct {
	Authorization string `header:"Authorization" required:"true"`
}

type adminReconcileOutput struct{}

func registerAdminReconcile(api huma.API, cfg Config) {
	huma.Register(api, huma.Operation{
		OperationID:   "adminReconcile",
		Method:        http.MethodPost,
		Path:          "/v1/admin/reconcile",
		Summary:       "Run reconciler synchronously",
		Tags:          []string{"admin"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, _ *adminReconcileInput) (*adminReconcileOutput, error) {
		if cfg.Admin == nil || cfg.Admin.Reconcile == nil {
			return nil, huma.Error503ServiceUnavailable("modem_unavailable: reconciler not wired")
		}
		if err := cfg.Admin.Reconcile(ctx); err != nil {
			return nil, huma.Error500InternalServerError("internal: " + err.Error())
		}
		return &adminReconcileOutput{}, nil
	})
}

// --- GET /v1/admin/queue ----------------------------------------------------

type adminQueueInput struct {
	Authorization string `header:"Authorization" required:"true"`
}

type adminQueueOutput struct {
	// Body is a free-form diagnostic shape; tag it so huma emits an open object.
	Body map[string]any
}

func registerAdminQueue(api huma.API, cfg Config) {
	huma.Register(api, huma.Operation{
		OperationID: "adminQueue",
		Method:      http.MethodGet,
		Path:        "/v1/admin/queue",
		Summary:     "Diagnostic info for atexec queue",
		Tags:        []string{"admin"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, _ *adminQueueInput) (*adminQueueOutput, error) {
		if cfg.Admin != nil && cfg.Admin.QueueInfo != nil {
			return &adminQueueOutput{Body: cfg.Admin.QueueInfo()}, nil
		}
		return &adminQueueOutput{Body: map[string]any{}}, nil
	})
}

// --- POST /v1/admin/at ------------------------------------------------------

type adminATInput struct {
	Authorization string `header:"Authorization" required:"true"`
	Body          struct {
		Cmd string `json:"cmd" required:"true" doc:"Raw AT command"`
		// *int (rather than int) so the field is optional. Huma marks non-pointer
		// struct fields as required by default.
		TimeoutMS *int `json:"timeout_ms,omitempty" doc:"Per-command timeout"`
	}
}

type adminATOutput struct {
	Body struct {
		Lines []string `json:"lines"`
		Final string   `json:"final"`
		Code  int      `json:"code"`
	}
}

func registerAdminAT(api huma.API, cfg Config) {
	huma.Register(api, huma.Operation{
		OperationID: "adminAT",
		Method:      http.MethodPost,
		Path:        "/v1/admin/at",
		Summary:     "Send a raw AT command (flag-gated)",
		Tags:        []string{"admin"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *adminATInput) (*adminATOutput, error) {
		if cfg.Admin == nil || cfg.Admin.Exec == nil {
			return nil, huma.Error503ServiceUnavailable("modem_unavailable: executor not wired")
		}
		req := atexec.Cmd(in.Body.Cmd)
		if in.Body.TimeoutMS != nil && *in.Body.TimeoutMS > 0 {
			req = req.WithTimeout(time.Duration(*in.Body.TimeoutMS) * time.Millisecond)
		}
		resp, err := cfg.Admin.Exec.Exec(ctx, req)
		if err != nil {
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		out := adminATOutput{}
		out.Body.Lines = resp.Lines
		out.Body.Final = resp.Final.Line
		out.Body.Code = resp.Final.Code
		return &out, nil
	})
}

// --- POST /v1/admin/at-reset ------------------------------------------------

type adminATResetInput struct {
	Authorization string `header:"Authorization" required:"true"`
}

type adminATResetOutput struct{}

func registerAdminATReset(api huma.API, cfg Config) {
	huma.Register(api, huma.Operation{
		OperationID:   "adminATReset",
		Method:        http.MethodPost,
		Path:          "/v1/admin/at-reset",
		Summary:       "AT+CFUN=1,1 — reboot modem (flag-gated)",
		Tags:          []string{"admin"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, _ *adminATResetInput) (*adminATResetOutput, error) {
		if cfg.Admin == nil || cfg.Admin.Exec == nil {
			return nil, huma.Error503ServiceUnavailable("modem_unavailable: executor not wired")
		}
		_, err := cfg.Admin.Exec.Exec(ctx, atexec.Cmd("AT+CFUN=1,1").WithTimeout(30*time.Second))
		if err != nil {
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		return &adminATResetOutput{}, nil
	})
}

// --- POST /v1/admin/vacuum --------------------------------------------------

type adminVacuumInput struct {
	Authorization string `header:"Authorization" required:"true"`
}

type adminVacuumOutput struct{}

func registerAdminVacuum(api huma.API, cfg Config) {
	huma.Register(api, huma.Operation{
		OperationID:   "adminVacuum",
		Method:        http.MethodPost,
		Path:          "/v1/admin/vacuum",
		Summary:       "Run SQLite VACUUM",
		Tags:          []string{"admin"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, _ *adminVacuumInput) (*adminVacuumOutput, error) {
		if cfg.Store == nil {
			return nil, huma.Error503ServiceUnavailable("modem_unavailable: store not wired")
		}
		if _, err := cfg.Store.DB().ExecContext(ctx, "VACUUM"); err != nil {
			return nil, huma.Error500InternalServerError("internal: " + err.Error())
		}
		return &adminVacuumOutput{}, nil
	})
}
