package modem

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sim7600d/internal/atexec"
	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

type Facade struct {
	ex    *atexec.Executor
	bus   *urc.Bus
	st    *store.Store
	epoch atomic.Int64

	mu     sync.Mutex
	cached ModemStatus
	callMu sync.Mutex
	// clccMissing counts consecutive authoritative CLCC snapshots that omit an
	// open call. A short grace period prevents transient multi-call transitions
	// from deleting a held call from the UI.
	clccMissing map[string]int

	incomingCalls chan IncomingCall
	inboundSMS    chan SMSArrived
	lifecycle     chan LifecycleEvent
	stopped       chan struct{}
	wg            sync.WaitGroup
	workflowsOnce sync.Once
	startedAt     time.Time
}

func New(ex *atexec.Executor, bus *urc.Bus, st *store.Store) (*Facade, error) {
	f := &Facade{
		ex:            ex,
		bus:           bus,
		st:            st,
		incomingCalls: make(chan IncomingCall, 32),
		inboundSMS:    make(chan SMSArrived, 32),
		lifecycle:     make(chan LifecycleEvent, 16),
		stopped:       make(chan struct{}),
		startedAt:     time.Now(),
		clccMissing:   make(map[string]int),
	}
	return f, nil
}

func (f *Facade) Close() {
	select {
	case <-f.stopped:
	default:
		close(f.stopped)
	}
	// A workflow may currently be blocked in Exec. Stop the executor before
	// waiting so shutdown does not depend on another modem frame arriving.
	_ = f.ex.Close()
	f.wg.Wait()
}

// Boot applies the canonical baseline AT config. Idempotent.
func (f *Facade) Boot(ctx context.Context) error {
	for _, cmd := range []string{
		"ATE0", "AT+CMEE=2", "AT+CMGF=0",
		"AT+CNMI=2,1,0,1,0", "AT+CLIP=1", "AT+CRC=1",
		`AT+CSCS="UCS2"`,
	} {
		if _, err := f.ex.Exec(ctx, atexec.Cmd(cmd)); err != nil {
			return fmt.Errorf("boot %s: %w", cmd, err)
		}
	}
	f.workflowsOnce.Do(func() {
		f.startInboundWorkflow()
		f.startCallWorkflow()
	})
	return nil
}

func (f *Facade) startCallWorkflow() { f.startCallWorkflowImpl() }

// Status reads the modem snapshot. If refresh is false, returns the cached
// kv snapshot if present and updated within the last 30 seconds.
func (f *Facade) Status(ctx context.Context, refresh bool) (ModemStatus, error) {
	if !refresh {
		if cached, ok, _ := f.readCachedStatus(ctx); ok && time.Since(cached.UpdatedAt) < 30*time.Second {
			return cached, nil
		}
	}
	st, err := f.queryStatus(ctx)
	if err != nil {
		return ModemStatus{}, err
	}
	f.cacheStatus(ctx, st)
	return st, nil
}

func (f *Facade) queryStatus(ctx context.Context) (ModemStatus, error) {
	st := ModemStatus{Epoch: f.epoch.Load(), UptimeSec: int64(time.Since(f.startedAt).Seconds()), UpdatedAt: time.Now().UTC()}

	// ATI -> model, firmware, IMEI
	r, err := f.ex.Exec(ctx, atexec.Cmd("ATI").WithTimeout(2*time.Second))
	if err != nil {
		return st, err
	}
	for _, ln := range r.Lines {
		switch {
		case strings.HasPrefix(ln, "Model:"):
			st.Model = strings.TrimSpace(strings.TrimPrefix(ln, "Model:"))
		case strings.HasPrefix(ln, "Revision:"):
			st.Firmware = strings.TrimSpace(strings.TrimPrefix(ln, "Revision:"))
		case strings.HasPrefix(ln, "IMEI:"):
			st.IMEI = strings.TrimSpace(strings.TrimPrefix(ln, "IMEI:"))
		}
	}

	// SIM state — +CPIN: is classified as URC by atproto, so subscribe before exec.
	{
		cpinCh := f.bus.Subscribe("+CPIN:")
		_, _ = f.ex.Exec(ctx, atexec.Cmd("AT+CPIN?"))
		select {
		case ev := <-cpinCh:
			v := strings.TrimSpace(strings.TrimPrefix(ev.Line, "+CPIN:"))
			st.SIM.State = mapPinState(v)
		default:
		}
	}

	// CSQ
	if r, err := f.ex.Exec(ctx, atexec.Cmd("AT+CSQ")); err == nil {
		for _, ln := range r.Lines {
			if strings.HasPrefix(ln, "+CSQ:") {
				st.Network.CSQ = parseCSQ(ln)
			}
		}
	}

	// Operator
	if r, err := f.ex.Exec(ctx, atexec.Cmd("AT+COPS?")); err == nil {
		for _, ln := range r.Lines {
			if strings.HasPrefix(ln, "+COPS:") {
				st.SIM.Operator = parseOperator(ln)
			}
		}
	}

	// Registered — +CREG: is classified as URC by atproto, so subscribe before exec.
	{
		cregCh := f.bus.Subscribe("+CREG:")
		_, _ = f.ex.Exec(ctx, atexec.Cmd("AT+CREG?"))
		select {
		case ev := <-cregCh:
			st.Network.Registered = parseRegistered(ev.Line)
		default:
		}
	}

	// Battery
	if r, err := f.ex.Exec(ctx, atexec.Cmd("AT+CBC")); err == nil {
		for _, ln := range r.Lines {
			if strings.HasPrefix(ln, "+CBC:") {
				st.BatteryV = parseBattery(ln)
			}
		}
	}

	// SIM identifiers
	if r, err := f.ex.Exec(ctx, atexec.Cmd("AT+CCID")); err == nil {
		for _, ln := range r.Lines {
			if strings.HasPrefix(ln, "+CCID:") {
				st.SIM.ICCID = strings.TrimSpace(strings.TrimPrefix(ln, "+CCID:"))
			}
		}
	}
	if r, err := f.ex.Exec(ctx, atexec.Cmd("AT+CIMI")); err == nil {
		for _, ln := range r.Lines {
			if !strings.HasPrefix(ln, "+") && len(ln) >= 14 {
				st.SIM.IMSI = strings.TrimSpace(ln)
				break
			}
		}
	}

	// Cell info
	if r, err := f.ex.Exec(ctx, atexec.Cmd("AT+CPSI?")); err == nil {
		for _, ln := range r.Lines {
			if strings.HasPrefix(ln, "+CPSI:") {
				parseCPSI(ln, &st.Network)
			}
		}
	}

	return st, nil
}

