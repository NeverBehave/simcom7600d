package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

func TestReset_BumpsEpochAndDrains(t *testing.T) {
	r, sim, _, st := setupReconciler(t)
	ctx := context.Background()

	// Pre-populate an open call.
	require.NoError(t, st.InsertCall(ctx, store.Call{
		ID: store.NewULID(), Direction: "out", RemoteAddr: "+1",
		State: "active", StartedAt: time.Now().UTC(),
	}))
	sim.OnExact("AT+CLCC", "OK")
	sim.OnExact("AT+CMGL=4", "OK")
	sim.OnExact("AT+CSQ", "OK")
	sim.OnExact("AT+COPS?", "OK")
	sim.OnExact("AT+CREG?", "OK")
	sim.OnExact("AT+CPIN?", "OK")
	sim.OnExact("AT+CBC", "OK")

	require.NoError(t, r.HandleReset(ctx, "RDY", urc.Event{}))
	open, err := st.ListOpenCalls(ctx)
	require.NoError(t, err)
	require.Empty(t, open)

	v, ok, err := st.GetKV(ctx, "epoch")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEmpty(t, v)
}
