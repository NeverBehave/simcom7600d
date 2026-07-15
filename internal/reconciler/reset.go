package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

// HandleReset is invoked when any of the modem-reset signals fire (transport
// reopen, RDY URC at runtime, +CPIN: READY when already READY).
//
// It bumps the epoch, marks all open calls ended (the modem doesn't know about
// them anymore), and runs a full Reconcile.
func (r *Reconciler) HandleReset(ctx context.Context, signal string, ev urc.Event) error {
	epoch, err := r.bumpEpoch(ctx)
	if err != nil {
		return err
	}
	_, _ = r.st.AppendEvent(ctx, store.Event{
		Kind: "modem.reset", Raw: signal,
		Detail: fmt.Sprintf(`{"epoch":%d}`, epoch),
	})
	// All previously-open calls are gone from the modem's POV.
	open, err := r.st.ListOpenCalls(ctx)
	if err == nil {
		for _, c := range open {
			_ = r.st.SetCallState(ctx, c.ID, store.CallUpdate{
				State: "ended", EndReason: "modem_reset", EndedAt: time.Now().UTC(),
			})
		}
	}
	// Re-run baseline config + reconcile.
	if err := r.mo.Boot(ctx); err != nil {
		return err
	}
	return r.Reconcile(ctx)
}

func (r *Reconciler) bumpEpoch(ctx context.Context) (int64, error) {
	current := int64(0)
	if v, ok, _ := r.st.GetKV(ctx, "epoch"); ok {
		var obj struct {
			N int64 `json:"n"`
		}
		if err := json.Unmarshal([]byte(v), &obj); err == nil {
			current = obj.N
		} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			current = n
		}
	}
	current++
	_ = r.st.PutKV(ctx, "epoch", fmt.Sprintf(`{"n":%d}`, current))
	return current, nil
}
