//go:build ignore

// Helper that boots a test server using internal/api with a stub modem,
// prints "URL=<url>\nTOKEN=<token>\n" to stdout, then waits for SIGTERM/SIGINT
// before shutting down.
package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sim7600d/internal/api"
	"sim7600d/internal/modem"
)

type stub struct{}

func (stub) Status(context.Context, bool) (modem.ModemStatus, error) {
	return modem.ModemStatus{
		Model:     "STUB",
		UpdatedAt: time.Now().UTC(),
		SIM:       modem.SIMStatus{State: "ready"},
		Network:   modem.NetworkStatus{Tech: "none"},
	}, nil
}
func (stub) SendSMS(_ context.Context, _ string, _ string, _ modem.SendOpts) (modem.Outbound, error) {
	return modem.Outbound{ID: "01HSTUB", State: "submitted", Encoding: "gsm7", CreatedAt: time.Now().UTC()}, nil
}
func (stub) GetOutbound(context.Context, string) (modem.Outbound, error) {
	return modem.Outbound{}, nil
}
func (stub) GetInbound(context.Context, string) (modem.Inbound, error) { return modem.Inbound{}, nil }
func (stub) ListInbound(context.Context, string, int) ([]modem.Inbound, error) {
	return []modem.Inbound{}, nil
}
func (stub) ListOutbound(context.Context, string, int) ([]modem.Outbound, error) {
	return []modem.Outbound{}, nil
}
func (stub) DeleteSMS(context.Context, string) error { return nil }
func (stub) Dial(_ context.Context, _ string, _ string) (modem.Call, error) {
	return modem.Call{ID: "01CSTUB", Direction: "out", RemoteAddr: "+15551234567", State: "dialing", StartedAt: time.Now().UTC()}, nil
}
func (stub) Hangup(context.Context, string) error                         { return nil }
func (stub) Answer(context.Context, string) error                         { return nil }
func (stub) Reject(context.Context, string) error                         { return nil }
func (stub) Hold(context.Context, string) error                           { return nil }
func (stub) Resume(context.Context, string) error                         { return nil }
func (stub) MergeCalls(context.Context, string) error                     { return nil }
func (stub) SendDTMF(context.Context, string, string, int) error          { return nil }
func (stub) StartCallAudio(context.Context, string) error                 { return nil }
func (stub) StopCallAudio(context.Context, string) error                  { return nil }
func (stub) GetCall(context.Context, string) (modem.Call, error)          { return modem.Call{}, nil }
func (stub) ListCalls(context.Context, string, int) ([]modem.Call, error) { return nil, nil }
func (stub) GetCallForwarding(context.Context) ([]modem.CallForwardingRule, error) {
	return []modem.CallForwardingRule{
		{Reason: "unconditional", Available: true},
		{Reason: "busy", Enabled: true, Number: "+12025550123", Available: true},
		{Reason: "no_reply", TimeoutSeconds: 20, Available: true},
		{Reason: "unreachable", Available: true},
	}, nil
}
func (stub) SetCallForwarding(_ context.Context, rule modem.CallForwardingRule) (modem.CallForwardingRule, error) {
	return rule, nil
}
func (stub) OnIncomingCall() <-chan modem.IncomingCall { return nil }
func (stub) OnInboundSMS() <-chan modem.SMSArrived     { return nil }
func (stub) OnLifecycle() <-chan modem.LifecycleEvent  { return nil }

// Compile-time interface check.
var _ modem.Modem = stub{}

func main() {
	srv := httptest.NewServer(api.NewRouter(api.Config{
		AuthToken: "smoke-test-token",
		Modem:     stub{},
	}))
	defer srv.Close()
	fmt.Printf("URL=%s\nTOKEN=smoke-test-token\n", srv.URL)
	// Flush stdout so the launcher can read URL/TOKEN immediately.
	os.Stdout.Sync() //nolint:errcheck

	// Block until SIGTERM or SIGINT (sent by run.sh when the JS test finishes).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
}
