package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server struct {
		Bind          string `toml:"bind"`
		AuthTokenFile string `toml:"auth_token_file"`
	} `toml:"server"`
	Modem struct {
		TTY  string `toml:"tty"`
		Baud int    `toml:"baud"`
	} `toml:"modem"`
	Audio struct {
		Device string `toml:"device"`
	} `toml:"audio"`
	Storage struct {
		Path string `toml:"path"`
	} `toml:"storage"`
	Retention RetentionConfig `toml:"retention"`
	Admin     struct {
		ATPassthrough   bool `toml:"at_passthrough"`
		AllowModemReset bool `toml:"allow_modem_reset"`
	} `toml:"admin"`
	Log struct {
		Level   string `toml:"level"`
		ATTrace bool   `toml:"at_trace"`
	} `toml:"log"`

	// Resolved at runtime, not in TOML.
	authToken string
}

type RetentionConfig struct {
	EventsDays int `toml:"events_days"`
	SMSDays    int `toml:"sms_days"`
	CallsDays  int `toml:"calls_days"`
	IdemHours  int `toml:"idem_hours"`
}

func defaultConfig() Config {
	c := Config{}
	c.Server.Bind = "127.0.0.1:8080"
	c.Modem.TTY = "/dev/ttyUSB3"
	c.Modem.Baud = 115200
	c.Storage.Path = "sim7600d.db"
	c.Retention.IdemHours = 24
	c.Log.Level = "info"
	return c
}

// Parse loads a config from CLI flags + optional TOML file + env. Flags take
// the highest precedence; env (SIM7600D_AUTH_TOKEN) resolves the auth token
// when no token file is configured.
func Parse(args []string) (Config, error) {
	fs := flag.NewFlagSet("sim7600d", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "Path to TOML config")
	bind := fs.String("bind", "", "HTTP bind address")
	tty := fs.String("tty", "", "TTY path (or PTY for dev)")
	dbPath := fs.String("db", "", "SQLite path")
	atTrace := fs.Bool("at-trace", false, "Log every AT exchange at info")
	level := fs.String("log-level", "", "debug|info|warn|error")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	c := defaultConfig()
	if *cfgPath != "" {
		if _, err := toml.DecodeFile(*cfgPath, &c); err != nil {
			return Config{}, fmt.Errorf("config %s: %w", *cfgPath, err)
		}
	}
	if *bind != "" {
		c.Server.Bind = *bind
	}
	if *tty != "" {
		c.Modem.TTY = *tty
	}
	if *dbPath != "" {
		c.Storage.Path = *dbPath
	}
	if *atTrace {
		c.Log.ATTrace = true
	}
	if *level != "" {
		c.Log.Level = *level
	}
	tok, err := resolveAuthToken(c)
	if err != nil {
		return Config{}, err
	}
	c.authToken = tok
	return c, nil
}

func resolveAuthToken(c Config) (string, error) {
	if v := strings.TrimSpace(os.Getenv("SIM7600D_AUTH_TOKEN")); v != "" {
		return validateAuthToken(v)
	}
	if c.Server.AuthTokenFile != "" {
		return readAuthToken(c.Server.AuthTokenFile)
	}
	// Generate next to the DB if it doesn't already exist.
	dir := filepath.Dir(c.Storage.Path)
	if dir == "" {
		dir = "."
	}
	tokenPath := filepath.Join(dir, "auth_token")
	if b, err := os.ReadFile(tokenPath); err == nil {
		if err := os.Chmod(tokenPath, 0o600); err != nil {
			return "", fmt.Errorf("secure auth token file: %w", err)
		}
		return validateAuthToken(strings.TrimSpace(string(b)))
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	tok, err := generateToken()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(tokenPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return readAuthToken(tokenPath)
	}
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(tok); err != nil {
		_ = f.Close()
		_ = os.Remove(tokenPath)
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tokenPath)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tokenPath)
		return "", err
	}
	fmt.Fprintf(os.Stderr, "sim7600d: generated auth token at %s\n", tokenPath)
	return tok, nil
}

func readAuthToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return validateAuthToken(strings.TrimSpace(string(b)))
}

func validateAuthToken(token string) (string, error) {
	if len(token) < 32 {
		return "", errors.New("auth token must contain at least 32 characters")
	}
	if len(token) > 512 {
		return "", errors.New("auth token must not exceed 512 characters")
	}
	for _, c := range token {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || strings.ContainsRune("-._~", c)) {
			return "", errors.New("auth token may only contain letters, digits, '-', '.', '_', and '~'")
		}
	}
	return token, nil
}

func generateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate auth token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
