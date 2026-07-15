package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"sim7600d/internal/store"
)

type eventJSON struct {
	ID      int64  `json:"id"`
	TS      string `json:"ts" format:"date-time"`
	Kind    string `json:"kind"`
	RefKind string `json:"ref_kind,omitempty"`
	RefID   string `json:"ref_id,omitempty"`
	Raw     string `json:"raw,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type eventsListInput struct {
	Authorization string `header:"Authorization" required:"true"`
	Since         int64  `query:"since" doc:"Numeric event ID; return events strictly newer"`
	Kind          string `query:"kind" doc:"Filter by event kind (e.g. sms.arrived)"`
	Limit         int    `query:"limit"`
}

type eventsListOutput struct {
	Body struct {
		Items []eventJSON `json:"items"`
	}
}

func registerEventsList(api huma.API, st *store.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "eventsList",
		Method:      http.MethodGet,
		Path:        "/v1/events",
		Summary:     "List daemon events",
		Tags:        []string{"events"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, func(ctx context.Context, in *eventsListInput) (*eventsListOutput, error) {
		out := eventsListOutput{}
		out.Body.Items = []eventJSON{}
		if st == nil {
			return &out, nil
		}
		evs, err := st.ListEvents(ctx, store.EventFilter{
			SinceID: in.Since, Kind: in.Kind, Limit: in.Limit,
		})
		if err != nil {
			return nil, huma.Error500InternalServerError("internal: " + err.Error())
		}
		out.Body.Items = make([]eventJSON, len(evs))
		for i, e := range evs {
			out.Body.Items[i] = eventToJSON(e)
		}
		return &out, nil
	})
}
