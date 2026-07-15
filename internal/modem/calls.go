package modem

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sim7600d/internal/atexec"
	"sim7600d/internal/atproto"
	"sim7600d/internal/store"
)

var ErrInvalidDTMF = errors.New("modem: invalid DTMF digits")
var ErrCallNotFound = errors.New("modem: call not found")
var ErrCallAudioUnavailable = errors.New("modem: call audio unavailable")
var ErrCallsNotMergeable = errors.New("modem: an active and a held call are required")

func (f *Facade) Dial(ctx context.Context, to, idem string) (Call, error) {
	if idem != "" {
		if rec, ok, err := f.st.GetIdem(ctx, idem); err == nil && ok && rec.Scope == "call" {
			if existing, err := f.st.GetCall(ctx, rec.RefID); err == nil {
				return toCall(existing), nil
			}
		}
	}
	id := store.NewULID()
	now := time.Now().UTC()
	c := store.Call{
		ID: id, Direction: "out", RemoteAddr: to,
		State: "dialing", StartedAt: now, IdemKey: idem,
	}
	if err := f.st.InsertCall(ctx, c); err != nil {
		if errors.Is(err, store.ErrDuplicate) && idem != "" {
			// look up by idem, return existing
			rec, ok, _ := f.st.GetIdem(ctx, idem)
			if ok {
				if existing, err := f.st.GetCall(ctx, rec.RefID); err == nil {
					return toCall(existing), nil
				}
			}
		}
		return Call{}, err
	}
	if idem != "" {
		_ = f.st.PutIdem(ctx, store.IdemRecord{
			Key: idem, Scope: "call", RefID: id, Status: 201, CreatedAt: now,
		})
	}
	appendCallEvent(ctx, f.st, "call.dialing", id, "dialing", "")
	cmd := fmt.Sprintf("ATD%s;", to)
	resp, err := f.ex.Exec(ctx, atexec.Cmd(cmd).WithTimeout(90*time.Second))
	if err != nil {
		_ = f.st.SetCallState(ctx, id, store.CallUpdate{State: "ended", EndReason: "error", ErrorDetail: err.Error(), EndedAt: time.Now().UTC()})
		appendCallEvent(ctx, f.st, "call.ended", id, "ended", "error")
		return Call{}, err
	}
	switch resp.Final.Final {
	case atproto.FinalOK:
		// state stays "dialing" until +CLCC poll updates it
	case atproto.FinalNoCarrier, atproto.FinalBusy, atproto.FinalNoAnswer:
		reason := finalToReason(resp.Final.Final)
		_ = f.st.SetCallState(ctx, id, store.CallUpdate{State: "ended", EndReason: reason, EndedAt: time.Now().UTC()})
		appendCallEvent(ctx, f.st, "call.ended", id, "ended", reason)
	default:
		_ = f.st.SetCallState(ctx, id, store.CallUpdate{State: "ended", EndReason: "error", ErrorCode: resp.Final.Line, EndedAt: time.Now().UTC()})
		appendCallEvent(ctx, f.st, "call.ended", id, "ended", "error")
		return Call{}, fmt.Errorf("dial: %s", resp.Final.Line)
	}
	got, _ := f.st.GetCall(ctx, id)
	return toCall(got), nil
}

