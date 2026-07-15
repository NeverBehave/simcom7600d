// Package callaudio bridges the SIM7600 raw USB Audio port to a browser
// WebSocket. Audio frames are signed 16-bit little-endian mono PCM at 16 kHz.
package callaudio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"sim7600d/internal/modem"
	"sim7600d/internal/ttyx"
)

const (
	Subprotocol = "sim7600.audio.v1"
	SampleRate  = 16000
	maxFrame    = 64 << 10
)

var errCallEnded = errors.New("cellular call ended")

// Controller is the call-control subset needed by the audio data plane.
type Controller interface {
	GetCall(context.Context, string) (modem.Call, error)
	StartCallAudio(context.Context, string) error
	StopCallAudio(context.Context, string) error
}

type DeviceOpener func(string) (io.ReadWriteCloser, error)

// Bridge owns exclusive access to one modem audio port. SIM7600 supports one
// voice leg, so a second browser must not steal or mix the first session.
type Bridge struct {
	controller Controller
	devicePath string
	openDevice DeviceOpener

	mu     sync.Mutex
	busy   bool
	closed bool
	cancel context.CancelFunc
}

func New(controller Controller, devicePath string) *Bridge {
	return NewWithOpener(controller, devicePath, func(path string) (io.ReadWriteCloser, error) {
		return ttyx.OpenTTY(path)
	})
}

func NewWithOpener(controller Controller, devicePath string, opener DeviceOpener) *Bridge {
	return &Bridge{controller: controller, devicePath: devicePath, openDevice: opener}
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	callID := chi.URLParam(r, "id")
	if callID == "" {
		http.Error(w, "missing call id", http.StatusBadRequest)
		return
	}
	if !b.acquire() {
		http.Error(w, "call audio is already connected in another browser", http.StatusConflict)
		return
	}
	defer b.release()

	call, err := b.controller.GetCall(r.Context(), callID)
	if err != nil {
		http.Error(w, "call not found", http.StatusNotFound)
		return
	}
	if !canAttach(call) {
		http.Error(w, "call audio is unavailable in this call state", http.StatusConflict)
		return
	}

	device, err := b.openDevice(b.devicePath)
	if err != nil {
		slog.Error("open call audio device", "path", b.devicePath, "err", err)
		http.Error(w, "modem audio device is unavailable", http.StatusServiceUnavailable)
		return
	}
	defer device.Close()

	startCtx, startCancel := context.WithTimeout(r.Context(), 5*time.Second)
	err = b.controller.StartCallAudio(startCtx, callID)
	startCancel()
	if err != nil {
		slog.Error("start call audio", "call_id", callID, "err", err)
		http.Error(w, "modem refused to start call audio", http.StatusBadGateway)
		return
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := b.controller.StopCallAudio(stopCtx, callID); err != nil {
			slog.Warn("stop call audio", "call_id", callID, "err", err)
		}
	}()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{Subprotocol},
	})
	if err != nil {
		slog.Warn("accept call audio websocket", "call_id", callID, "err", err)
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxFrame)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.setCancel(cancel)
	if err := writeReady(ctx, conn, callID); err != nil {
		return
	}

	errCh := make(chan error, 3)
	go func() { errCh <- streamBrowserToModem(ctx, conn, device) }()
	go func() { errCh <- streamModemToBrowser(ctx, conn, device) }()
	go func() { errCh <- b.monitorCall(ctx, callID) }()

	streamErr := <-errCh
	cancel()
	// Closing the raw port interrupts a read that would otherwise remain idle
	// after the WebSocket or cellular call ends.
	_ = device.Close()
	if errors.Is(streamErr, errCallEnded) {
		_ = conn.Close(websocket.StatusNormalClosure, "cellular call ended")
		return
	}
	if status := websocket.CloseStatus(streamErr); status != -1 {
		return
	}
	slog.Warn("call audio stream ended", "call_id", callID, "err", streamErr)
	_ = conn.Close(websocket.StatusInternalError, "audio stream stopped")
}

func (b *Bridge) acquire() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.busy || b.closed {
		return false
	}
	b.busy = true
	return true
}

func (b *Bridge) release() {
	b.mu.Lock()
	b.busy = false
	b.cancel = nil
	b.mu.Unlock()
}

func (b *Bridge) setCancel(cancel context.CancelFunc) {
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()
}

// Close terminates an attached browser session during daemon shutdown.
func (b *Bridge) Close() {
	b.mu.Lock()
	b.closed = true
	cancel := b.cancel
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func writeReady(ctx context.Context, conn *websocket.Conn, callID string) error {
	payload, err := json.Marshal(map[string]any{
		"type": "ready", "call_id": callID, "sample_rate": SampleRate,
		"channels": 1, "encoding": "pcm_s16le",
	})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, payload)
}

func streamBrowserToModem(ctx context.Context, conn *websocket.Conn, device io.Writer) error {
	for {
		kind, reader, err := conn.Reader(ctx)
		if err != nil {
			return err
		}
		if kind != websocket.MessageBinary {
			_, _ = io.Copy(io.Discard, reader)
			return errors.New("call audio accepts binary PCM frames only")
		}
		frame, err := io.ReadAll(io.LimitReader(reader, maxFrame+1))
		if err != nil {
			return err
		}
		if len(frame) > maxFrame {
			return errors.New("call audio frame is too large")
		}
		if len(frame)%2 != 0 {
			return errors.New("call audio frame has an incomplete PCM sample")
		}
		if err := writeFull(device, frame); err != nil {
			return fmt.Errorf("write modem audio: %w", err)
		}
	}
}

func streamModemToBrowser(ctx context.Context, conn *websocket.Conn, device io.Reader) error {
	// Forty milliseconds keeps framing overhead low without adding enough
	// latency to make conversation feel unnatural.
	buf := make([]byte, SampleRate*2*40/1000)
	for {
		n, err := device.Read(buf)
		if n > 0 {
			if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return fmt.Errorf("read modem audio: %w", err)
		}
	}
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

func (b *Bridge) monitorCall(ctx context.Context, callID string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			call, err := b.controller.GetCall(ctx, callID)
			if err != nil || !canAttach(call) {
				return errCallEnded
			}
		}
	}
}

func canAttach(call modem.Call) bool {
	if call.State == "active" {
		return true
	}
	return call.Direction == "out" && (call.State == "dialing" || call.State == "alerting")
}
