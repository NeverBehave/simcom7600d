// Package atproto parses lines arriving from the modem TTY into typed Frames.
// It is intentionally stateless: classification depends only on line content,
// never on whether a command is "in flight". The executor decides what to do
// with each frame based on its own state.
package atproto

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
)

type Kind int

const (
	KindIntermediate Kind = iota
	KindFinal
	KindURC
	KindPrompt
)

type FinalCode int

const (
	FinalNone FinalCode = iota
	FinalOK
	FinalError
	FinalCMEError
	FinalCMSError
	FinalNoCarrier
	FinalBusy
	FinalNoAnswer
	FinalConnect
)

type Frame struct {
	Kind  Kind
	Final FinalCode
	Line  string
	Code  int
}

var ErrEOF = io.EOF

type Scanner struct {
	br *bufio.Reader
}

func NewScanner(r io.Reader) *Scanner {
	return &Scanner{br: bufio.NewReader(r)}
}

var urcPrefixes = []string{
	"RING",
	"+CLIP:", "+CRING:",
	"+CMTI:", "+CMT:", "+CDS:",
	"+CREG:", "+CGREG:", "+CEREG:",
	"+CPIN:", "+CUSD:",
	"RDY", "+CFUN:", "PB DONE", "SMS DONE",
}

func (s *Scanner) Next() (Frame, error) {
	for {
		if peek, err := s.br.Peek(2); err == nil && peek[0] == '>' && peek[1] == ' ' {
			_, _ = s.br.Discard(2)
			return Frame{Kind: KindPrompt, Line: "> "}, nil
		}

		line, err := s.br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" {
				return Frame{}, ErrEOF
			}
			if !errors.Is(err, io.EOF) {
				return Frame{}, err
			}
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		return classify(line), nil
	}
}

func classify(line string) Frame {
	switch line {
	case "OK":
		return Frame{Kind: KindFinal, Line: line, Final: FinalOK}
	case "ERROR":
		return Frame{Kind: KindFinal, Line: line, Final: FinalError}
	case "NO CARRIER":
		return Frame{Kind: KindFinal, Line: line, Final: FinalNoCarrier}
	case "BUSY":
		return Frame{Kind: KindFinal, Line: line, Final: FinalBusy}
	case "NO ANSWER":
		return Frame{Kind: KindFinal, Line: line, Final: FinalNoAnswer}
	}
	if strings.HasPrefix(line, "CONNECT") {
		return Frame{Kind: KindFinal, Line: line, Final: FinalConnect}
	}
	if strings.HasPrefix(line, "+CME ERROR:") {
		return Frame{Kind: KindFinal, Line: line, Final: FinalCMEError, Code: extractCode(line)}
	}
	if strings.HasPrefix(line, "+CMS ERROR:") {
		return Frame{Kind: KindFinal, Line: line, Final: FinalCMSError, Code: extractCode(line)}
	}
	for _, p := range urcPrefixes {
		if line == p || strings.HasPrefix(line, p) {
			return Frame{Kind: KindURC, Line: line}
		}
	}
	return Frame{Kind: KindIntermediate, Line: line}
}

func extractCode(line string) int {
	i := strings.IndexByte(line, ':')
	if i < 0 || i+1 >= len(line) {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(line[i+1:]))
	return n
}
