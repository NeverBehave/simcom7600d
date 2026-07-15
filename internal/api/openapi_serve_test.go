package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAPI_ServedUnauthenticated(t *testing.T) {
	srv := newTestServer(t, &fakeModem{})

	resp, err := http.Get(srv.URL + "/openapi.json")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var spec map[string]any
	require.NoError(t, json.Unmarshal(body, &spec))
	require.NotEmpty(t, spec["openapi"], "spec should have an openapi version field")
	require.NotEmpty(t, spec["paths"])
}
