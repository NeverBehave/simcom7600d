package atexec

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"sim7600d/internal/atproto"
	"sim7600d/internal/simmodem"
	"sim7600d/internal/urc"
)

// SendPDU is the canonical multi-step transaction we'll need for SMS.
func SendPDU(line, body string) Request {
	return Request{
		Line:    line,
		Timeout: 5_000_000_000, // 5s; modem.SendSMS will provide better timeouts
		Run: func(w io.Writer, frames func() (atproto.Frame, error)) (Response, error) {
			if _, err := w.Write([]byte(line + "\r\n")); err != nil {
				return Response{}, err
			}
			// Wait for "> " prompt.
			for {
				f, err := frames()
				if err != nil {
					return Response{}, err
				}
				if f.Kind == atproto.KindPrompt {
					break
				}
				if f.Kind == atproto.KindFinal && f.Final != atproto.FinalOK {
					return Response{Final: f}, nil
				}
			}
			if _, err := w.Write([]byte(body)); err != nil {
				return Response{}, err
			}
			if _, err := w.Write([]byte{0x1A}); err != nil {
				return Response{}, err
			}
			var resp Response
			for {
				f, err := frames()
				if err != nil {
					return resp, err
				}
				if f.Kind == atproto.KindIntermediate {
					resp.Lines = append(resp.Lines, f.Line)
					continue
				}
				if f.Kind == atproto.KindFinal {
					resp.Final = f
					return resp, nil
				}
			}
		},
	}
}

func TestExec_PromptDrivenSequence(t *testing.T) {
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ex := New(peer, bus)
	defer func() { ex.Close(); bus.Close() }()

	sim.OnPrefix("AT+CMGS=", simmodem.PromptThen("+CMGS: 7", "OK"))

	resp, err := ex.Exec(context.Background(), SendPDU("AT+CMGS=23", "00ABC"))
	require.NoError(t, err)
	require.Equal(t, atproto.FinalOK, resp.Final.Final)
	require.Equal(t, []string{"+CMGS: 7"}, resp.Lines)
}

func TestExec_PromptSequenceCancel(t *testing.T) {
	sim, peer := simmodem.New(t)
	bus := urc.NewBus()
	ex := New(peer, bus)
	defer func() { ex.Close(); bus.Close() }()

	// Sim emits prompt but never the final.
	sim.OnPrefix("AT+CMGS=", func(_ string, w io.Writer, _ func() ([]byte, error)) {
		_, _ = w.Write([]byte("\r\n> "))
		// drop the body, never reply with final
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50_000_000) // 50ms
	defer cancel()
	_, err := ex.Exec(ctx, SendPDU("AT+CMGS=10", "00").WithTimeout(100_000_000))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded))
}
