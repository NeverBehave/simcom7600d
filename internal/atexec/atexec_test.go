package atexec

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/atproto"
	"sim7600d/internal/simmodem"
	"sim7600d/internal/urc"
)

func setup(t *testing.T) (*Executor, *simmodem.Sim) {
	t.Helper()
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ex := New(peer, bus)
	t.Cleanup(func() { ex.Close(); bus.Close() })
	return ex, sim
}

func TestExec_OK(t *testing.T) {
	ex, sim := setup(t)
	sim.OnExact("AT", "OK")

	resp, err := ex.Exec(context.Background(), Cmd("AT"))
	require.NoError(t, err)
	require.Equal(t, atproto.FinalOK, resp.Final.Final)
}

func TestExec_IntermediateThenOK(t *testing.T) {
	ex, sim := setup(t)
	sim.OnExact("AT+CSQ", "+CSQ: 20,99", "OK")

	resp, err := ex.Exec(context.Background(), Cmd("AT+CSQ"))
	require.NoError(t, err)
	require.Equal(t, atproto.FinalOK, resp.Final.Final)
	require.Len(t, resp.Lines, 1)
	require.Equal(t, "+CSQ: 20,99", resp.Lines[0])
}

func TestExec_CMEErrorReturnedTyped(t *testing.T) {
	ex, sim := setup(t)
	sim.OnExact("AT+CPIN?", "+CME ERROR: 10")

	resp, err := ex.Exec(context.Background(), Cmd("AT+CPIN?"))
	require.NoError(t, err) // protocol error, not Go error
	require.Equal(t, atproto.FinalCMEError, resp.Final.Final)
	require.Equal(t, 10, resp.Final.Code)
}

func TestExec_ContextCancelDoesNotLeak(t *testing.T) {
	ex, sim := setup(t)
	// No handler registered -> sim returns ERROR after a delay we don't honor;
	// the request should still be cancelable.
	sim.OnExact("AT+SLOW" /* nothing */)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := ex.Exec(ctx, Cmd("AT+SLOW").WithTimeout(100*time.Millisecond))
	require.Error(t, err)
	// New requests still flow.
	sim.OnExact("AT", "OK")
	resp, err := ex.Exec(context.Background(), Cmd("AT"))
	require.NoError(t, err)
	require.Equal(t, atproto.FinalOK, resp.Final.Final)
}

func TestExec_CanceledWhileQueuedNeverReachesTransport(t *testing.T) {
	ex, sim := setup(t)
	started := make(chan struct{})
	release := make(chan struct{})
	sim.OnPrefix("AT+BLOCK", func(_ string, w io.Writer, _ func() ([]byte, error)) {
		close(started)
		<-release
		_, _ = w.Write([]byte("\r\nOK\r\n"))
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := ex.Exec(context.Background(), Cmd("AT+BLOCK").WithTimeout(time.Second))
		firstDone <- err
	}()
	<-started

	var executed atomic.Bool
	sim.OnPrefix("AT+DIAL", func(_ string, w io.Writer, _ func() ([]byte, error)) {
		executed.Store(true)
		_, _ = w.Write([]byte("\r\nOK\r\n"))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := ex.Exec(ctx, Cmd("AT+DIAL"))
	require.ErrorIs(t, err, context.DeadlineExceeded)

	close(release)
	require.NoError(t, <-firstDone)
	time.Sleep(100 * time.Millisecond)
	require.False(t, executed.Load(), "canceled queued command reached the modem")
}

func TestExec_URCRoutedToBus(t *testing.T) {
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ring := bus.Subscribe("RING")
	ex := New(peer, bus)
	defer func() { ex.Close(); bus.Close() }()

	sim.OnExact("AT", "OK")
	go sim.EmitURC("RING")
	_, err := ex.Exec(context.Background(), Cmd("AT"))
	require.NoError(t, err)

	select {
	case e := <-ring:
		require.Equal(t, "RING", e.Line)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected RING to be routed to URC bus")
	}
}

func TestExecutor_TransportEOFSignalsDone(t *testing.T) {
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ex := New(peer, bus)
	defer bus.Close()

	sim.Close()
	select {
	case <-ex.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected transport EOF to stop executor")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := ex.Exec(ctx, Cmd("AT"))
	require.ErrorIs(t, err, ErrTransportClosed)
}

func TestExecutor_CloseIsIdempotent(t *testing.T) {
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ex := New(peer, bus)
	require.NoError(t, ex.Close())
	require.NoError(t, ex.Close())
	sim.Close()
}
