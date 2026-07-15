package modem

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sim7600d/internal/atexec"
	"sim7600d/internal/sms"
	"sim7600d/internal/store"
	"sim7600d/internal/urc"
)

// startInboundWorkflow subscribes to +CMTI URCs and processes each one.
func (f *Facade) startInboundWorkflow() {
	cmti := f.bus.Subscribe("+CMTI:")
	f.wg.Add(1)
	go func() { defer f.wg.Done(); f.inboundLoop(cmti) }()
}

func (f *Facade) inboundLoop(events <-chan urc.Event) {
	for {
		select {
		case <-f.stopped:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			idx := parseCMTIIndex(ev.Line)
			if idx <= 0 {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := f.ingestStorageIndex(ctx, idx); err != nil {
				_, _ = f.st.AppendEvent(ctx, store.Event{
					Kind: "sms.ingest_error", Raw: ev.Line, Detail: err.Error(),
				})
			}
			cancel()
		}
	}
}

func (f *Facade) ingestStorageIndex(ctx context.Context, idx int) error {
	resp, err := f.ex.Exec(ctx, atexec.Cmd(fmt.Sprintf("AT+CMGR=%d", idx)).WithTimeout(5*time.Second))
	if err != nil {
		return err
	}
	hexPDU := extractCMGRHex(resp.Lines)
	if hexPDU == "" {
		return fmt.Errorf("no PDU in CMGR response for idx=%d", idx)
	}
	d, err := sms.DecodeModemPDU(hexPDU)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if d.UDH != nil {
		return f.handleConcatPart(ctx, idx, d, hexPDU)
	}
	return f.commitInbound(ctx, idx, d, []string{hexPDU})
}

// IngestStorageIndex imports one SMS already present in modem storage. It is
// used by boot-time reconciliation when the daemon missed the original CMTI.
func (f *Facade) IngestStorageIndex(ctx context.Context, idx int) error {
	return f.ingestStorageIndex(ctx, idx)
}

func (f *Facade) handleConcatPart(ctx context.Context, storageIdx int, d sms.Delivered, hexPDU string) error {
	if err := f.st.PutPart(ctx, store.InboundPart{
		Ref: d.UDH.Ref, Total: d.UDH.Total, Seq: d.UDH.Seq,
		FromAddr: d.FromAddr, SMSCTime: d.SMSCTime,
		Body: d.Body, Encoding: d.Encoding, RawPDU: hexPDU,
		ReceivedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	parts, err := f.st.GetPartsForReassembly(ctx, d.FromAddr, d.UDH.Ref)
	if err != nil {
		return err
	}
	if len(parts) < d.UDH.Total {
		// not yet complete; leave the modem slot, will be cleaned by sweep
		_, _ = f.ex.Exec(ctx, atexec.Cmd(fmt.Sprintf("AT+CMGD=%d", storageIdx)))
		return nil
	}
	var sb strings.Builder
	rawPDUs := make([]string, len(parts))
	for i, p := range parts {
		sb.WriteString(p.Body)
		rawPDUs[i] = p.RawPDU
	}
	merged := sms.Delivered{
		FromAddr: d.FromAddr, Body: sb.String(),
		Encoding: d.Encoding, SMSCTime: d.SMSCTime,
	}
	if err := f.commitInbound(ctx, storageIdx, merged, rawPDUs); err != nil {
		return err
	}
	return f.st.DeleteParts(ctx, d.FromAddr, d.UDH.Ref)
}

func (f *Facade) commitInbound(ctx context.Context, storageIdx int, d sms.Delivered, rawPDUs []string) error {
	id := store.NewULID()
	in := store.Inbound{
		ID: id, FromAddr: d.FromAddr, Body: d.Body, Encoding: d.Encoding,
		Parts: len(rawPDUs), SMSCTime: d.SMSCTime,
		ReceivedAt: time.Now().UTC(),
		DedupeKey:  sms.DedupeKey(d.FromAddr, d.SMSCTime, d.Body),
		RawPDUs:    jsonStrings(rawPDUs),
	}
	if err := f.st.InsertInbound(ctx, in); err != nil {
		// duplicates aren't a real error here — just delete the modem slot
		if err == store.ErrDuplicate {
			_, _ = f.ex.Exec(ctx, atexec.Cmd(fmt.Sprintf("AT+CMGD=%d", storageIdx)))
			return nil
		}
		return err
	}
	detail, _ := json.Marshal(map[string]string{"from": d.FromAddr, "body": d.Body})
	_, _ = f.st.AppendEvent(ctx, store.Event{
		Kind: "sms.arrived", RefKind: "sms", RefID: id, Raw: fmt.Sprintf("+CMTI:\"ME\",%d", storageIdx),
		Detail: string(detail),
	})
	_, _ = f.ex.Exec(ctx, atexec.Cmd(fmt.Sprintf("AT+CMGD=%d", storageIdx)))

	select {
	case f.inboundSMS <- SMSArrived{ID: id, From: d.FromAddr, Body: d.Body}:
	default:
	}
	return nil
}

func parseCMTIIndex(line string) int {
	// `+CMTI: "ME",5`
	i := strings.LastIndexByte(line, ',')
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(line[i+1:]))
	return n
}

func extractCMGRHex(lines []string) string {
	for i, ln := range lines {
		if strings.HasPrefix(ln, "+CMGR:") && i+1 < len(lines) {
			cand := strings.TrimSpace(lines[i+1])
			if isHex(cand) {
				return cand
			}
		}
	}
	// fallback: last hex-looking line
	for i := len(lines) - 1; i >= 0; i-- {
		ln := strings.TrimSpace(lines[i])
		if isHex(ln) {
			return ln
		}
	}
	return ""
}

func isHex(s string) bool {
	if len(s) < 4 || len(s)%2 != 0 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func jsonStrings(xs []string) string {
	if len(xs) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range xs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(s)
		b.WriteByte('"')
	}
	b.WriteByte(']')
	return b.String()
}
