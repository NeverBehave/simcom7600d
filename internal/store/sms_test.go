package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMS_InboundInsertAndList(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	in := Inbound{
		ID:         NewULID(),
		FromAddr:   "+15551234567",
		Body:       "hello",
		Encoding:   "gsm7",
		Parts:      1,
		ReceivedAt: time.Now().UTC(),
		DedupeKey:  "k1",
		RawPDUs:    `["00..."]`,
	}
	require.NoError(t, s.InsertInbound(ctx, in))

	got, err := s.ListInbound(ctx, InboundFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "hello", got[0].Body)
}

func TestSMS_InboundDedupe(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	in := Inbound{ID: NewULID(), FromAddr: "+1", Body: "x", Encoding: "gsm7", Parts: 1, ReceivedAt: time.Now().UTC(), DedupeKey: "k", RawPDUs: "[]"}
	require.NoError(t, s.InsertInbound(ctx, in))
	in.ID = NewULID()
	err := s.InsertInbound(ctx, in)
	require.ErrorIs(t, err, ErrDuplicate)
}

func TestSMS_OutboundLifecycle(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	o := Outbound{
		ID: NewULID(), ToAddr: "+1", Body: "hi", Encoding: "gsm7",
		Parts: 1, State: "queued", DeliveryReport: false,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, s.InsertOutbound(ctx, o))
	require.NoError(t, s.SetOutboundState(ctx, o.ID, "submitted", []int{42}, "", ""))

	got, err := s.GetOutbound(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, "submitted", got.State)
	require.Equal(t, []int{42}, got.MRs)
}
