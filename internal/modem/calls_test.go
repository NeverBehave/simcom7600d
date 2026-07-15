package modem

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/store"
)

func TestDial_AcceptedThenHangup(t *testing.T) {
	f, sim := newTestFacade(t)
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))

	sim.OnExact("ATD+15551234567;", "OK")
	sim.OnExact("AT+CLCC", `+CLCC: 1,0,3,0,0,"+15551234567",145`, "OK")
	call, err := f.Dial(context.Background(), "+15551234567", "")
	require.NoError(t, err)
	require.Equal(t, "out", call.Direction)
	require.Equal(t, "dialing", call.State)

	sim.OnExact("AT+CHUP", "OK")
	require.NoError(t, f.Hangup(context.Background(), call.ID))

	got, err := f.GetCall(context.Background(), call.ID)
	require.NoError(t, err)
	require.Equal(t, "ended", got.State)
	require.Equal(t, "hangup", got.EndReason)
	_ = time.Time{}
}

func TestReject_UsesUnconditionalVoiceHangup(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "in", RemoteAddr: "+15551234567",
		State: "ringing", StartedAt: time.Now().UTC(),
	}))

	sim.OnExact("AT+CHUP", "OK")
	require.NoError(t, f.Reject(ctx, id))

	got, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "rejected", got.State)
}

func TestDTMF_Validation(t *testing.T) {
	f, sim := newTestFacade(t)
	for _, c := range []string{"ATE0", "AT+CMEE=2", "AT+CMGF=0", "AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1", `AT+CSCS="UCS2"`} {
		sim.OnExact(c, "OK")
	}
	require.NoError(t, f.Boot(context.Background()))

	require.ErrorIs(t, f.SendDTMF(context.Background(), "x", "0123abc!", 100), ErrInvalidDTMF)
}

func TestHoldAndResume_UseNetworkCallControlAndUpdateState(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "active", StartedAt: time.Now().UTC(), AnsweredAt: time.Now().UTC(),
	}))

	sim.OnExact("AT+CHLD=2", "OK")
	require.NoError(t, f.Hold(ctx, id))
	held, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "held", held.State)

	sim.OnExact("AT+CHLD=2", "OK")
	require.NoError(t, f.Resume(ctx, id))
	active, err := f.GetCall(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "active", active.State)
}

func TestHoldAndResume_ValidateCurrentState(t *testing.T) {
	f, _ := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "dialing", StartedAt: time.Now().UTC(),
	}))

	require.ErrorContains(t, f.Hold(ctx, id), "state=dialing")
	require.ErrorContains(t, f.Resume(ctx, id), "state=dialing")
}

func TestHangup_TargetsSelectedCallWhenAnotherCallIsHeld(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	heldID := store.NewULID()
	activeID := store.NewULID()
	now := time.Now().UTC()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: heldID, Direction: "out", RemoteAddr: "+15551230001",
		State: "held", StartedAt: now.Add(-time.Minute), AnsweredAt: now.Add(-55 * time.Second),
	}))
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: activeID, Direction: "out", RemoteAddr: "+15551230002",
		State: "active", StartedAt: now.Add(-30 * time.Second), AnsweredAt: now.Add(-25 * time.Second),
	}))

	sim.OnExact("AT+CLCC",
		`+CLCC: 1,0,1,0,0,"+15551230001",145`,
		`+CLCC: 2,0,0,0,0,"+15551230002",145`, "OK")
	sim.OnExact("AT+CHLD=12", "OK")
	require.NoError(t, f.Hangup(ctx, activeID))

	held, err := f.GetCall(ctx, heldID)
	require.NoError(t, err)
	require.Equal(t, "held", held.State)
	ended, err := f.GetCall(ctx, activeID)
	require.NoError(t, err)
	require.Equal(t, "ended", ended.State)
}

func TestMergeCalls_AddsHeldCallToActiveCall(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, call := range []store.Call{
		{ID: store.NewULID(), Direction: "out", RemoteAddr: "+15551230001", State: "held", StartedAt: now},
		{ID: store.NewULID(), Direction: "out", RemoteAddr: "+15551230002", State: "active", StartedAt: now},
	} {
		require.NoError(t, f.st.InsertCall(ctx, call))
	}
	open, err := f.st.ListOpenCalls(ctx)
	require.NoError(t, err)
	sim.OnExact("AT+CHLD=3", "OK")
	require.NoError(t, f.MergeCalls(ctx, open[0].ID))

	merged, err := f.st.ListOpenCalls(ctx)
	require.NoError(t, err)
	require.Len(t, merged, 2)
	for _, call := range merged {
		require.Equal(t, "active", call.State)
	}
}

func TestDTMF_SendsExplicitModemDuration(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "active", StartedAt: time.Now().UTC(), AnsweredAt: time.Now().UTC(),
	}))

	sim.OnExact("AT+VTS=5,2", "OK")
	require.NoError(t, f.SendDTMF(ctx, id, "5", 200))
}

func TestDTMF_DefaultsToAudibleDurationAndRejectsNonOK(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "active", StartedAt: time.Now().UTC(), AnsweredAt: time.Now().UTC(),
	}))

	sim.OnExact("AT+VTS=#,2", "ERROR")
	require.ErrorContains(t, f.SendDTMF(ctx, id, "#", 0), "dtmf")
}

func TestListCalls_DoesNotDeadlockWithSingleDBConnection(t *testing.T) {
	f, _ := newTestFacade(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: store.NewULID(), Direction: "out", RemoteAddr: "+15551234567",
		State: "ended", StartedAt: time.Now().UTC(),
	}))

	calls, err := f.ListCalls(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, calls, 1)
}

func TestDial_IdempotencyReturnsExistingCall(t *testing.T) {
	f, sim := newTestFacade(t)
	sim.OnExact("ATD+15551234567;", "OK")

	first, err := f.Dial(context.Background(), "+15551234567", "same-request")
	require.NoError(t, err)
	second, err := f.Dial(context.Background(), "+15551234567", "same-request")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
}

func TestCallAudio_RequiresAttachableCallAndControlsUSBPCM(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "ended", StartedAt: time.Now().UTC(),
	}))

	require.ErrorIs(t, f.StartCallAudio(ctx, id), ErrCallAudioUnavailable)
	require.NoError(t, f.st.SetCallState(ctx, id, store.CallUpdate{
		State: "active", AnsweredAt: time.Now().UTC(),
	}))
	sim.OnExact("AT+CPCMFRM=1", "OK")
	sim.OnExact("AT+CPCMBANDWIDTH=0,1", "OK")
	sim.OnExact("AT+CPCMREG=1", "OK")
	sim.OnExact("AT+CPCMREG=0,1", "OK")
	require.NoError(t, f.StartCallAudio(ctx, id))
	require.NoError(t, f.StopCallAudio(ctx, id))
}

func TestCallAudio_RejectsNonOKFinal(t *testing.T) {
	f, sim := newTestFacade(t)
	ctx := context.Background()
	id := store.NewULID()
	require.NoError(t, f.st.InsertCall(ctx, store.Call{
		ID: id, Direction: "out", RemoteAddr: "+15551234567",
		State: "active", StartedAt: time.Now().UTC(), AnsweredAt: time.Now().UTC(),
	}))
	sim.OnExact("AT+CPCMBANDWIDTH=0,1", "ERROR")
	require.ErrorContains(t, f.StartCallAudio(ctx, id), "start call audio")
}
