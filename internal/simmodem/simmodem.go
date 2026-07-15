// Package simmodem provides a scripted, in-process modem fake. It owns one
// end of a unix socketpair so the executor sees real I/O semantics.
package simmodem

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// Handler decides how to respond to a received AT command line.
// It receives the line (without trailing \r\n) and a writer for response bytes.
// For prompt-driven sequences it can read more bytes from `more` (the body
// after the prompt, until 0x1A is seen).
type Handler func(line string, w io.Writer, more func() ([]byte, error))

// Sim is a scripted modem fake.
type Sim struct {
	mu       sync.Mutex
	handlers []handlerEntry
	a, b     *os.File // a = sim's read/write side, b = peer for the test
	br       *bufio.Reader
	closed   bool
}

type handlerEntry struct {
	match func(string) bool
	h     Handler
}

// New returns a new Sim and the peer file descriptor (the end the executor
// or test would attach to). The Sim's reader goroutine starts immediately.
func New(t *testing.T) (*Sim, *os.File) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	a := os.NewFile(uintptr(fds[0]), "sim")
	b := os.NewFile(uintptr(fds[1]), "peer")
	s := &Sim{a: a, b: b, br: bufio.NewReader(a)}
	t.Cleanup(s.Close)
	go s.loop()
	return s, b
}

// OnExact installs a handler that fires for the exact line `cmd` and writes
// each response line followed by \r\n.
func (s *Sim) OnExact(cmd string, response ...string) {
	s.OnMatch(func(l string) bool { return l == cmd }, simpleResponse(response))
}

// OnPrefix installs a handler that fires when the line starts with `prefix`.
func (s *Sim) OnPrefix(prefix string, h Handler) {
	s.OnMatch(func(l string) bool { return strings.HasPrefix(l, prefix) }, h)
}

// OnMatch installs an arbitrary matcher.
func (s *Sim) OnMatch(match func(string) bool, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, handlerEntry{match, h})
}

// EmitURC writes lines as if the modem volunteered them.
func (s *Sim) EmitURC(lines ...string) {
	s.write(lines...)
}

func (s *Sim) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.a.Close()
	s.b.Close()
}

func (s *Sim) loop() {
	for {
		line, err := s.br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) || s.isClosed() {
				return
			}
			return
		}
		line = strings.TrimRight(line, "\r\n")
		line = strings.TrimLeft(line, "\x1b") // ESC = abort/no-op (real modem behavior)
		if line == "" {
			continue
		}
		s.dispatch(line)
	}
}

func (s *Sim) dispatch(line string) {
	s.mu.Lock()
	hs := append([]handlerEntry(nil), s.handlers...)
	s.mu.Unlock()
	for _, e := range hs {
		if e.match(line) {
			e.h(line, writerFunc(s.writeRaw), s.readBody)
			return
		}
	}
	// Default: ERROR.
	s.write("ERROR")
}

// readBody reads bytes from the peer until 0x1A is seen, returning what was
// read (excluding the 0x1A). Used by PromptThen.
func (s *Sim) readBody() ([]byte, error) {
	var buf []byte
	for {
		b, err := s.br.ReadByte()
		if err != nil {
			return buf, err
		}
		if b == 0x1A {
			return buf, nil
		}
		buf = append(buf, b)
	}
}

func (s *Sim) write(lines ...string) {
	var sb strings.Builder
	sb.WriteString("\r\n")
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\r\n")
	}
	s.writeRaw([]byte(sb.String()))
}

func (s *Sim) writeRaw(p []byte) (int, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, errors.New("simmodem: closed")
	}
	s.mu.Unlock()
	return s.a.Write(p)
}

func (s *Sim) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// simpleResponse returns a Handler that writes the given lines as a normal AT
// response (\r\n prefix; each line + \r\n).
func simpleResponse(lines []string) Handler {
	return func(_ string, w io.Writer, _ func() ([]byte, error)) {
		var sb strings.Builder
		sb.WriteString("\r\n")
		for _, l := range lines {
			sb.WriteString(l)
			sb.WriteString("\r\n")
		}
		_, _ = w.Write([]byte(sb.String()))
	}
}

// PromptThen returns a Handler that emits "> " (prompt), waits for the body
// terminated by 0x1A, then writes the trailing response lines.
func PromptThen(lines ...string) Handler {
	return func(_ string, w io.Writer, more func() ([]byte, error)) {
		_, _ = w.Write([]byte("\r\n> "))
		_, _ = more()
		var sb strings.Builder
		sb.WriteString("\r\n")
		for _, l := range lines {
			sb.WriteString(l)
			sb.WriteString("\r\n")
		}
		_, _ = w.Write([]byte(sb.String()))
	}
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
