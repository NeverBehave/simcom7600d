// Package urc demultiplexes Unsolicited Result Codes from the modem to
// subscriber channels. The bus also stamps each event with a monotonic ID
// so consumers can resume after a daemon restart.
package urc

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// UnknownPrefix matches any URC with no registered subscriber. Subscribe to
// this to receive everything else (typically for logging).
const UnknownPrefix = "__unknown__"

type Event struct {
	ID   int64     // monotonic, assigned at publish time
	TS   time.Time // wall clock at publish
	Line string    // raw URC line, no \r\n
}

// Bus fans Events out to subscriber channels keyed by line prefix.
type Bus struct {
	mu     sync.Mutex
	subs   map[string][]chan Event
	nextID atomic.Int64
	closed bool
}

func NewBus() *Bus { return &Bus{subs: make(map[string][]chan Event)} }

// Subscribe returns a buffered channel that receives Events whose Line begins
// with prefix. The channel is closed by Close().
func (b *Bus) Subscribe(prefix string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, 32)
	b.subs[prefix] = append(b.subs[prefix], ch)
	return ch
}

// Publish writes an Event. It never blocks: subscribers with full channels
// drop the event (we'll log and bump a counter at the call site).
func (b *Bus) Publish(e Event) {
	if e.ID == 0 {
		e.ID = b.nextID.Add(1)
	}
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	matched := false
	for prefix, chs := range b.subs {
		if prefix == UnknownPrefix {
			continue
		}
		if e.Line == prefix || strings.HasPrefix(e.Line, prefix) {
			matched = true
			for _, ch := range chs {
				select {
				case ch <- e:
				default:
				}
			}
		}
	}
	if !matched {
		for _, ch := range b.subs[UnknownPrefix] {
			select {
			case ch <- e:
			default:
			}
		}
	}
}

// Close closes all subscriber channels. After Close, Publish is a no-op.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, chs := range b.subs {
		for _, ch := range chs {
			close(ch)
		}
	}
	b.subs = nil
}
