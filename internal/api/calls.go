package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"sim7600d/internal/modem"
)

type callJSON struct {
	ID         string `json:"id"`
	Direction  string `json:"direction" enum:"in,out"`
	To         string `json:"to,omitempty"`
	From       string `json:"from,omitempty"`
	State      string `json:"state" enum:"ringing,dialing,alerting,active,held,ended,missed,rejected"`
	EndReason  string `json:"end_reason,omitempty"`
	StartedAt  string `json:"started_at" format:"date-time"`
	AnsweredAt string `json:"answered_at,omitempty" format:"date-time"`
	EndedAt    string `json:"ended_at,omitempty" format:"date-time"`
	DurationMS int    `json:"duration_ms,omitempty"`
}

func callToJSON(c modem.Call) callJSON {
	out := callJSON{
		ID:         c.ID,
		Direction:  c.Direction,
		State:      c.State,
		EndReason:  c.EndReason,
		StartedAt:  c.StartedAt.UTC().Format(time.RFC3339),
		DurationMS: c.DurationMS,
	}
	if c.Direction == "out" {
		out.To = c.RemoteAddr
	} else {
		out.From = c.RemoteAddr
	}
	if !c.AnsweredAt.IsZero() {
		out.AnsweredAt = c.AnsweredAt.UTC().Format(time.RFC3339)
	}
	if !c.EndedAt.IsZero() {
		out.EndedAt = c.EndedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// --- GET /v1/calls ----------------------------------------------------------

type callsListInput struct {
	Authorization string `header:"Authorization" required:"true"`
	Since         string `query:"since"`
	Limit         int    `query:"limit"`
}

type callsListOutput struct {
	Body struct {
		Items []callJSON `json:"items"`
	}
}

func registerCallsList(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID: "callsList",
		Method:      http.MethodGet,
		Path:        "/v1/calls",
		Summary:     "List calls",
		Tags:        []string{"calls"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *callsListInput) (*callsListOutput, error) {
		items, err := m.ListCalls(ctx, in.Since, in.Limit)
		if err != nil {
			return nil, huma.Error500InternalServerError("internal: " + err.Error())
		}
		out := callsListOutput{}
		out.Body.Items = make([]callJSON, len(items))
		for i, c := range items {
			out.Body.Items[i] = callToJSON(c)
		}
		return &out, nil
	})
}

// --- POST /v1/calls ---------------------------------------------------------

type callsDialInput struct {
	Authorization  string `header:"Authorization" required:"true"`
	IdempotencyKey string `header:"Idempotency-Key"`
	Raw            bool   `query:"raw"`
	Body           struct {
		To string `json:"to" required:"true" example:"+15551234567"`
	}
}

type callsDialOutput struct {
	Body callJSON
}

func registerCallsDial(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID:   "callsDial",
		Method:        http.MethodPost,
		Path:          "/v1/calls",
		Summary:       "Place an outbound call",
		Tags:          []string{"calls"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *callsDialInput) (*callsDialOutput, error) {
		to, err := normalizePhone(in.Body.To, in.Raw)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		c, err := m.Dial(ctx, to, in.IdempotencyKey)
		if err != nil {
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		return &callsDialOutput{Body: callToJSON(c)}, nil
	})
}

// --- GET /v1/calls/{id} -----------------------------------------------------

type callsGetInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
}

type callsGetOutput struct {
	Body callJSON
}

func registerCallsGet(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID: "callsGet",
		Method:      http.MethodGet,
		Path:        "/v1/calls/{id}",
		Summary:     "Get a call by ID",
		Tags:        []string{"calls"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *callsGetInput) (*callsGetOutput, error) {
		c, err := m.GetCall(ctx, in.ID)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		return &callsGetOutput{Body: callToJSON(c)}, nil
	})
}

// --- POST /v1/calls/{id}/{action} -------------------------------------------

type callActionInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
}

type callActionOutput struct {
	Body callJSON
}

func registerCallAction(
	api huma.API, m modem.Modem,
	opID, pathSeg, summary string,
	action func(context.Context, string) error,
) {
	huma.Register(api, huma.Operation{
		OperationID: opID,
		Method:      http.MethodPost,
		Path:        "/v1/calls/{id}/" + pathSeg,
		Summary:     summary,
		Tags:        []string{"calls"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *callActionInput) (*callActionOutput, error) {
		if err := action(ctx, in.ID); err != nil {
			if errors.Is(err, modem.ErrCallNotFound) {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		c, _ := m.GetCall(ctx, in.ID)
		return &callActionOutput{Body: callToJSON(c)}, nil
	})
}

func registerCallsAnswer(api huma.API, m modem.Modem) {
	registerCallAction(api, m, "callsAnswer", "answer", "Answer a ringing call",
		func(ctx context.Context, id string) error { return m.Answer(ctx, id) })
}

func registerCallsReject(api huma.API, m modem.Modem) {
	registerCallAction(api, m, "callsReject", "reject", "Reject a ringing call",
		func(ctx context.Context, id string) error { return m.Reject(ctx, id) })
}

func registerCallsHold(api huma.API, m modem.Modem) {
	registerCallAction(api, m, "callsHold", "hold", "Place an active call on hold",
		func(ctx context.Context, id string) error { return m.Hold(ctx, id) })
}

func registerCallsResume(api huma.API, m modem.Modem) {
	registerCallAction(api, m, "callsResume", "resume", "Resume a held call",
		func(ctx context.Context, id string) error { return m.Resume(ctx, id) })
}

func registerCallsMerge(api huma.API, m modem.Modem) {
	registerCallAction(api, m, "callsMerge", "merge", "Merge an active and a held call",
		func(ctx context.Context, id string) error { return m.MergeCalls(ctx, id) })
}

func registerCallsHangup(api huma.API, m modem.Modem) {
	registerCallAction(api, m, "callsHangup", "hangup", "Hang up an active call",
		func(ctx context.Context, id string) error { return m.Hangup(ctx, id) })
}

// --- POST /v1/calls/{id}/dtmf ----------------------------------------------

type callsDTMFInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
	Body          struct {
		Digits string `json:"digits" required:"true" doc:"DTMF digits ([0-9A-D*#]+)"`
		// *int (rather than int) so the field is optional. Huma marks non-pointer
		// struct fields as required by default.
		DurationMS *int `json:"duration_ms,omitempty" doc:"Per-digit duration in ms"`
	}
}

type callsDTMFOutput struct{}

func registerCallsDTMF(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID:   "callsDTMF",
		Method:        http.MethodPost,
		Path:          "/v1/calls/{id}/dtmf",
		Summary:       "Send DTMF on an active call",
		Tags:          []string{"calls"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *callsDTMFInput) (*callsDTMFOutput, error) {
		dur := 0
		if in.Body.DurationMS != nil {
			dur = *in.Body.DurationMS
		}
		if err := m.SendDTMF(ctx, in.ID, in.Body.Digits, dur); err != nil {
			if errors.Is(err, modem.ErrInvalidDTMF) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		return &callsDTMFOutput{}, nil
	})
}
