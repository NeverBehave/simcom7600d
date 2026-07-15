package urc

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBus_FanOutByPrefix(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ring := b.Subscribe("RING")
	cmti := b.Subscribe("+CMTI:")

	b.Publish(Event{Line: "RING"})
	b.Publish(Event{Line: "+CMTI: \"ME\",5"})

	requireRecv(t, ring, "RING")
	requireRecv(t, cmti, "+CMTI: \"ME\",5")
}

func TestBus_MultipleSubscribers(t *testing.T) {
	b := NewBus()
	defer b.Close()

	a := b.Subscribe("RING")
	bb := b.Subscribe("RING")
	b.Publish(Event{Line: "RING"})
	requireRecv(t, a, "RING")
	requireRecv(t, bb, "RING")
}

func TestBus_AssignsMonotonicIDs(t *testing.T) {
	b := NewBus()
	defer b.Close()
	sub := b.Subscribe("X:")
	b.Publish(Event{Line: "X: 1"})
	b.Publish(Event{Line: "X: 2"})
	first := <-sub
	second := <-sub
	require.Greater(t, second.ID, first.ID)
}

func TestBus_UnknownGoesToUnknown(t *testing.T) {
	b := NewBus()
	defer b.Close()
	unk := b.Subscribe(UnknownPrefix)
	b.Publish(Event{Line: "+ZZZZ: hi"})
	requireRecv(t, unk, "+ZZZZ: hi")
}

func requireRecv(t *testing.T, ch <-chan Event, contains string) {
	t.Helper()
	select {
	case e := <-ch:
		require.True(t, strings.Contains(e.Line, contains), "got %q", e.Line)
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for event containing %q", contains)
	}
}

// Make sure linter doesn't complain about unused atomic in this file.
var _ atomic.Int64
