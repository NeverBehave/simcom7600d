package modem

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sim7600d/internal/atexec"
	"sim7600d/internal/atproto"
	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

const clccMissingPollsBeforeEnd = 3

func (f *Facade) startCallWorkflowImpl() {
	clip := f.bus.Subscribe("+CLIP:")
	ring := f.bus.Subscribe("RING")
	cring := f.bus.Subscribe("+CRING:")
	f.wg.Add(1)
	go func() { defer f.wg.Done(); f.callURCLoop(clip, ring, cring) }()
	f.wg.Add(1)
	go func() { defer f.wg.Done(); f.clccPollLoop() }()
}

func (f *Facade) callURCLoop(clip, ring, cring <-chan urc.Event) {
	for {
		select {
		case <-f.stopped:
			return
		case e, ok := <-clip:
			if !ok {
				clip = nil
				continue
			}
			from := parseCLIPNumber(e.Line)
			if from != "" {
				f.handleIncomingCall(from)
			}
		case _, ok := <-ring:
			if !ok {
				ring = nil
				continue
			}
			f.discoverIncomingCalls()
		case _, ok := <-cring:
			if !ok {
				cring = nil
				continue
			}
			f.discoverIncomingCalls()
		}
	}
}

func (f *Facade) handleIncomingCall(from string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := f.openInbound(ctx, from)
	if err != nil {
		return
	}
	select {
	case f.incomingCalls <- IncomingCall{ID: id, From: from}:
	default:
	}
}

// discoverIncomingCalls covers firmware/network combinations that emit a
// generic RING/+CRING indication before +CLIP, or omit caller ID altogether.
func (f *Facade) discoverIncomingCalls() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := f.ex.Exec(ctx, atexec.Cmd("AT+CLCC").WithTimeout(3*time.Second))
	if err != nil || resp.Final.Final != atproto.FinalOK {
		return
	}
	for _, row := range parseCLCC(resp.Lines) {
		if row.Dir == 1 && (row.State == 4 || row.State == 5) {
			f.handleIncomingCall(row.Number)
		}
	}
}

// pollCLCC is the authoritative mechanism for detecting that a previously-open
// call has ended. NO CARRIER URC routing is intentionally NOT used because
// atproto classifies NO CARRIER as KindFinal, not KindURC.
func (f *Facade) clccPollLoop() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-f.stopped:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			f.pollCLCC(ctx)
			cancel()
		}
	}
}

func (f *Facade) pollCLCC(ctx context.Context) {
	open, err := f.st.ListOpenCalls(ctx)
	if err != nil || len(open) == 0 {
		return
	}
	resp, err := f.ex.Exec(ctx, atexec.Cmd("AT+CLCC"))
	if err != nil || resp.Final.Final != atproto.FinalOK {
		return
	}
	live := parseCLCC(resp.Lines)
	// Reconcile both presence and state. CLCC is the modem's authoritative view.
	for _, c := range open {
		if row, ok := matchInCLCC(c, live); ok {
			f.clearCLCCMissing(c.ID)
			state := clccState(row.State)
			if state != "" && state != c.State {
				u := store.CallUpdate{State: state}
				if state == "active" && c.AnsweredAt.IsZero() {
					u.AnsweredAt = time.Now().UTC()
				}
				if f.st.SetCallState(ctx, c.ID, u) == nil {
					appendCallEvent(ctx, f.st, "call.updated", c.ID, state, "")
				}
			}
			continue
		}
		// The modem can take a moment to expose a newly accepted ATD call.
		if c.State == "dialing" && time.Since(c.StartedAt) < 3*time.Second {
			f.clearCLCCMissing(c.ID)
			continue
		}
		if f.noteCLCCMissing(c.ID) < clccMissingPollsBeforeEnd {
			continue
		}
		{
			end := time.Now().UTC()
			dur := 0
			if !c.AnsweredAt.IsZero() {
				dur = int(end.Sub(c.AnsweredAt).Milliseconds())
			}
			_ = f.st.SetCallState(ctx, c.ID, store.CallUpdate{State: "ended", EndReason: "reconciled_missing", EndedAt: end, DurationMS: dur})
			appendCallEvent(ctx, f.st, "call.ended", c.ID, "ended", "reconciled_missing")
			f.clearCLCCMissing(c.ID)
		}
	}
	// For each CLCC entry: if no matching open row, log it (the URC dispatcher should have created one; this is a safety net).
	for _, l := range live {
		if !matchedAny(l, open) {
			_, _ = f.st.AppendEvent(ctx, store.Event{
				Kind: "reconcile.diff", Detail: fmt.Sprintf(`{"clcc":%q}`, l.Raw),
			})
		}
	}
}