func (f *Facade) cacheStatus(ctx context.Context, st ModemStatus) {
	f.mu.Lock()
	f.cached = st
	f.mu.Unlock()
	if b, err := json.Marshal(st); err == nil {
		_ = f.st.PutKV(ctx, "status", string(b))
	}
}

func (f *Facade) readCachedStatus(ctx context.Context) (ModemStatus, bool, error) {
	v, ok, err := f.st.GetKV(ctx, "status")
	if err != nil || !ok {
		return ModemStatus{}, false, err
	}
	var st ModemStatus
	if err := json.Unmarshal([]byte(v), &st); err != nil {
		return ModemStatus{}, false, err
	}
	return st, true, nil
}

// --- channels surfaced to callers ---

func (f *Facade) OnIncomingCall() <-chan IncomingCall { return f.incomingCalls }
func (f *Facade) OnInboundSMS() <-chan SMSArrived     { return f.inboundSMS }
func (f *Facade) OnLifecycle() <-chan LifecycleEvent  { return f.lifecycle }

// --- parsers ---

func mapPinState(v string) string {
	switch strings.ToUpper(v) {
	case "READY":
		return "ready"
	case "SIM PIN", "SIM PIN2":
		return "pin_required"
	case "NOT INSERTED":
		return "absent"
	default:
		return strings.ToLower(strings.ReplaceAll(v, " ", "_"))
	}
}

func parseCSQ(line string) int {
	// "+CSQ: 20,99"
	parts := strings.Split(strings.TrimPrefix(line, "+CSQ:"), ",")
	if len(parts) >= 1 {
		v, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		return v
	}
	return 0
}

func parseOperator(line string) string {
	// `+COPS: 0,0,"Helium",7`
	a := strings.IndexByte(line, '"')
	b := strings.LastIndexByte(line, '"')
	if a >= 0 && b > a {
		return line[a+1 : b]
	}
	return ""
}

func parseRegistered(line string) bool {
	// `+CREG: 0,1` — second field 1=home, 5=roaming
	parts := strings.Split(strings.TrimPrefix(line, "+CREG:"), ",")
	if len(parts) >= 2 {
		v, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		return v == 1 || v == 5
	}
	return false
}

func parseBattery(line string) float64 {
	// `+CBC: 3.95V`
	v := strings.TrimSpace(strings.TrimPrefix(line, "+CBC:"))
	v = strings.TrimSuffix(v, "V")
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f
}

// parseCPSI populates fields from `+CPSI: LTE,Online,310-260,...,EUTRAN-BAND2,...`
func parseCPSI(line string, n *NetworkStatus) {
	parts := strings.Split(strings.TrimPrefix(line, "+CPSI:"), ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) >= 1 {
		n.Tech = parts[0]
	}
	if len(parts) >= 7 && strings.HasPrefix(parts[6], "EUTRAN-") {
		n.Band = parts[6]
	}
	if len(parts) >= 12 {
		// SIM7600 reports RSRQ then RSRP in tenths of a dB.
		if v, err := strconv.Atoi(parts[10]); err == nil {
			n.RSRQdB = v / 10
		}
		if v, err := strconv.Atoi(parts[11]); err == nil {
			n.RSRPdBm = v / 10
		}
	}
}
