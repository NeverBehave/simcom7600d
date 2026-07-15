package callaudio

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"sim7600d/internal/modem"
)

type fakeController struct {
	mu      sync.Mutex
	call    modem.Call
	started int
	stopped int
	stopCh  chan struct{}
}

func (f *fakeController) GetCall(context.Context, string) (modem.Call, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.call, nil
}
func (f *fakeController) StartCallAudio(context.Context, string) error {
	f.mu.Lock()
	f.started++
	f.mu.Unlock()
	return nil
}
func (f *fakeController) StopCallAudio(context.Context, string) error {
	f.mu.Lock()
	f.stopped++
	f.mu.Unlock()
	select {
	case f.stopCh <- struct{}{}:
	default:
	}
	return nil
}

func TestBridge_BidirectionalPCM(t *testing.T) {
	controller := &fakeController{
		call:   modem.Call{ID: "call-1", State: "active"},
		stopCh: make(chan struct{}, 1),
	}
	serverDevice, devicePeer := net.Pipe()
	defer devicePeer.Close()
	bridge := NewWithOpener(controller, "/dev/audio", func(string) (io.ReadWriteCloser, error) {
		return serverDevice, nil
	})
	router := chi.NewRouter()
	router.Get("/v1/calls/{id}/audio", bridge.ServeHTTP)
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/calls/call-1/audio"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{Subprotocols: []string{Subprotocol}})
	require.NoError(t, err)

	kind, ready, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, kind)
	require.Contains(t, string(ready), `"sample_rate":16000`)

	second, err := http.Get(server.URL + "/v1/calls/call-1/audio")
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, second.StatusCode)
	second.Body.Close()

	upstream := []byte{1, 2, 3, 4, 5, 6}
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, upstream))
	gotUpstream := make([]byte, len(upstream))
	require.NoError(t, devicePeer.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = io.ReadFull(devicePeer, gotUpstream)
	require.NoError(t, err)
	require.Equal(t, upstream, gotUpstream)

	downstream := []byte{9, 8, 7, 6}
	require.NoError(t, devicePeer.SetWriteDeadline(time.Now().Add(time.Second)))
	_, err = devicePeer.Write(downstream)
	require.NoError(t, err)
	kind, gotDownstream, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, kind)
	require.Equal(t, downstream, gotDownstream)

	require.NoError(t, conn.Close(websocket.StatusNormalClosure, "done"))
	select {
	case <-controller.stopCh:
	case <-ctx.Done():
		t.Fatal("audio stop command was not issued")
	}
	controller.mu.Lock()
	require.Equal(t, 1, controller.started)
	require.Equal(t, 1, controller.stopped)
	controller.mu.Unlock()
}

func TestBridge_RejectsInactiveCallBeforeUpgrade(t *testing.T) {
	controller := &fakeController{call: modem.Call{ID: "call-1", State: "alerting"}}
	bridge := NewWithOpener(controller, "/dev/audio", func(string) (io.ReadWriteCloser, error) {
		t.Fatal("inactive call must not open the audio device")
		return nil, nil
	})
	router := chi.NewRouter()
	router.Get("/v1/calls/{id}/audio", bridge.ServeHTTP)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/calls/call-1/audio")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}
