// Package ttyx is the transport layer between sim7600d and the modem TTY.
// It wraps an io.ReadWriteCloser, sets termios on real TTYs, and tracks health
// so the executor can recycle the connection on errors.
package ttyx

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

type State int

const (
	StateUp State = iota
	StateFlapping
	StateDown
)

func (s State) String() string {
	switch s {
	case StateUp:
		return "up"
	case StateFlapping:
		return "flapping"
	case StateDown:
		return "down"
	}
	return "?"
}

// Transport is what the executor sees. It is an io.ReadWriteCloser plus a
// health channel and an explicit Recycle() request.
type Transport interface {
	io.ReadWriteCloser
	Health() <-chan State
	Recycle() // close current fd and reopen on next Read/Write
}

type fileTransport struct {
	mu     sync.Mutex
	f      *os.File
	health chan State
	closed atomic.Bool
}

func newFromFile(f *os.File) *fileTransport {
	return &fileTransport{f: f, health: make(chan State, 4)}
}

// OpenTTY opens path as a serial-ish device, applies raw termios, returns a
// Transport. Used in production with /dev/ttyUSB3 and in dev with the socat
// PTY at /tmp/sim7600. For socketpair-backed test fakes, use newFromFile.
func OpenTTY(path string) (Transport, error) {
	f, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	// Keep O_NONBLOCK set. os.File presents blocking Read semantics through
	// Go's poller, while Close can still interrupt an idle serial read.
	if err := applyRawTermios(int(f.Fd())); err != nil {
		// Not fatal — socat-PTY and socketpair don't support termios.
		// We log via the caller; here we just continue.
		_ = err
	}
	return newFromFile(f), nil
}

func (t *fileTransport) Read(p []byte) (int, error) {
	t.mu.Lock()
	f := t.f
	t.mu.Unlock()
	if f == nil {
		return 0, errors.New("ttyx: closed")
	}
	n, err := f.Read(p)
	if err != nil && (errors.Is(err, io.EOF) || isTransportErr(err)) {
		t.flap()
	}
	return n, err
}

func (t *fileTransport) Write(p []byte) (int, error) {
	t.mu.Lock()
	f := t.f
	t.mu.Unlock()
	if f == nil {
		return 0, errors.New("ttyx: closed")
	}
	n, err := f.Write(p)
	if err != nil && isTransportErr(err) {
		t.flap()
	}
	return n, err
}

func (t *fileTransport) Close() error {
	t.closed.Store(true)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		return nil
	}
	err := t.f.Close()
	t.f = nil
	return err
}

func (t *fileTransport) Health() <-chan State { return t.health }

func (t *fileTransport) Recycle() { t.flap() }

func (t *fileTransport) flap() {
	select {
	case t.health <- StateFlapping:
	default:
	}
}

func isTransportErr(err error) bool {
	return errors.Is(err, unix.EIO) || errors.Is(err, os.ErrClosed)
}

// Reopen recreates the underlying file. The caller (executor) drives this
// after seeing StateFlapping. backoff is exponential, capped at 30s.
func Reopen(path string, attempt int) (*os.File, time.Duration) {
	d := time.Duration(1<<min(attempt, 5)) * 500 * time.Millisecond
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	time.Sleep(d)
	f, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, d
	}
	_ = applyRawTermios(int(f.Fd()))
	return f, d
}