func (f *Facade) Hangup(ctx context.Context, callID string) error {
	c, err := f.st.GetCall(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	if err := f.releaseCall(ctx, c, "hangup"); err != nil {
		return err
	}
	end := time.Now().UTC()
	dur := 0
	if !c.AnsweredAt.IsZero() {
		dur = int(end.Sub(c.AnsweredAt).Milliseconds())
	}
	if err := f.st.SetCallState(ctx, callID, store.CallUpdate{State: "ended", EndReason: "hangup", EndedAt: end, DurationMS: dur}); err != nil {
		return err
	}
	appendCallEvent(ctx, f.st, "call.ended", callID, "ended", "hangup")
	f.clearCLCCMissing(callID)
	return nil
}

func (f *Facade) Answer(ctx context.Context, callID string) error {
	c, err := f.st.GetCall(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	if c.State != "ringing" {
		return fmt.Errorf("call not ringing (state=%s)", c.State)
	}
	resp, err := f.ex.Exec(ctx, atexec.Cmd("ATA").WithTimeout(5*time.Second))
	if err != nil {
		return err
	}
	if resp.Final.Final != atproto.FinalOK {
		return fmt.Errorf("answer: %s", resp.Final.Line)
	}
	if err := f.st.SetCallState(ctx, callID, store.CallUpdate{State: "active", AnsweredAt: time.Now().UTC()}); err != nil {
		return err
	}
	appendCallEvent(ctx, f.st, "call.updated", callID, "active", "")
	return nil
}

func (f *Facade) Reject(ctx context.Context, callID string) error {
	c, err := f.st.GetCall(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	if c.State != "ringing" {
		return fmt.Errorf("call not ringing (state=%s)", c.State)
	}
	if err := f.releaseCall(ctx, c, "reject"); err != nil {
		return err
	}
	if err := f.st.SetCallState(ctx, callID, store.CallUpdate{State: "rejected", EndReason: "rejected", EndedAt: time.Now().UTC()}); err != nil {
		return err
	}
	appendCallEvent(ctx, f.st, "call.ended", callID, "rejected", "rejected")
	return nil
}

// releaseCall targets a modem call index whenever another call is open. The
// broad AT+CHUP command is retained for the single-call case because this
// firmware handles it more consistently than ATH. With multiple calls,
// AT+CHLD=1X prevents hanging up or disturbing a held peer call.
func (f *Facade) releaseCall(ctx context.Context, c store.Call, action string) error {
	open, err := f.st.ListOpenCalls(ctx)
	if err != nil {
		return err
	}
	cmd := "AT+CHUP"
	if len(open) > 1 {
		snapshot, err := f.ex.Exec(ctx, atexec.Cmd("AT+CLCC").WithTimeout(5*time.Second))
		if err != nil {
			return err
		}
		if snapshot.Final.Final != atproto.FinalOK {
			return fmt.Errorf("%s: could not identify modem call: %s", action, snapshot.Final.Line)
		}
		row, ok := matchInCLCC(c, parseCLCC(snapshot.Lines))
		if !ok {
			return fmt.Errorf("%s: selected call is not present in modem call list", action)
		}
		cmd = fmt.Sprintf("AT+CHLD=1%d", row.Idx)
	}
	resp, err := f.ex.Exec(ctx, atexec.Cmd(cmd).WithTimeout(15*time.Second))
	if err != nil {
		return err
	}
	if resp.Final.Final != atproto.FinalOK {
		return fmt.Errorf("%s: %s", action, resp.Final.Line)
	}
	return nil
}

// Hold places the active voice call on network hold. SIM7600 implements both
// sides of the single-call hold toggle with AT+CHLD=2: for an active call it
// holds, and for a held call it retrieves that call.
func (f *Facade) Hold(ctx context.Context, callID string) error {
	return f.changeHoldState(ctx, callID, "active", "held")
}

// Resume retrieves a held voice call.
func (f *Facade) Resume(ctx context.Context, callID string) error {
	return f.changeHoldState(ctx, callID, "held", "active")
}

// MergeCalls adds the held call to the active call, creating a network
// multiparty conversation. The live firmware advertises CHLD operation 3.
func (f *Facade) MergeCalls(ctx context.Context, callID string) error {
	c, err := f.st.GetCall(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	if c.State != "active" && c.State != "held" {
		return ErrCallsNotMergeable
	}
	open, err := f.st.ListOpenCalls(ctx)
	if err != nil {
		return err
	}
	hasActive, hasHeld := false, false
	for _, candidate := range open {
		hasActive = hasActive || candidate.State == "active"
		hasHeld = hasHeld || candidate.State == "held"
	}
	if !hasActive || !hasHeld {
		return ErrCallsNotMergeable
	}
	resp, err := f.ex.Exec(ctx, atexec.Cmd("AT+CHLD=3").WithTimeout(15*time.Second))
	if err != nil {
		return err
	}
	if resp.Final.Final != atproto.FinalOK {
		return fmt.Errorf("merge calls: %s", resp.Final.Line)
	}
	for _, candidate := range open {
		if candidate.State != "active" && candidate.State != "held" {
			continue
		}
		update := store.CallUpdate{State: "active"}
		if candidate.AnsweredAt.IsZero() {
			update.AnsweredAt = time.Now().UTC()
		}
		if f.st.SetCallState(ctx, candidate.ID, update) == nil {
			appendCallEvent(ctx, f.st, "call.updated", candidate.ID, "active", "conference")
		}
	}
	return nil
}

func (f *Facade) changeHoldState(ctx context.Context, callID, from, to string) error {
	c, err := f.st.GetCall(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	if c.State != from {
		return fmt.Errorf("call not %s (state=%s)", from, c.State)
	}
	resp, err := f.ex.Exec(ctx, atexec.Cmd("AT+CHLD=2").WithTimeout(15*time.Second))
	if err != nil {
		return err
	}
	if resp.Final.Final != atproto.FinalOK {
		return fmt.Errorf("%s call: %s", to, resp.Final.Line)
	}
	if err := f.st.SetCallState(ctx, callID, store.CallUpdate{State: to}); err != nil {
		return err
	}
	appendCallEvent(ctx, f.st, "call.updated", callID, to, "")
	return nil
}

func (f *Facade) SendDTMF(ctx context.Context, callID, digits string, durMS int) error {
	for _, r := range digits {
		if !isDTMF(r) {
			return ErrInvalidDTMF
		}
	}
	c, err := f.st.GetCall(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	if c.State != "active" {
		return fmt.Errorf("call not active (state=%s)", c.State)
	}
	if durMS < 100 || durMS > 1000 {
		durMS = 200
	}
	// SIM7600 AT+VTS expresses per-digit duration in tenths of a second.
	// Round up so a requested tone is never shorter than the caller expects.
	durationTenths := (durMS + 99) / 100
	for _, r := range digits {
		cmd := fmt.Sprintf("AT+VTS=%c,%d", r, durationTenths)
		resp, err := f.ex.Exec(ctx, atexec.Cmd(cmd).WithTimeout(2*time.Second))
		if err != nil {
			return err
		}
		if resp.Final.Final != atproto.FinalOK {
			return fmt.Errorf("dtmf: %s", resp.Final.Line)
		}
	}
	return nil
}

// StartCallAudio asks the modem to begin exchanging 16 kHz, signed 16-bit,
// mono PCM on its USB Audio port. The raw audio device itself is owned by the
// callaudio bridge; AT commands remain serialized through the executor here.
func (f *Facade) StartCallAudio(ctx context.Context, callID string) error {
	c, err := f.st.GetCall(ctx, callID)
	if err != nil {
		return ErrCallNotFound
	}
	if !canAttachCallAudio(toCall(c)) {
		return fmt.Errorf("%w (state=%s)", ErrCallAudioUnavailable, c.State)
	}
	// CPCMFRM is volatile and returns to 8 kHz whenever the modem resets. Apply
	// the documented SIM7500/7600 16 kHz setting immediately before enabling
	// USB PCM so the modem and browser never stream at mismatched rates.
	for _, cmd := range []string{"AT+CPCMFRM=1", "AT+CPCMBANDWIDTH=0,1", "AT+CPCMREG=1"} {
		resp, err := f.ex.Exec(ctx, atexec.Cmd(cmd).WithTimeout(5*time.Second))
		if err != nil {
			return err
		}
		if resp.Final.Final != atproto.FinalOK {
			return fmt.Errorf("start call audio (%s): %s", cmd, resp.Final.Line)
		}
	}
	return nil
}

func canAttachCallAudio(call Call) bool {
	if call.State == "active" || call.State == "held" {
		return true
	}
	return call.Direction == "out" && (call.State == "dialing" || call.State == "alerting")
}

// StopCallAudio stops USB PCM transfer without changing the cellular call.
// It is intentionally best-effort at call teardown because the modem may
// already have ended the voice session before the browser disconnects.
func (f *Facade) StopCallAudio(ctx context.Context, callID string) error {
	if _, err := f.st.GetCall(ctx, callID); err != nil {
		return ErrCallNotFound
	}
	// The second parameter asks the firmware to tear down the USB transfer
	// immediately. SIM7600G-H V2 rejects the shorter CPCMREG=0 form even
	// though older application notes show it.
	resp, err := f.ex.Exec(ctx, atexec.Cmd("AT+CPCMREG=0,1").WithTimeout(5*time.Second))
	if err != nil {
		return err
	}
	if resp.Final.Final != atproto.FinalOK {
		return fmt.Errorf("stop call audio: %s", resp.Final.Line)
	}
	return nil
}

func (f *Facade) GetCall(ctx context.Context, id string) (Call, error) {
	c, err := f.st.GetCall(ctx, id)
	if err != nil {
		return Call{}, err
	}
	return toCall(c), nil
}

func (f *Facade) ListCalls(ctx context.Context, since string, limit int) ([]Call, error) {
	// Simple: read recent calls. Pagination details happen at the API layer.
	rows, err := f.st.DB().QueryContext(ctx, `
		SELECT id FROM calls WHERE id > ? ORDER BY started_at DESC, id DESC LIMIT ?
	`, since, defaultLimit(limit))
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var out []Call
	for _, id := range ids {
		c, err := f.st.GetCall(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, toCall(c))
	}
	return out, nil
}

func defaultLimit(n int) int {
	if n <= 0 || n > 500 {
		return 50
	}
	return n
}

func isDTMF(r rune) bool {
	return (r >= '0' && r <= '9') || r == '*' || r == '#' || (r >= 'A' && r <= 'D')
}

func finalToReason(f atproto.FinalCode) string {
	switch f {
	case atproto.FinalNoCarrier:
		return "no_carrier"
	case atproto.FinalBusy:
		return "busy"
	case atproto.FinalNoAnswer:
		return "no_answer"
	}
	return "error"
}

func toCall(c store.Call) Call {
	return Call{
		ID: c.ID, Direction: c.Direction, RemoteAddr: c.RemoteAddr,
		State: c.State, EndReason: c.EndReason,
		StartedAt: c.StartedAt, AnsweredAt: c.AnsweredAt, EndedAt: c.EndedAt,
		DurationMS: c.DurationMS,
	}
}
