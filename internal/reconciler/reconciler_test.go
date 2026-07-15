package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/atexec"
	"sim7600d/internal/modem"
	"sim7600d/internal/simmodem"
	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

func setupReconciler(t *testing.T) (*Reconciler, *simmodem.Sim, *modem.Facade, *store.Store) {
	t.Helper()
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ex := atexec.New(peer, bus)
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	f, err := modem.New(ex, bus, st)
	require.NoError(t, err)
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))
	r := New(ex, st, f)
	t.Cleanup(func() { f.Close(); ex.Close(); bus.Close(); st.Close() })
	return r, sim, f, st
}

func TestReconciler_BootSweepsStuckActive(t *testing.T) {
	r, sim, _, st := setupReconciler(t)
	ctx := context.Background()

	// Insert an "active" call >1h old.
	old := time.Now().UTC().Add(-2 * time.Hour)
	require.NoError(t, st.InsertCall(ctx, store.Call{
		ID: store.NewULID(), Direction: "out", RemoteAddr: "+1",
		State: "active", StartedAt: old,
	}))
	sim.OnExact("AT+CLCC", "OK")
	sim.OnExact("AT+CMGL=4", "OK")
	sim.OnExact("AT+CSQ", "+CSQ: 20,99", "OK")
	sim.OnExact("AT+COPS?", `+COPS: 0,0,"X",7`, "OK")
	sim.OnExact("AT+CREG?", `+CREG: 0,1`, "OK")
	sim.OnExact("AT+CPIN?", `+CPIN: READY`, "OK")
	sim.OnExact("AT+CBC", `+CBC: 3.9V`, "OK")

	require.NoError(t, r.Boot(ctx))

	open, err := st.ListOpenCalls(ctx)
	require.NoError(t, err)
	require.Empty(t, open, "stuck active call should be reconciled to ended")
}

func TestReconciler_AdoptsCLCCNotInDB(t *testing.T) {
	r, sim, _, st := setupReconciler(t)
	ctx := context.Background()

	sim.OnExact("AT+CLCC", `+CLCC: 1,1,4,0,0,"+15551234567",145`, "OK") // active inbound, in DB? no.
	sim.OnExact("AT+CMGL=4", "OK")
	sim.OnExact("AT+CSQ", "OK")
	sim.OnExact("AT+COPS?", "OK")
	sim.OnExact("AT+CREG?", "OK")
	sim.OnExact("AT+CPIN?", "OK")
	sim.OnExact("AT+CBC", "OK")

	require.NoError(t, r.Reconcile(ctx))
	open, err := st.ListOpenCalls(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	require.Equal(t, "+15551234567", open[0].RemoteAddr)
}

func TestReconciler_DoesNotReadoptCallLingeringAfterHangup(t *testing.T) {
	r, sim, _, st := setupReconciler(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, st.InsertCall(ctx, store.Call{
		ID: store.NewULID(), Direction: "out", RemoteAddr: "+15551234567",
		State: "ended", EndReason: "hangup", StartedAt: now.Add(-time.Minute), EndedAt: now,
	}))
	sim.OnExact("AT+CLCC", `+CLCC: 3,0,0,0,0,"+15551234567",145`, "OK")
	sim.OnExact("AT+CMGL=4", "OK")
	sim.OnExact("AT+CSQ", "OK")
	sim.OnExact("AT+COPS?", "OK")
	sim.OnExact("AT+CREG?", "OK")
	sim.OnExact("AT+CPIN?", "OK")
	sim.OnExact("AT+CBC", "OK")

	require.NoError(t, r.Reconcile(ctx))
	open, err := st.ListOpenCalls(ctx)
	require.NoError(t, err)
	require.Empty(t, open)
}

func TestReconciler_ImportsSMSAlreadyInModemStorage(t *testing.T) {
	r, sim, _, st := setupReconciler(t)
	ctx := context.Background()

	sim.OnExact("AT+CLCC", "OK")
	sim.OnExact("AT+CMGL=4", "+CMGL: 5,0,,17", modemDeliverFixturePDU(), "OK")
	sim.OnExact("AT+CMGR=5", "+CMGR: 0,,17", modemDeliverFixturePDU(), "OK")
	sim.OnExact("AT+CMGD=5", "OK")
	sim.OnExact("AT+CSQ", "OK")
	sim.OnExact("AT+COPS?", "OK")
	sim.OnExact("AT+CREG?", "OK")
	sim.OnExact("AT+CPIN?", "OK")
	sim.OnExact("AT+CBC", "OK")

	require.NoError(t, r.Reconcile(ctx))
	items, err := st.ListInbound(ctx, store.InboundFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "Hello", items[0].Body)
}

func modemDeliverFixturePDU() string {
	return "06912143658709" + "000B915155214365F700004210510103000005C8329BFD06"
}
