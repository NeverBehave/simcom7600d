package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"sim7600d/internal/modem"
)

type callForwardingRuleJSON struct {
	Reason         string `json:"reason" enum:"unconditional,busy,no_reply,unreachable"`
	Enabled        bool   `json:"enabled"`
	Number         string `json:"number,omitempty" example:"+12025551234"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" minimum:"5" maximum:"30" multipleOf:"5" doc:"Ring time before forwarding; only used for no_reply"`
	Available      bool   `json:"available" doc:"True when the network confirmed the current state"`
	Error          string `json:"error,omitempty" doc:"Carrier or modem status error when current state is unavailable"`
}

type callForwardingGetInput struct {
	Authorization string `header:"Authorization" required:"true"`
}

type callForwardingGetOutput struct {
	Body struct {
		Items []callForwardingRuleJSON `json:"items"`
	}
}

func registerCallForwardingGet(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID: "callForwardingGet",
		Method:      http.MethodGet,
		Path:        "/v1/call-forwarding",
		Summary:     "Get voice call-forwarding status",
		Tags:        []string{"call-forwarding"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, _ *callForwardingGetInput) (*callForwardingGetOutput, error) {
		rules, err := m.GetCallForwarding(ctx)
		if err != nil {
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		out := &callForwardingGetOutput{}
		out.Body.Items = make([]callForwardingRuleJSON, len(rules))
		for i, rule := range rules {
			out.Body.Items[i] = callForwardingToJSON(rule)
		}
		return out, nil
	})
}

type callForwardingUpdateInput struct {
	Authorization string `header:"Authorization" required:"true"`
	Reason        string `path:"reason" required:"true" enum:"unconditional,busy,no_reply,unreachable"`
	Body          struct {
		Enabled        bool   `json:"enabled"`
		Number         string `json:"number,omitempty" example:"+12025551234"`
		TimeoutSeconds *int   `json:"timeout_seconds,omitempty" minimum:"5" maximum:"30" multipleOf:"5"`
	}
}

type callForwardingUpdateOutput struct {
	Body callForwardingRuleJSON
}

func registerCallForwardingUpdate(api huma.API, m modem.Modem) {
	huma.Register(api, huma.Operation{
		OperationID: "callForwardingUpdate",
		Method:      http.MethodPut,
		Path:        "/v1/call-forwarding/{reason}",
		Summary:     "Update a voice call-forwarding rule",
		Tags:        []string{"call-forwarding"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *callForwardingUpdateInput) (*callForwardingUpdateOutput, error) {
		rule := modem.CallForwardingRule{Reason: in.Reason, Enabled: in.Body.Enabled}
		if in.Body.Enabled {
			number, err := normalizePhone(in.Body.Number, false)
			if err != nil {
				return nil, huma.Error400BadRequest(err.Error())
			}
			rule.Number = number
		}
		if in.Body.TimeoutSeconds != nil {
			rule.TimeoutSeconds = *in.Body.TimeoutSeconds
		}

		updated, err := m.SetCallForwarding(ctx, rule)
		if err != nil {
			if errors.Is(err, modem.ErrInvalidCallForwardingReason) ||
				errors.Is(err, modem.ErrInvalidCallForwardingNumber) ||
				errors.Is(err, modem.ErrInvalidCallForwardingTimeout) {
				return nil, huma.Error400BadRequest(err.Error())
			}
			return nil, huma.Error502BadGateway("modem_error: " + err.Error())
		}
		return &callForwardingUpdateOutput{Body: callForwardingToJSON(updated)}, nil
	})
}

func callForwardingToJSON(rule modem.CallForwardingRule) callForwardingRuleJSON {
	return callForwardingRuleJSON{
		Reason: rule.Reason, Enabled: rule.Enabled, Number: rule.Number,
		TimeoutSeconds: rule.TimeoutSeconds, Available: rule.Available, Error: rule.Error,
	}
}
