package ttyx

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// pair returns two *os.File ends of a socketpair. Use the first as the "TTY",
// the second as the test's view of the other side.
func pair(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	a := os.NewFile(uintptr(fds[0]), "a")
	b := os.NewFile(uintptr(fds[1]), "b")
	t.Cleanup(func() { a.Close(); b.Close() })
	return a, b
}

func TestTransport_ReadWritePassthrough(t *testing.T) {
	a, b := pair(t)
	tr := newFromFile(a)
	defer tr.Close()

	// Write from the test side, expect to read it from the transport.
	_, err := b.Write([]byte("AT\r\n"))
	require.NoError(t, err)
	buf := make([]byte, 4)
	require.NoError(t, readFull(tr, buf, 1*time.Second))
	require.Equal(t, "AT\r\n", string(buf))

	// Write into the transport, expect to read on the test side.
	_, err = tr.Write([]byte("OK\r\n"))
	require.NoError(t, err)
	out := make([]byte, 4)
	require.NoError(t, readFull(b, out, 1*time.Second))
	require.Equal(t, "OK\r\n", string(out))
}

func TestTransport_HealthDownOnEOF(t *testing.T) {
	a, b := pair(t)
	tr := newFromFile(a)
	defer tr.Close()

	go func() {
		time.Sleep(20 * time.Millisecond)
		b.Close()
	}()

	buf := make([]byte, 1)
	_, err := tr.Read(buf)
	require.ErrorIs(t, err, io.EOF)

	select {
	case st := <-tr.Health():
		require.Equal(t, StateFlapping, st)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected Flapping after EOF")
	}
}

func TestTransport_CloseInterruptsIdleRead(t *testing.T) {
	a, b := pair(t)
	require.NoError(t, unix.SetNonblock(int(a.Fd()), true))
	tr := newFromFile(a)
	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := tr.Read(buf)
		readDone <- err
	}()

	closeDone := make(chan error, 1)
	go func() { closeDone <- tr.Close() }()
	select {
	case err := <-closeDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not interrupt idle read")
	}
	select {
	case err := <-readDone:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("idle read remained blocked after Close")
	}
	_ = b.Close()
}

func readFull(r io.Reader, buf []byte, d time.Duration) error {
	type res struct{ err error }
	ch := make(chan res, 1)
	go func() { _, err := io.ReadFull(r, buf); ch <- res{err} }()
	select {
	case r := <-ch:
		return r.err
	case <-time.After(d):
		return os.ErrDeadlineExceeded
	}
}
