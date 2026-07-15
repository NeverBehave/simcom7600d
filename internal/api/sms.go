package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/nyaruka/phonenumbers"

	"sim7600d/internal/modem"
)

// --- shared response shapes ---------------------------------------------------

type sendSMSPart struct {
	MR int `json:"mr"`
}

type outboundSMSResponse struct {
	ID          string        `json:"id"`
	Direction   string        `json:"direction" enum:"out"`
	To          string        `json:"to"`
	Body        string        `json:"body"`
	State       string        `json:"state" enum:"queued,submitted,accepted,delivered,failed,indeterminate"`
	Parts       []sendSMSPart `json:"parts"`
	Encoding    string        `json:"encoding" enum:"gsm7,ucs2,8bit"`
	TS          string        `json:"ts" format:"date-time"`
	ErrorCode   string        `json:"error_code,omitempty"`
	ErrorDetail string        `json:"error_detail,omitempty"`
}

type inboundSMSResponse struct {
	ID         string `json:"id"`
	Direction  string `json:"direction" enum:"in"`
	From       string `json:"from"`
	Body       string `json:"body"`
	ReceivedAt string `json:"received_at" format:"date-time"`
	SMSCTime   string `json:"smsc_ts" format:"date-time"`
	Parts      int    `json:"parts"`
	Encoding   string `json:"encoding" enum:"gsm7,ucs2,8bit"`
	Incomplete bool   `json:"incomplete"`
}

// --- POST /v1/sms ------------------------------------------------------------

type smsSendInput struct {
	Authorization  string `header:"Authorization" required:"true" doc:"Bearer token"`
	IdempotencyKey string `header:"Idempotency-Key" doc:"Replay-protection key (24h cache)"`
	Raw            bool   `query:"raw" doc:"If 1, accept any non-E.164 destination (e.g. short codes)"`
	Body           struct {
		To   string `json:"to" required:"true" doc:"E.164 number or short code if raw=1" example:"+15551234567"`
		Body string `json:"body" required:"true" doc:"Message text"`
		// *bool (rather than bool) so the field is optional in the request body.
		// Huma marks non-pointer struct fields as required by default.
		DeliveryReport *bool `json:"delivery_report,omitempty" doc:"Request delivery report from carrier"`
	}
}

type smsSendOutput struct {
	Body outboundSMSResponse
}

func registerSMSSend(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID:   "smsSend",
		Method:        http.MethodPost,
		Path:          "/v1/sms",
		Summary:       "Send an SMS",
		Tags:          []string{"sms"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *smsSendInput) (*smsSendOutput, error) {
		to, err := normalizePhone(in.Body.To, in.Raw)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		dr := in.Body.DeliveryReport != nil && *in.Body.DeliveryReport
		opts := modem.SendOpts{
			IdemKey:        in.IdempotencyKey,
			DeliveryReport: dr,
		}
		out, err := m.SendSMS(ctx, to, in.Body.Body, opts)
		if err != nil {
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		return &smsSendOutput{Body: outboundToResponse(out)}, nil
	})
}

// --- GET /v1/sms -------------------------------------------------------------

type smsListInput struct {
	Authorization string `header:"Authorization" required:"true"`
	Direction     string `query:"direction" enum:"in,out" doc:"Default: in"`
	Since         string `query:"since" doc:"ULID; return items strictly newer"`
	Limit         int    `query:"limit" doc:"Max items"`
}

type smsListOutput struct {
	Body smsListBody
}

type smsListBody struct {
	Items []any `json:"items" doc:"Either inbound or outbound items depending on the direction query parameter"`
}

func registerSMSList(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID: "smsList",
		Method:      http.MethodGet,
		Path:        "/v1/sms",
		Summary:     "List SMS messages",
		Tags:        []string{"sms"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *smsListInput) (*smsListOutput, error) {
		switch in.Direction {
		case "out":
			items, err := m.ListOutbound(ctx, in.Since, in.Limit)
			if err != nil {
				return nil, huma.Error500InternalServerError("internal: " + err.Error())
			}
			out := make([]any, len(items))
			for i, it := range items {
				out[i] = outboundToResponse(it)
			}
			return &smsListOutput{Body: smsListBody{Items: out}}, nil
		default:
			items, err := m.ListInbound(ctx, in.Since, in.Limit)
			if err != nil {
				return nil, huma.Error500InternalServerError("internal: " + err.Error())
			}
			out := make([]any, len(items))
			for i, it := range items {
				out[i] = inboundToResponse(it)
			}
			return &smsListOutput{Body: smsListBody{Items: out}}, nil
		}
	})
}

// --- GET /v1/sms/{id} --------------------------------------------------------

type smsGetInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
}

type smsGetOutput struct {
	Body any
}

func registerSMSGet(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID: "smsGet",
		Method:      http.MethodGet,
		Path:        "/v1/sms/{id}",
		Summary:     "Get a single SMS by ID",
		Tags:        []string{"sms"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *smsGetInput) (*smsGetOutput, error) {
		// Try outbound first.
		if o, err := m.GetOutbound(ctx, in.ID); err == nil && o.ID == in.ID {
			return &smsGetOutput{Body: outboundToResponse(o)}, nil
		}
		if it, err := m.GetInbound(ctx, in.ID); err == nil {
			return &smsGetOutput{Body: inboundToResponse(it)}, nil
		}
		return nil, huma.Error404NotFound("no sms with that id")
	})
}

// --- DELETE /v1/sms/{id} -----------------------------------------------------

type smsDeleteInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
}

type smsDeleteOutput struct{}

func registerSMSDelete(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID:   "smsDelete",
		Method:        http.MethodDelete,
		Path:          "/v1/sms/{id}",
		Summary:       "Delete an SMS from history",
		Tags:          []string{"sms"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *smsDeleteInput) (*smsDeleteOutput, error) {
		if err := m.DeleteSMS(ctx, in.ID); err != nil {
			return nil, huma.Error500InternalServerError("internal: " + err.Error())
		}
		return &smsDeleteOutput{}, nil
	})
}

// --- helpers -----------------------------------------------------------------

func outboundToResponse(o modem.Outbound) outboundSMSResponse {
	parts := make([]sendSMSPart, len(o.Parts))
	for i, p := range o.Parts {
		parts[i] = sendSMSPart{MR: p.MR}
	}
	return outboundSMSResponse{
		ID: o.ID, Direction: "out", To: o.To, Body: o.Body,
		State: o.State, Parts: parts, Encoding: o.Encoding,
		TS: o.CreatedAt.UTC().Format(time.RFC3339), ErrorCode: o.ErrorCode,
		ErrorDetail: o.ErrorDetail,
	}
}

func inboundToResponse(it modem.Inbound) inboundSMSResponse {
	return inboundSMSResponse{
		ID: it.ID, Direction: "in", From: it.From, Body: it.Body,
		ReceivedAt: it.ReceivedAt.UTC().Format(time.RFC3339),
		SMSCTime:   it.SMSCTime.UTC().Format(time.RFC3339),
		Parts:      it.Parts, Encoding: it.Encoding, Incomplete: it.Incomplete,
	}
}

func normalizePhone(s string, raw bool) (string, error) {
	if s == "" {
		return "", errors.New("phone required")
	}
	if raw {
		return s, nil
	}
	parsed, err := phonenumbers.Parse(s, "US")
	if err != nil {
		return "", err
	}
	if !phonenumbers.IsValidNumber(parsed) {
		return "", errors.New("invalid phone number")
	}
	return phonenumbers.Format(parsed, phonenumbers.E164), nil
}
