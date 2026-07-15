package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalls_LifecycleAndOpen(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	now := time.Now().UTC()
	c := Call{
		ID: NewULID(), Direction: "out", RemoteAddr: "+15551234567",
		State: "dialing", StartedAt: now,
	}
	require.NoError(t, s.InsertCall(ctx, c))
	open, err := s.ListOpenCalls(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)

	require.NoError(t, s.SetCallState(ctx, c.ID, CallUpdate{
		State: "active", AnsweredAt: now.Add(time.Second),
	}))
	require.NoError(t, s.SetCallState(ctx, c.ID, CallUpdate{
		State: "ended", EndReason: "normal", EndedAt: now.Add(10 * time.Second),
		DurationMS: 9000,
	}))
	open, err = s.ListOpenCalls(ctx)
	require.NoError(t, err)
	require.Empty(t, open)

	got, err := s.GetCall(ctx, c.ID)
	require.NoError(t, err)
	require.Equal(t, "ended", got.State)
	require.Equal(t, "normal", got.EndReason)
	require.Equal(t, 9000, got.DurationMS)

	recent, err := s.HasRecentlyEndedCall(ctx, "out", "15551234567", now)
	require.NoError(t, err)
	require.True(t, recent)
	stale, err := s.HasRecentlyEndedCall(ctx, "out", "+15551234567", now.Add(11*time.Second))
	require.NoError(t, err)
	require.False(t, stale)
}
