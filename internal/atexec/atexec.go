// Package atexec serializes AT commands over a single transport. One goroutine
// reads frames; another consumes the request channel. URCs are forwarded to a
// urc.Bus regardless of pending-command state. Final-result frames complete
// the in-flight request.
package atexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"sim7600d/internal/atproto"
	"sim7600d/internal/urc"
)

const (
	defaultTimeout = 2 * time.Second
	pacing         = 50 * time.Millisecond
)

var (
	ErrTransportClosed = errors.New("atexec: transport closed")
	ErrTimeout         = errors.New("atexec: command timeout")
	ErrModemReset      = errors.New("atexec: modem reset")
)

// Cmd builds a single-line request. Use WithTimeout to override the default.
func Cmd(line string) Request {
	return Request{Line: line, Timeout: defaultTimeout}
}

// Request is a unit of work for the executor. For multi-step interactions
// (e.g. AT+CMGS prompt → PDU + 0x1A), provide a Run callback (Task 8).
type Request struct {
	Line    string
	Timeout time.Duration
	Run     RunFunc // optional; if set, replaces the default single-line handler
}

func (r Request) WithTimeout(d time.Duration) Request {
	r.Timeout = d
	return r
}

// RunFunc is invoked inside the executor goroutine. It owns the TTY for the
// duration of the call. Use frames() to read the next classified frame; use
// w.Write to send raw bytes (the line and \r\n must be included by the caller).
type RunFunc func(w io.Writer, frames func() (atproto.Frame, error)) (Response, error)

// Response is what callers see. Lines are intermediate response lines; Final
// is the terminating frame.
type Response struct {
	Lines []string
	Final atproto.Frame
}

// Executor owns the transport read loop and the request queue.
type Executor struct {
	tr        io.ReadWriteCloser
	scanner   *atproto.Scanner
	bus       *urc.Bus
	requests  chan reqEnv
	done      chan struct{}
	doneOnce  sync.Once
	closeOnce sync.Once
	frames    chan atproto.Frame
	active    atomic.Bool
}

type reqEnv struct {
	ctx context.Context
	req Request
	out chan respEnv
}

type respEnv struct {
	resp Response
	err  error
}

// New creates an Executor reading from / writing to tr. URCs are published to
// bus. Read and dispatch goroutines start immediately.
func New(tr io.ReadWriteCloser, bus *urc.Bus) *Executor {
	e := &Executor{
		tr:       tr,
		scanner:  atproto.NewScanner(tr),
		bus:      bus,
		requests: make(chan reqEnv, 16),
		done:     make(chan struct{}),
		frames:   make(chan atproto.Frame, 64),
	}
	go e.readLoop()
	go e.dispatchLoop()
	return e
}

// Exec submits a request and blocks until response or context cancel.
func (e *Executor) Exec(ctx context.Context, req Request) (Response, error) {
	if req.Timeout == 0 {
		req.Timeout = defaultTimeout
	}
	out := make(chan respEnv, 1)
	select {
	case e.requests <- reqEnv{ctx: ctx, req: req, out: out}:
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-e.done:
		return Response{}, ErrTransportClosed
	}
	select {
	case r := <-out:
		return r.resp, r.err
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-e.done:
		return Response{}, ErrTransportClosed
	}
}

// Close shuts down the executor. Outstanding requests get ErrTransportClosed.
func (e *Executor) Close() error {
	var err error
	e.closeOnce.Do(func() {
		e.stop()
		err = e.tr.Close()
	})
	return err
}

// Done is closed when the executor is shut down or the modem transport fails.
func (e *Executor) Done() <-chan struct{} { return e.done }

// QueueStats returns a cheap diagnostic snapshot for the admin UI.
func (e *Executor) QueueStats() (queued, capacity int, active, transportUp bool) {
	select {
	case <-e.done:
		transportUp = false
	default:
		transportUp = true
	}
	return len(e.requests), cap(e.requests), e.active.Load(), transportUp
}

func (e *Executor) stop() { e.doneOnce.Do(func() { close(e.done) }) }

// readLoop reads frames forever. URCs go to the bus immediately. Other frames
// go to the frames channel for the dispatch loop to consume.
func (e *Executor) readLoop() {
	for {
		f, err := e.scanner.Next()
		if err != nil {
			e.stop()
			close(e.frames)
			return
		}
		if f.Kind == atproto.KindURC {
			e.bus.Publish(urc.Event{Line: f.Line})
			continue
		}
		select {
		case e.frames <- f:
		case <-e.done:
			return
		}
	}
}

// dispatchLoop pulls one request off the queue at a time and runs it.
func (e *Executor) dispatchLoop() {
	for {
		// Drain any stale frames left over (e.g. NO CARRIER from a missed call
		// outside a command context). Without this, the next command would
		// consume them as its own response.
		for drained := true; drained; {
			select {
			case _, ok := <-e.frames:
				if !ok {
					return
				}
			default:
				drained = false
			}
		}
		select {
		case env, ok := <-e.requests:
			if !ok {
				return
			}
			// The caller may have given up while this request was waiting behind
			// another modem operation. Never let an already-canceled request reach
			// the transport: doing so can turn an HTTP timeout into a later,
			// unexpected call or duplicate SMS.
			if err := env.ctx.Err(); err != nil {
				env.out <- respEnv{err: err}
				continue
			}
			e.active.Store(true)
			resp, err := e.run(env.ctx, env.req)
			e.active.Store(false)
			env.out <- respEnv{resp: resp, err: err}
			time.Sleep(pacing)
		case <-e.done:
			return
		}
	}
}

func (e *Executor) run(ctx context.Context, req Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	if req.Run != nil {
		return req.Run(e.tr, e.nextFrame(ctx))
	}
	return e.runSimple(ctx, req)
}

func (e *Executor) runSimple(ctx context.Context, req Request) (Response, error) {
	if _, err := fmt.Fprintf(e.tr, "%s\r\n", req.Line); err != nil {
		return Response{}, err
	}
	var resp Response
	for {
		f, err := e.readFrame(ctx)
		if err != nil {
			return resp, err
		}
		switch f.Kind {
		case atproto.KindIntermediate, atproto.KindPrompt:
			resp.Lines = append(resp.Lines, f.Line)
		case atproto.KindFinal:
			resp.Final = f
			return resp, nil
		}
	}
}

// nextFrame returns a closure suitable for RunFunc.
func (e *Executor) nextFrame(ctx context.Context) func() (atproto.Frame, error) {
	return func() (atproto.Frame, error) { return e.readFrame(ctx) }
}

func (e *Executor) readFrame(ctx context.Context) (atproto.Frame, error) {
	select {
	case f, ok := <-e.frames:
		if !ok {
			return atproto.Frame{}, ErrTransportClosed
		}
		return f, nil
	case <-ctx.Done():
		// Best-effort abort: send ESC, drain briefly. We don't guarantee the
		// modem is back to a known-good state — that's the recycle path.
		_, _ = e.tr.Write([]byte{0x1B})
		drainCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		go func() {
			for {
				select {
				case <-drainCtx.Done():
					return
				case <-e.frames:
				}
			}
		}()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return atproto.Frame{}, ErrTimeout
		}
		return atproto.Frame{}, ctx.Err()
	}
}
