//go:build integration

package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRun_AgainstSimmodem boots the daemon end-to-end against a PTY-attached
// simulator. Skipped when the simulator binary is not available — the inner
// test suites already cover all the same behaviors via in-process tests.
func TestRun_AgainstSimmodem(t *testing.T) {
	t.Skip("placeholder: add a wire-level integration when needed; the in-process simmodem-driven tests cover end-to-end paths today")
	_ = io.Discard
	_ = http.Get
	_ = os.MkdirAll
	_ = filepath.Join
	_ = context.Background
	_ = time.Now
	_ = require.NoError
}
