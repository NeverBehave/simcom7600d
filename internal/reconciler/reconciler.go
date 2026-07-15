// Package reconciler bridges modem-truth and DB-truth. It is the only place
// that owns the rule "modem is authoritative for live state, DB is
// authoritative for history and intents".
package reconciler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"sim7600d/internal/atexec"
	"sim7600d/internal/atproto"
	"sim7600d/internal/modem"
	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

type Reconciler struct {
	ex *atexec.Executor
	st *store.Store
	mo *modem.Facade

	mu      sync.Mutex
	lastRun time.Time
}

func New(ex *atexec.Executor, st *store.Store, mo *modem.Facade) *Reconciler {
	return &Reconciler{ex: ex, st: st, mo: mo}
}

// Boot runs the boot-time invariant sweeps and a full Reconcile.
func (r *Reconciler) Boot(ctx context.Context) error {
	if err := r.sweepStuck(ctx); err != nil {
		return err
	}
	return r.Reconcile(ctx)
}

// Reconcile reads CLCC + CMGL=4 + status fields and brings the DB into
// agreement with the modem.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	r.mu.Lock()
	r.lastRun = time.Now().UTC()
	r.mu.Unlock()

	if err := r.reconcileCalls(ctx); err != nil {
		return err
	}
	if err := r.reconcileSMS(ctx); err != nil {
		return err
	}
	// Refresh status snapshot so /v1/status is fresh.
	if _, err := r.mo.Status(ctx, true); err != nil {
		// non-fatal
		_, _ = r.st.AppendEvent(ctx, store.Event{Kind: "reconcile.warn", Raw: err.Error()})
	}
	return nil
}

func (r *Reconciler) reconcileCalls(ctx context.Context) error {
	resp, err := r.ex.Exec(ctx, atexec.Cmd("AT+CLCC"))
	if err != nil {
		return err
	}
	if resp.Final.Final != atproto.FinalOK {
		return fmt.Errorf("AT+CLCC: %s", resp.Final.Line)
	}
	live := parseCLCC(resp.Lines)
	open, err := r.st.ListOpenCalls(ctx)
	if err != nil {
		return err
	}
	// DB rows missing from CLCC -> ended.
	for _, c := range open {
		if !inCLCC(c, live) {
			if c.State == "dialing" && time.Since(c.StartedAt) < 3*time.Second {
				continue
			}
			_ = r.st.SetCallState(ctx, c.ID, store.CallUpdate{State: "ended", EndReason: "reconciled_missing", EndedAt: time.Now().UTC()})
			_, _ = r.st.AppendEvent(ctx, store.Event{Kind: "reconcile.diff", RefKind: "call", RefID: c.ID, Detail: `{"reason":"missing_in_clcc"}`})
		}
	}
	// CLCC rows missing from DB -> insert.
	for _, l := range live {
		if !inDB(l, open) {
			recent, err := r.st.HasRecentlyEndedCall(ctx, clccDirection(l.Dir), addPlus(l.Number), time.Now().UTC().Add(-15*time.Second))
			if err == nil && recent {
				continue
			}
			id := store.NewULID()
			c := store.Call{
				ID: id, Direction: clccDirection(l.Dir), RemoteAddr: addPlus(l.Number),
				State: clccState(l.State), StartedAt: time.Now().UTC(),
			}
			_ = r.st.InsertCall(ctx, c)
			_, _ = r.st.AppendEvent(ctx, store.Event{Kind: "reconcile.diff", RefKind: "call", RefID: id, Detail: fmt.Sprintf(`{"reason":"adopted","raw":%q}`, l.Raw)})
		}
	}
	return nil
}

func (r *Reconciler) reconcileSMS(ctx context.Context) error {
	// AT+CMGL=4 lists all messages in storage. The detailed parsing is shared
	// with the inbound workflow; here we just defer to the modem facade's
	// existing ingest by triggering AT+CMGR for any indices we don't recognise.
	resp, err := r.ex.Exec(ctx, atexec.Cmd("AT+CMGL=4").WithTimeout(5*time.Second))
	if err != nil {
		return nil // non-fatal — modem may be busy
	}
	for _, ln := range resp.Lines {
		// `+CMGL: <idx>,<stat>,...`
		if !strings.HasPrefix(ln, "+CMGL:") {
			continue
		}
		idx := parseCMGLIndex(ln)
		if idx <= 0 {
			continue
		}
		_, _ = r.st.AppendEvent(ctx, store.Event{Kind: "reconcile.cmgl", Raw: ln, Detail: fmt.Sprintf(`{"idx":%d}`, idx)})
		// Import directly: the original +CMTI may have been missed while the
		// daemon was down, and many modems do not replay it after reconnecting.
		if err := r.mo.IngestStorageIndex(ctx, idx); err != nil {
			_, _ = r.st.AppendEvent(ctx, store.Event{
				Kind: "sms.ingest_error", Raw: ln, Detail: err.Error(),
			})
		}
	}
	return nil
}

