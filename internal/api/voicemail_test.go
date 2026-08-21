package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/store"
	"sim7600d/internal/voicemail"
)

type fakeVoicemailSyncer struct{ calls int }

func (f *fakeVoicemailSyncer) Sync(context.Context) (voicemail.SyncResult, error) {
	f.calls++
	return voicemail.SyncResult{Fetched: 2, Added: 1, Updated: 1}, nil
}

func TestVoicemailAPIPlaybackReadAndDelete(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	item, err := st.UpsertVoicemail(context.Background(), store.Voicemail{
		SourceID: "imap-7", FromAddr: "+15551234567", ReceivedAt: time.Now().UTC(),
		DurationMS: 12000, ContentType: "audio/wav", Audio: []byte("RIFF0123456789"),
	})
	require.NoError(t, err)
	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret", Store: st})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/voicemails", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list struct {
		Items []voicemailResponse `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	require.Len(t, list.Items, 1)
	require.False(t, list.Items[0].Read)
	require.Equal(t, len("RIFF0123456789"), list.Items[0].AudioBytes)

	audioReq, _ := http.NewRequest(http.MethodGet, srv.URL+list.Items[0].AudioURL, nil)
	audioReq.Header.Set("Authorization", "Bearer secret")
	audioReq.Header.Set("Range", "bytes=4-7")
	audioResp, err := http.DefaultClient.Do(audioReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, audioResp.StatusCode)
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(audioResp.Body)
	require.NoError(t, err)
	require.Equal(t, "0123", buf.String())

	updateReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/voicemails/"+item.ID, bytes.NewBufferString(`{"read":true}`))
	updateReq.Header.Set("Authorization", "Bearer secret")
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(updateReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	deleteReq, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/voicemails/"+item.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer secret")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
}

func TestVoicemailAudioRequiresAuth(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret", Store: st})
	resp, err := http.Get(srv.URL + "/v1/voicemails/missing/audio")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestVoicemailSyncAPI(t *testing.T) {
	syncer := &fakeVoicemailSyncer{}
	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret", Voicemail: syncer})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/voicemails/sync", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 1, syncer.calls)
	var result voicemail.SyncResult
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Equal(t, voicemail.SyncResult{Fetched: 2, Added: 1, Updated: 1}, result)
}

func TestVoicemailSyncUnavailableWhenDisabled(t *testing.T) {
	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/voicemails/sync", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
