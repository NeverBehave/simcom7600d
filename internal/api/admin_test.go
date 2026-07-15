package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/store"
)

func TestEvents_RequiresAuth(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	defer st.Close()

	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret", Store: st})
	resp, err := http.Get(srv.URL + "/v1/events")
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestEvents_Empty(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	defer st.Close()

	srv := newTestServerWith(t, &fakeModem{}, Config{AuthToken: "secret", Store: st})
	req, _ := http.NewRequest("GET", srv.URL+"/v1/events", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Empty(t, body["items"])
}

func TestAdminCapabilities_AreReadOnlyAndReflectFlags(t *testing.T) {
	srv := newTestServerWith(t, &fakeModem{}, Config{
		AuthToken: "secret", AllowATPassthrough: true, AllowModemReset: false,
	})
	req, _ := http.NewRequest("GET", srv.URL+"/v1/admin/capabilities", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		ATPassthrough bool `json:"at_passthrough"`
		ModemReset    bool `json:"modem_reset"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.True(t, body.ATPassthrough)
	require.False(t, body.ModemReset)
}
