package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParts_RoundTrip(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, s.PutPart(ctx, InboundPart{
		Ref: 7, Total: 2, Seq: 1, FromAddr: "+1",
		Body: "hello ", Encoding: "gsm7", RawPDU: "00", ReceivedAt: now,
	}))
	parts, err := s.GetPartsForReassembly(ctx, "+1", 7)
	require.NoError(t, err)
	require.Len(t, parts, 1)

	require.NoError(t, s.PutPart(ctx, InboundPart{
		Ref: 7, Total: 2, Seq: 2, FromAddr: "+1",
		Body: "world", Encoding: "gsm7", RawPDU: "01", ReceivedAt: now,
	}))
	parts, err = s.GetPartsForReassembly(ctx, "+1", 7)
	require.NoError(t, err)
	require.Len(t, parts, 2)
	require.Equal(t, "hello ", parts[0].Body)
	require.Equal(t, "world", parts[1].Body)

	require.NoError(t, s.DeleteParts(ctx, "+1", 7))
	parts, err = s.GetPartsForReassembly(ctx, "+1", 7)
	require.NoError(t, err)
	require.Empty(t, parts)
}
