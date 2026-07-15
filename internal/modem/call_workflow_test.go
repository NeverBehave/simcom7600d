package modem

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/store"
)

func TestInboundCall_RingingThenAnswer(t *testing.T) {
	f, sim := newTestFacade(t)
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))

	// CLCC poller will run; respond with no-calls until we say otherwise.
	sim.OnExact("AT+CLCC", "OK")

	sim.EmitURC(`+CLIP: "+15551234567",145,,,"",0`)
	select {
	case ic := <-f.OnIncomingCall():
		require.Equal(t, "+15551234567", ic.From)
		require.NotEmpty(t, ic.ID)
	case <-time.After(time.Second):
		t.Fatal("expected incoming-call event")
	}
}

func TestInboundCall_CRINGDiscoversCallWithoutCLIP(t *testing.T) {
	f, sim := newTestFacade(t)
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))
	sim.OnExact("AT+CLCC", `+CLCC: 1,1,4,0,0,"+15559876543",145`, "OK")

	sim.EmitURC(`+CRING: VOICE`)
	select {
	case incoming := <-f.OnIncomingCall():
		require.Equal(t, "+15559876543", incoming.From)
		got, err := f.GetCall(context.Background(), incoming.ID)
		require.NoError(t, err)
		require.Equal(t, "ringing", got.State)
		require.Equal(t, "+15559876543", got.RemoteAddr)
	case <-time.After(2 * time.Second):
		t.Fatal("expected +CRING to discover an incoming call through CLCC")
	}
}

func TestOpenInbound_FillsDelayedCallerID(t *testing.T) {
	f, _ := newTestFacade(t)
	ctx := context.Background()
	id, err := f.openInbound(ctx, "")
	require.NoError(t, err)
	sameID, err := f.openInbound(ctx, "+15559876543")
	require.NoError(t, err)
	require.Equal(t, id, sameID)
	got, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "+15559876543", got.RemoteAddr)
}

func TestPollCLCC_UpdatesOutgoingCallToAlerting(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "dialing", StartedAt: time.Now().UTC().Add(-5 * time.Second),
	}))

	sim.OnExact("AT+CLCC", `+CLCC: 1,0,3,0,0,"+15551234567",145`, "OK")
	f.pollCLCC(ctx)
	alerting, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "alerting", alerting.State)
}

func TestPollCLCC_UpdatesOutgoingCallToActive(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "alerting", StartedAt: time.Now().UTC().Add(-5 * time.Second),
	}))

	sim.OnExact("AT+CLCC", `+CLCC: 1,0,0,0,0,"+15551234567",145`, "OK")
	f.pollCLCC(ctx)
	active, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "active", active.State)
	require.False(t, active.AnsweredAt.IsZero())
}

func TestPollCLCC_GivesNewDialATimeToAppear(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "dialing", StartedAt: time.Now().UTC(),
	}))
	sim.OnExact("AT+CLCC", "OK")
	f.pollCLCC(ctx)

	got, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "dialing", got.State)
}

func TestPollCLCC_IgnoresNonOKSnapshotDuringDisconnect(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "held", StartedAt: time.Now().UTC().Add(-time.Minute),
	}))
	sim.OnExact("AT+CLCC", "NO CARRIER")
	f.pollCLCC(ctx)

	got, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "held", got.State)
}

func TestPollCLCC_RequiresRepeatedAuthoritativeAbsence(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "held", StartedAt: time.Now().UTC().Add(-time.Minute),
	}))
	for range clccMissingPollsBeforeEnd {
		sim.OnExact("AT+CLCC", "OK")
	}
	for i := 1; i < clccMissingPollsBeforeEnd; i++ {
		f.pollCLCC(ctx)
		got, err := f.GetCall(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "held", got.State)
	}
	f.pollCLCC(ctx)
	got, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "ended", got.State)
}