// sweepStuck marks "active"/"ringing"/"dialing" calls older than 1h as ended,
// and submitted SMS without an MR older than 5m as indeterminate.
func (r *Reconciler) sweepStuck(ctx context.Context) error {
	now := time.Now().UTC()
	_, err := r.st.DB().ExecContext(ctx, `
		UPDATE calls SET state='ended', end_reason='reconciled_missing', ended_at=?
		WHERE state IN ('ringing','dialing','alerting','active')
		  AND started_at < ?
	`, now.Format(time.RFC3339Nano), now.Add(-1*time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	_, err = r.st.DB().ExecContext(ctx, `
		UPDATE sms_outbound SET state='indeterminate', updated_at=?
		WHERE state='submitted' AND mrs='[]' AND created_at < ?
	`, now.Format(time.RFC3339Nano), now.Add(-5*time.Minute).Format(time.RFC3339Nano))
	return err
}

// --- CLCC helpers (duplicated narrowly with modem.calls; the alternative is
// to expose parser there, but keeping reconciler self-contained is simpler).

type clccRow struct {
	Idx, Dir, State int
	Number          string
	Raw             string
}

func parseCLCC(lines []string) []clccRow {
	var out []clccRow
	for _, ln := range lines {
		if !strings.HasPrefix(ln, "+CLCC:") {
			continue
		}
		fields := strings.Split(strings.TrimPrefix(ln, "+CLCC:"), ",")
		if len(fields) < 5 {
			continue
		}
		var r clccRow
		r.Raw = ln
		fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &r.Idx)
		fmt.Sscanf(strings.TrimSpace(fields[1]), "%d", &r.Dir)
		fmt.Sscanf(strings.TrimSpace(fields[2]), "%d", &r.State)
		if len(fields) >= 6 {
			r.Number = strings.Trim(strings.TrimSpace(fields[5]), `"`)
		}
		out = append(out, r)
	}
	return out
}

func parseCMGLIndex(ln string) int {
	rest := strings.TrimPrefix(ln, "+CMGL:")
	parts := strings.Split(rest, ",")
	if len(parts) == 0 {
		return 0
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &n)
	return n
}

func clccDirection(d int) string {
	if d == 1 {
		return "in"
	}
	return "out"
}

func clccState(s int) string {
	// 0=active 1=held 2=dialing 3=alerting 4=incoming 5=waiting
	switch s {
	case 0:
		return "active"
	case 2:
		return "dialing"
	case 3:
		return "alerting"
	case 4:
		return "ringing"
	}
	return "active"
}

func addPlus(num string) string {
	if num == "" || strings.HasPrefix(num, "+") {
		return num
	}
	return "+" + num
}

func inCLCC(c store.Call, live []clccRow) bool {
	for _, l := range live {
		if eqAddr(l.Number, c.RemoteAddr) {
			return true
		}
	}
	return false
}

func inDB(l clccRow, open []store.Call) bool {
	for _, c := range open {
		if eqAddr(l.Number, c.RemoteAddr) {
			return true
		}
	}
	return false
}

func eqAddr(a, b string) bool {
	return strings.TrimPrefix(a, "+") == strings.TrimPrefix(b, "+")
}

// urcBusLike is the small subset of *urc.Bus used here. Defined as a local
// interface to avoid an import cycle (the wiring layer constructs us with
// the real bus).
type urcBusLike interface {
	Subscribe(prefix string) <-chan urc.Event
}

// RunForever subscribes to reset signals on the URC bus and drives a periodic
// Reconcile. It returns when ctx is canceled.
func (r *Reconciler) RunForever(ctx context.Context, bus urcBusLike) {
	rdy := bus.Subscribe("RDY")
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-rdy:
			_ = r.HandleReset(ctx, "RDY", ev)
		case <-tick.C:
			_ = r.Reconcile(ctx)
		}
	}
}
