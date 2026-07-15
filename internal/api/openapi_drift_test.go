package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAPI_NoDrift(t *testing.T) {
	srv := NewServer(Config{
		AuthToken:          "drift-test",
		AllowATPassthrough: true,
		AllowModemReset:    true,
	})

	live := bytes.TrimSpace(srv.Spec)

	committed, err := os.ReadFile(filepath.Join("openapi.json"))
	require.NoError(t, err, "internal/api/openapi.json must exist; run `make openapi`")
	committed = bytes.TrimSpace(committed)

	if bytes.Equal(canonicalizeJSON(t, live), canonicalizeJSON(t, committed)) {
		return
	}

	t.Fatalf("openapi.json is stale; run `make openapi`.\n--- committed (first 400 bytes) ---\n%s\n--- live (first 400 bytes) ---\n%s",
		head(committed, 400), head(live, 400))
}

// canonicalizeJSON re-marshals JSON with 2-space indent so comparison is
// robust to whitespace differences. Map keys are emitted by Go's json package
// in sorted order automatically, so this is a pass-through that walks into
// nested values.
func canonicalizeJSON(t *testing.T, b []byte) []byte {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal(b, &v))
	out, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	return out
}

func head(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
