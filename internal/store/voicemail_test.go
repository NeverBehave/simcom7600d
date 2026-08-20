package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVoicemailLifecycleAndAudio(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	received := time.Now().UTC().Add(-time.Minute)

	created, err := s.UpsertVoicemail(ctx, Voicemail{
		SourceID: "carrier-42", FromAddr: "+15551234567", ReceivedAt: received,
		DurationMS: 42000, ContentType: "audio/wav", Audio: []byte("RIFF-audio"),
		Transcript: "Call me back", SourceSMSID: "sms-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	require.Equal(t, []byte("RIFF-audio"), created.Audio)
	require.True(t, created.ReadAt.IsZero())

	items, err := s.ListVoicemails(ctx, VoicemailFilter{UnreadOnly: true})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].Audio, "list queries must not load audio blobs")

	read, err := s.SetVoicemailRead(ctx, created.ID, true)
	require.NoError(t, err)
	require.False(t, read.ReadAt.IsZero())
	unread, err := s.ListVoicemails(ctx, VoicemailFilter{UnreadOnly: true})
	require.NoError(t, err)
	require.Empty(t, unread)

	// A carrier resync updates the payload while preserving local read state.
	updated, err := s.UpsertVoicemail(ctx, Voicemail{
		SourceID: "carrier-42", FromAddr: "+15557654321", ReceivedAt: received,
		DurationMS: 43000, ContentType: "audio/wav", Audio: []byte("new-audio"),
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, updated.ID)
	require.False(t, updated.ReadAt.IsZero())
	require.Equal(t, []byte("new-audio"), updated.Audio)

	require.NoError(t, s.DeleteVoicemail(ctx, created.ID))
	_, err = s.GetVoicemail(ctx, created.ID, false)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = s.UpsertVoicemail(ctx, Voicemail{
		SourceID: "carrier-42", ReceivedAt: received, ContentType: "audio/wav", Audio: []byte("restored"),
	})
	require.ErrorIs(t, err, ErrVoicemailDeleted)
}
