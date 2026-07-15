package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/modem"
)

func TestSendSMS_HappyPath(t *testing.T) {
	fm := &fakeModem{
		sendOut: modem.Outbound{
			ID: "01HV", State: "submitted", Encoding: "gsm7",
			Parts: []modem.OutboundPart{{MR: 7}}, CreatedAt: time.Now().UTC(),
		},
	}
	srv := newTestServer(t, fm)

	body, _ := json.Marshal(map[string]any{"to": "+12025551234", "body": "hi"})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/sms", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "01HV", got["id"])
	require.Equal(t, "submitted", got["state"])
}

func TestDeleteSMS_Returns204(t *testing.T) {
	srv := newTestServer(t, &fakeModem{})
	req, _ := http.NewRequest("DELETE", srv.URL+"/v1/sms/01HV", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestSendSMS_ValidatesTo(t *testing.T) {
	srv := newTestServer(t, &fakeModem{})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/sms",
		strings.NewReader(`{"to":"hello","body":"x"}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestErrorEnvelope_Shape(t *testing.T) {
	srv := newTestServer(t, &fakeModem{})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/sms",
		strings.NewReader(`{"to":"hello","body":"x"}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	errObj, ok := got["error"].(map[string]any)
	require.True(t, ok, "response must have an `error` object; got %v", got)
	require.NotEmpty(t, errObj["code"])
	require.NotEmpty(t, errObj["message"])
}
