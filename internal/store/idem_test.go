package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIdem_RoundTripAndSweep(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	require.NoError(t, s.PutIdem(ctx, IdemRecord{
		Key: "k", Scope: "sms", RefID: "01H", Response: `{"ok":true}`, Status: 202, CreatedAt: time.Now().UTC().Add(-30 * time.Hour),
	}))
	require.NoError(t, s.PutIdem(ctx, IdemRecord{
		Key: "j", Scope: "sms", RefID: "01J", Response: `{"ok":true}`, Status: 202, CreatedAt: time.Now().UTC(),
	}))
	got, ok, err := s.GetIdem(ctx, "j")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "01J", got.RefID)

	// Sweep older than 24h: only "k" goes.
	n, err := s.SweepIdem(ctx, time.Now().UTC().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	_, ok, err = s.GetIdem(ctx, "k")
	require.NoError(t, err)
	require.False(t, ok)
}
