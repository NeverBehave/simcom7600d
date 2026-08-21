package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersionDefault(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must default to a non-empty string")
	}
}

func TestResolveAuthTokenCreatesPrivateStableToken(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.Storage.Path = filepath.Join(dir, "sim7600d.db")

	token, err := resolveAuthToken(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 48 {
		t.Fatalf("generated token length = %d, want 48", len(token))
	}
	again, err := resolveAuthToken(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if token != again {
		t.Fatal("generated token was not stable")
	}
	info, err := os.Stat(filepath.Join(dir, "auth_token"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("auth token mode = %o, want 600", got)
	}
}

func TestResolveAuthTokenRejectsWeakConfiguredToken(t *testing.T) {
	t.Setenv("SIM7600D_AUTH_TOKEN", "too-short")
	if _, err := resolveAuthToken(defaultConfig()); err == nil {
		t.Fatal("expected weak auth token to be rejected")
	}
}

func TestVoicemailDefaultsToOptIn(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Voicemail.Enabled {
		t.Fatal("voicemail mailbox access must be disabled until explicitly configured")
	}
	if cfg.voicemailInterval != 5*time.Minute {
		t.Fatalf("voicemail interval = %s, want 5m", cfg.voicemailInterval)
	}
}

func TestHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if srv.ReadHeaderTimeout != 10*time.Second || srv.IdleTimeout != 2*time.Minute {
		t.Fatalf("unexpected HTTP timeouts: header=%s read=%s idle=%s", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.IdleTimeout)
	}
	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 {
		t.Fatalf("long-lived streams require no socket read/write timeout; got read=%s write=%s", srv.ReadTimeout, srv.WriteTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Fatalf("MaxHeaderBytes = %d", srv.MaxHeaderBytes)
	}
}
