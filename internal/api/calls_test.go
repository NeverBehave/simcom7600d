package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/modem"
)

func TestDial_HappyPath(t *testing.T) {
	fm := &fakeModem{dial: modem.Call{ID: "01HV", Direction: "out", RemoteAddr: "+12025551234", State: "dialing", StartedAt: time.Now().UTC()}}
	srv := newTestServer(t, fm)

	body, _ := json.Marshal(map[string]any{"to": "+12025551234"})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/calls", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "01HV", got["id"])
	require.Equal(t, "dialing", got["state"])
}

func TestHangup_NotFound(t *testing.T) {
	fm := &fakeModem{hangupErr: modem.ErrCallNotFound}
	srv := newTestServer(t, fm)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/calls/missing/hangup", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestMergeCalls_HappyPath(t *testing.T) {
	fm := &fakeModem{getCall: modem.Call{ID: "active", Direction: "out", RemoteAddr: "+12025551234", State: "active", StartedAt: time.Now().UTC()}}
	srv := newTestServer(t, fm)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/calls/active/merge", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "active", fm.mergedID)
}

func TestDTMF_BadDigits(t *testing.T) {
	fm := &fakeModem{dtmfErr: modem.ErrInvalidDTMF}
	srv := newTestServer(t, fm)
	body := bytes.NewReader([]byte(`{"digits":"abc","duration_ms":100}`))
	req, _ := http.NewRequest("POST", srv.URL+"/v1/calls/x/dtmf", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
