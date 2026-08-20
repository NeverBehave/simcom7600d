package api

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"sim7600d/internal/store"
	"sim7600d/internal/voicemail"
)

type voicemailResponse struct {
	ID          string `json:"id"`
	SourceID    string `json:"source_id"`
	From        string `json:"from"`
	ReceivedAt  string `json:"received_at" format:"date-time"`
	DurationMS  int    `json:"duration_ms"`
	Read        bool   `json:"read"`
	ReadAt      string `json:"read_at,omitempty" format:"date-time"`
	ContentType string `json:"content_type"`
	AudioBytes  int    `json:"audio_bytes"`
	AudioURL    string `json:"audio_url"`
	Transcript  string `json:"transcript,omitempty"`
	SourceSMSID string `json:"source_sms_id,omitempty"`
}

type voicemailListInput struct {
	Authorization string `header:"Authorization" required:"true"`
	Unread        bool   `query:"unread" doc:"Return only unheard voicemail"`
	Limit         int    `query:"limit" minimum:"1" maximum:"500" doc:"Maximum items"`
}

type voicemailListOutput struct {
	Body struct {
		Items []voicemailResponse `json:"items"`
	}
}

type voicemailGetInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
}

type voicemailGetOutput struct{ Body voicemailResponse }

type voicemailUpdateInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
	Body          struct {
		Read bool `json:"read" doc:"Mark the voicemail heard or unheard"`
	}
}

type voicemailUpdateOutput struct{ Body voicemailResponse }

type voicemailDeleteInput struct {
	Authorization string `header:"Authorization" required:"true"`
	ID            string `path:"id" required:"true"`
}

type voicemailDeleteOutput struct{}

type voicemailSyncInput struct {
	Authorization string `header:"Authorization" required:"true"`
}

type voicemailSyncOutput struct{ Body voicemail.SyncResult }

