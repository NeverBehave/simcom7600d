package simmodem

import (
	"bufio"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSim_RespondsToAT(t *testing.T) {
	s, peer := New(t)
	s.OnExact("AT", "OK")
	defer s.Close()

	_, err := peer.Write([]byte("AT\r\n"))
	require.NoError(t, err)

	br := bufio.NewReader(peer)
	got := readUntil(t, br, "OK\r\n", time.Second)
	require.Contains(t, got, "OK\r\n")
}

func TestSim_EmitsURC(t *testing.T) {
	s, peer := New(t)
	defer s.Close()
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.EmitURC("RING")
	}()

	br := bufio.NewReader(peer)
	got := readUntil(t, br, "RING\r\n", time.Second)
	require.Contains(t, got, "RING\r\n")
}

func TestSim_PromptForCMGS(t *testing.T) {
	s, peer := New(t)
	s.OnPrefix("AT+CMGS=", PromptThen("+CMGS: 42", "OK"))
	defer s.Close()

	_, _ = peer.Write([]byte("AT+CMGS=23\r\n"))
	br := bufio.NewReader(peer)
	require.Contains(t, readUntil(t, br, "> ", 500*time.Millisecond), "> ")
	// Caller writes PDU + 0x1A
	_, _ = peer.Write([]byte("00\x1a"))
	got := readUntil(t, br, "OK\r\n", time.Second)
	require.Contains(t, got, "+CMGS: 42\r\n")
	require.Contains(t, got, "OK\r\n")
}

func readUntil(t *testing.T, br *bufio.Reader, terminator string, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	var sb strings.Builder
	for time.Now().Before(deadline) {
		b, err := br.ReadByte()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		sb.WriteByte(b)
		if strings.HasSuffix(sb.String(), terminator) {
			return sb.String()
		}
	}
	t.Fatalf("timeout waiting for %q; got %q", terminator, sb.String())
	return ""
}
