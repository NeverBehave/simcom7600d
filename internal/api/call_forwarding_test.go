package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/modem"
)

func TestCallForwardingGet(t *testing.T) {
	fm := &fakeModem{forwarding: []modem.CallForwardingRule{
		{Reason: "unconditional", Enabled: false, Available: true},
		{Reason: "no_reply", Enabled: true, Number: "+12025550123", TimeoutSeconds: 20, Available: true},
	}}
	srv := newTestServer(t, fm)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/call-forwarding", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Items []callForwardingRuleJSON `json:"items"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Items, 2)
	require.True(t, body.Items[1].Enabled)
	require.True(t, body.Items[1].Available)
	require.Equal(t, "+12025550123", body.Items[1].Number)
}

func TestCallForwardingUpdateNormalizesNumber(t *testing.T) {
	fm := &fakeModem{}
	srv := newTestServer(t, fm)
	body := bytes.NewBufferString(`{"enabled":true,"number":"(202) 555-0123","timeout_seconds":25}`)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/call-forwarding/no_reply", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, modem.CallForwardingRule{
		Reason: "no_reply", Enabled: true, Number: "+12025550123", TimeoutSeconds: 25,
	}, fm.forwardingSet)
}

func TestCallForwardingUpdateRejectsMissingNumber(t *testing.T) {
	srv := newTestServer(t, &fakeModem{})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/call-forwarding/busy", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCallForwardingUpdateMapsModemError(t *testing.T) {
	srv := newTestServer(t, &fakeModem{forwardingErr: errors.New("carrier rejected request")})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/call-forwarding/busy", bytes.NewBufferString(`{"enabled":false}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
}