func registerVoicemails(api huma.API, st *store.Store, syncer VoicemailSyncer) {
	security := []map[string][]string{{"bearerAuth": {}}}
	huma.Register(api, huma.Operation{
		OperationID: "voicemailsSync", Method: http.MethodPost, Path: "/v1/voicemails/sync",
		Summary: "Sync voicemail mailbox", Tags: []string{"voicemail"}, Security: security,
	}, func(ctx context.Context, in *voicemailSyncInput) (*voicemailSyncOutput, error) {
		if syncer == nil {
			return nil, huma.Error503ServiceUnavailable("voicemail sync unavailable")
		}
		result, err := syncer.Sync(ctx)
		if err != nil {
			return nil, huma.Error502BadGateway("voicemail sync failed")
		}
		return &voicemailSyncOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "voicemailsList", Method: http.MethodGet, Path: "/v1/voicemails",
		Summary: "List voicemail", Tags: []string{"voicemail"}, Security: security,
	}, func(ctx context.Context, in *voicemailListInput) (*voicemailListOutput, error) {
		if st == nil {
			return nil, huma.Error503ServiceUnavailable("voicemail store unavailable")
		}
		items, err := st.ListVoicemails(ctx, store.VoicemailFilter{UnreadOnly: in.Unread, Limit: in.Limit})
		if err != nil {
			return nil, huma.Error500InternalServerError("voicemail query failed")
		}
		out := &voicemailListOutput{}
		out.Body.Items = make([]voicemailResponse, len(items))
		for i, item := range items {
			out.Body.Items[i] = voicemailJSON(item)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "voicemailsGet", Method: http.MethodGet, Path: "/v1/voicemails/{id}",
		Summary: "Get voicemail metadata", Tags: []string{"voicemail"}, Security: security,
	}, func(ctx context.Context, in *voicemailGetInput) (*voicemailGetOutput, error) {
		item, err := getVoicemailMetadata(ctx, st, in.ID)
		if err != nil {
			return nil, err
		}
		return &voicemailGetOutput{Body: voicemailJSON(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "voicemailsUpdate", Method: http.MethodPut, Path: "/v1/voicemails/{id}",
		Summary: "Update voicemail read state", Tags: []string{"voicemail"}, Security: security,
	}, func(ctx context.Context, in *voicemailUpdateInput) (*voicemailUpdateOutput, error) {
		if st == nil {
			return nil, huma.Error503ServiceUnavailable("voicemail store unavailable")
		}
		item, err := st.SetVoicemailRead(ctx, in.ID, in.Body.Read)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, huma.Error404NotFound("voicemail not found")
		}
		if err != nil {
			return nil, huma.Error500InternalServerError("voicemail update failed")
		}
		return &voicemailUpdateOutput{Body: voicemailJSON(item)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "voicemailsDelete", Method: http.MethodDelete, Path: "/v1/voicemails/{id}",
		Summary: "Delete voicemail", Tags: []string{"voicemail"}, Security: security,
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *voicemailDeleteInput) (*voicemailDeleteOutput, error) {
		if st == nil {
			return nil, huma.Error503ServiceUnavailable("voicemail store unavailable")
		}
		err := st.DeleteVoicemail(ctx, in.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, huma.Error404NotFound("voicemail not found")
		}
		if err != nil {
			return nil, huma.Error500InternalServerError("voicemail delete failed")
		}
		return &voicemailDeleteOutput{}, nil
	})

	registerVoicemailAudioDoc(api)
}

func getVoicemailMetadata(ctx context.Context, st *store.Store, id string) (store.Voicemail, error) {
	if st == nil {
		return store.Voicemail{}, huma.Error503ServiceUnavailable("voicemail store unavailable")
	}
	item, err := st.GetVoicemail(ctx, id, false)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Voicemail{}, huma.Error404NotFound("voicemail not found")
	}
	if err != nil {
		return store.Voicemail{}, huma.Error500InternalServerError("voicemail query failed")
	}
	return item, nil
}

func voicemailJSON(item store.Voicemail) voicemailResponse {
	out := voicemailResponse{
		ID: item.ID, SourceID: item.SourceID, From: item.FromAddr,
		ReceivedAt: item.ReceivedAt.UTC().Format(time.RFC3339), DurationMS: item.DurationMS,
		Read: !item.ReadAt.IsZero(), ContentType: item.ContentType, AudioBytes: item.AudioBytes,
		AudioURL: "/v1/voicemails/" + item.ID + "/audio", Transcript: item.Transcript,
		SourceSMSID: item.SourceSMSID,
	}
	if !item.ReadAt.IsZero() {
		out.ReadAt = item.ReadAt.UTC().Format(time.RFC3339)
	}
	return out
}

func registerVoicemailAudioRoute(r chi.Router, st *store.Store) {
	r.Get("/v1/voicemails/{id}/audio", func(w http.ResponseWriter, req *http.Request) {
		if st == nil {
			http.Error(w, "voicemail store unavailable", http.StatusServiceUnavailable)
			return
		}
		item, err := st.GetVoicemail(req.Context(), chi.URLParam(req, "id"), true)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "voicemail not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "voicemail audio unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", item.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="voicemail-%s%s"`, item.ID, audioExtension(item.ContentType)))
		http.ServeContent(w, req, "voicemail"+audioExtension(item.ContentType), item.UpdatedAt, bytes.NewReader(item.Audio))
	})
}

func audioExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/amr":
		return ".amr"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/aac":
		return ".m4a"
	default:
		return ".audio"
	}
}

func registerVoicemailAudioDoc(api huma.API) {
	api.OpenAPI().AddOperation(&huma.Operation{
		OperationID: "voicemailsAudio", Method: http.MethodGet, Path: "/v1/voicemails/{id}/audio",
		Summary: "Play or download voicemail audio", Tags: []string{"voicemail"},
		Security: []map[string][]string{{"bearerAuth": {}}},
		Parameters: []*huma.Param{
			{Name: "Authorization", In: "header", Required: true, Schema: &huma.Schema{Type: huma.TypeString}},
			{Name: "id", In: "path", Required: true, Schema: &huma.Schema{Type: huma.TypeString}},
		},
		Responses: map[string]*huma.Response{"200": {
			Description: "Voicemail audio with byte-range support",
			Content:     map[string]*huma.MediaType{"audio/*": {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}}},
		}},
	})
}
