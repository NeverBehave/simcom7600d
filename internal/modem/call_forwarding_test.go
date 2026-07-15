package modem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCallForwardingReadsVoiceRules(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact("AT+CCFC=0,2", "+CCFC: 0,255", "OK")
	sim.OnExact("AT+CCFC=1,2",
		`+CCFC: 1,2,"+12025550000",145`,
		`+CCFC: 1,1,"+12025550111",145`, "OK")
	sim.OnExact("AT+CCFC=2,2", `+CCFC: 1,1,"+12025550222",145,,,25`, "OK")
	sim.OnExact("AT+CCFC=3,2", "+CCFC: 0,1", "OK")

	rules, err := f.GetCallForwarding(context.Background())
	require.NoError(t, err)
	require.Equal(t, []CallForwardingRule{
		{Reason: "unconditional", Available: true},
		{Reason: "busy", Enabled: true, Number: "+12025550111", Available: true},
		{Reason: "no_reply", Enabled: true, Number: "+12025550222", TimeoutSeconds: 25, Available: true},
		{Reason: "unreachable", Available: true},
	}, rules)
}

func TestSetCallForwardingRegistersAndConfirms(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact(`AT+CCFC=2,3,"+12025550123",145,1,,,20`, "OK")
	sim.OnExact("AT+CCFC=2,2", `+CCFC: 1,1,"+12025550123",145,,,20`, "OK")

	got, err := f.SetCallForwarding(context.Background(), CallForwardingRule{
		Reason: "no_reply", Enabled: true, Number: "+12025550123", TimeoutSeconds: 20,
	})
	require.NoError(t, err)
	require.Equal(t, CallForwardingRule{
		Reason: "no_reply", Enabled: true, Number: "+12025550123", TimeoutSeconds: 20, Available: true,
	}, got)
}

func TestGetCallForwardingReportsCarrierFailureWithoutGuessing(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact("AT+CCFC=0,2", "+CME ERROR: network rejected request")

	rules, err := f.GetCallForwarding(context.Background())
	require.NoError(t, err)
	require.Len(t, rules, 4)
	for _, rule := range rules {
		require.False(t, rule.Available)
		require.Contains(t, rule.Error, "network rejected request")
	}
}

func TestSetCallForwardingUsesCarrierCompatibleDisableForm(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact("AT+CCFC=0,0", "OK")
	sim.OnExact("AT+CCFC=0,2", "+CCFC: 0,1", "OK")

	got, err := f.SetCallForwarding(context.Background(), CallForwardingRule{Reason: "unconditional"})
	require.NoError(t, err)
	require.False(t, got.Enabled)
}

func TestSetCallForwardingAcceptsConfirmedStateAfterCarrierTimeout(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact("AT+CCFC=0,0", "+CME ERROR: network timeout")
	sim.OnExact("AT+CCFC=0,2", "+CCFC: 0,255", "OK")

	got, err := f.SetCallForwarding(context.Background(), CallForwardingRule{Reason: "unconditional"})
	require.NoError(t, err)
	require.True(t, got.Available)
	require.False(t, got.Enabled)
}

func TestSetCallForwardingPreservesCarrierErrorWhenStateDoesNotMatch(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact("AT+CCFC=0,0", "+CME ERROR: network timeout")
	sim.OnExact("AT+CCFC=0,2", `+CCFC: 1,1,"+12025550123",145`, "OK")

	_, err := f.SetCallForwarding(context.Background(), CallForwardingRule{Reason: "unconditional"})
	require.ErrorContains(t, err, "network timeout")
}

func TestSetCallForwardingCanApplyWhenCarrierStatusIsUnavailable(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact("AT+CCFC=3,2", "+CME ERROR: network timeout")
	sim.OnExact(`AT+CCFC=3,3,"+12025550123",145,1`, "OK")

	got, err := f.SetCallForwarding(context.Background(), CallForwardingRule{
		Reason: "unreachable", Enabled: true, Number: "+12025550123",
	})
	require.NoError(t, err)
	require.True(t, got.Enabled)
	require.False(t, got.Available)
	require.Contains(t, got.Error, "update accepted; confirmation failed")
}

func TestSetCallForwardingValidatesTimeout(t *testing.T) {
	f, _ := newTestFacade(t)
	_, err := f.SetCallForwarding(context.Background(), CallForwardingRule{
		Reason: "no_reply", Enabled: true, Number: "+12025550123", TimeoutSeconds: 17,
	})
	require.ErrorIs(t, err, ErrInvalidCallForwardingTimeout)
}

func TestParseCCFCLine(t *testing.T) {
	record, ok := parseCCFCLine(`+CCFC: 1,1,"+12025550123",145,,,30`)
	require.True(t, ok)
	require.True(t, record.enabled)
	require.Equal(t, 1, record.serviceClass)
	require.Equal(t, "+12025550123", record.number)
	require.Equal(t, 30, record.timeoutSeconds)
}