func (f *Facade) openInbound(ctx context.Context, from string) (string, error) {
	f.callMu.Lock()
	defer f.callMu.Unlock()
	// Dedupe repeated RING/+CRING/+CLIP indications. If RING arrived before
	// caller ID, fill the existing row rather than creating a second call.
	open, _ := f.st.ListOpenCalls(ctx)
	for _, c := range open {
		if c.Direction != "in" || c.State != "ringing" {
			continue
		}
		if samePhoneNumber(c.RemoteAddr, from) {
			return c.ID, nil
		}
		if c.RemoteAddr == "" && from != "" {
			if err := f.st.SetCallRemoteAddr(ctx, c.ID, from); err != nil {
				return "", err
			}
			return c.ID, nil
		}
	}
	id := store.NewULID()
	c := store.Call{
		ID: id, Direction: "in", RemoteAddr: from,
		State: "ringing", StartedAt: time.Now().UTC(),
	}
	if err := f.st.InsertCall(ctx, c); err != nil {
		return "", err
	}
	detail, _ := json.Marshal(map[string]string{"from": from})
	_, _ = f.st.AppendEvent(ctx, store.Event{
		Kind: "call.ringing", RefKind: "call", RefID: id, Raw: from, Detail: string(detail),
	})
	return id, nil
}

func (f *Facade) markOpenCallsEnded(ctx context.Context, reason, raw string) {
	open, _ := f.st.ListOpenCalls(ctx)
	end := time.Now().UTC()
	for _, c := range open {
		dur := 0
		if !c.AnsweredAt.IsZero() {
			dur = int(end.Sub(c.AnsweredAt).Milliseconds())
		}
		state := "ended"
		if c.State == "ringing" {
			state = "missed"
		}
		_ = f.st.SetCallState(ctx, c.ID, store.CallUpdate{State: state, EndReason: reason, EndedAt: end, DurationMS: dur})
		_, _ = f.st.AppendEvent(ctx, store.Event{
			Kind: "call.ended", RefKind: "call", RefID: c.ID, Raw: raw,
		})
	}
}

type clccRow struct {
	Idx, Dir, State int
	Number          string
	Raw             string
}

func parseCLIPNumber(line string) string {
	// `+CLIP: "+15551234567",145,...`
	a := strings.IndexByte(line, '"')
	if a < 0 {
		return ""
	}
	b := strings.IndexByte(line[a+1:], '"')
	if b < 0 {
		return ""
	}
	return line[a+1 : a+1+b]
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

func matchedInCLCC(c store.Call, live []clccRow) bool {
	_, ok := matchInCLCC(c, live)
	return ok
}

func matchInCLCC(c store.Call, live []clccRow) (clccRow, bool) {
	direction := 0
	if c.Direction == "in" {
		direction = 1
	}
	var fallback clccRow
	foundFallback := false
	for _, l := range live {
		if l.Dir != direction || (c.RemoteAddr != "" && !samePhoneNumber(l.Number, c.RemoteAddr)) {
			continue
		}
		if clccState(l.State) == c.State {
			return l, true
		}
		if !foundFallback {
			fallback = l
			foundFallback = true
		}
	}
	return fallback, foundFallback
}

func clccState(state int) string {
	switch state {
	case 0:
		return "active"
	case 1:
		return "held"
	case 2:
		return "dialing"
	case 3:
		return "alerting"
	case 4, 5:
		return "ringing"
	default:
		return ""
	}
}

func appendCallEvent(ctx context.Context, st *store.Store, kind, id, state, reason string) {
	_, _ = st.AppendEvent(ctx, store.Event{
		Kind: kind, RefKind: "call", RefID: id,
		Detail: fmt.Sprintf(`{"state":%q,"reason":%q}`, state, reason),
	})
}

func matchedAny(l clccRow, open []store.Call) bool {
	for _, c := range open {
		direction := 0
		if c.Direction == "in" {
			direction = 1
		}
		if l.Dir == direction && (c.RemoteAddr == "" || samePhoneNumber(l.Number, c.RemoteAddr)) {
			return true
		}
	}
	return false
}

func samePhoneNumber(a, b string) bool {
	return strings.TrimPrefix(a, "+") == strings.TrimPrefix(b, "+")
}

func (f *Facade) noteCLCCMissing(callID string) int {
	f.callMu.Lock()
	defer f.callMu.Unlock()
	f.clccMissing[callID]++
	return f.clccMissing[callID]
}

func (f *Facade) clearCLCCMissing(callID string) {
	f.callMu.Lock()
	delete(f.clccMissing, callID)
	f.callMu.Unlock()
}
