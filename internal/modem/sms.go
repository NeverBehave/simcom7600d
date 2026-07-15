package modem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"sim7600d/internal/atexec"
	"sim7600d/internal/atproto"
	"sim7600d/internal/sms"
	"sim7600d/internal/store"
)

func (f *Facade) SendSMS(ctx context.Context, to, body string, opts SendOpts) (Outbound, error) {
	if opts.IdemKey != "" {
		if rec, ok, err := f.st.GetIdem(ctx, opts.IdemKey); err == nil && ok && rec.Scope == "sms" {
			if existing, err := f.st.GetOutbound(ctx, rec.RefID); err == nil {
				return toOutbound(existing), nil
			}
		}
	}

	parts, err := sms.Encode(to, body, sms.EncodeOptions{StatusReport: opts.DeliveryReport})
	if err != nil {
		return Outbound{}, fmt.Errorf("encode: %w", err)
	}

	id := store.NewULID()
	now := time.Now().UTC()
	row := store.Outbound{
		ID: id, ToAddr: to, Body: body, Encoding: parts[0].Encoding,
		Parts: len(parts), State: "queued",
		DeliveryReport: opts.DeliveryReport,
		IdemKey:        opts.IdemKey,
		CreatedAt:      now, UpdatedAt: now,
	}
	if err := f.st.InsertOutbound(ctx, row); err != nil {
		if errors.Is(err, store.ErrDuplicate) && opts.IdemKey != "" {
			ex, _ := f.st.GetOutboundByIdem(ctx, opts.IdemKey)
			return toOutbound(ex), nil
		}
		return Outbound{}, err
	}

	var mrs []int
	for _, p := range parts {
		mr, err := f.sendOnePart(ctx, p)
		if err != nil {
			_ = f.st.SetOutboundState(ctx, id, "failed", mrs, "atexec", err.Error())
			return Outbound{}, err
		}
		mrs = append(mrs, mr)
	}
	state := "submitted"
	if err := f.st.SetOutboundState(ctx, id, state, mrs, "", ""); err != nil {
		return Outbound{}, err
	}

	out := Outbound{
		ID: id, To: to, Body: body, State: state,
		Encoding: parts[0].Encoding, CreatedAt: now,
	}
	for _, mr := range mrs {
		out.Parts = append(out.Parts, OutboundPart{MR: mr})
	}

	if opts.IdemKey != "" {
		_ = f.st.PutIdem(ctx, store.IdemRecord{
			Key: opts.IdemKey, Scope: "sms", RefID: id,
			Response: outboundJSON(out), Status: 202, CreatedAt: now,
		})
	}
	return out, nil
}

// sendOnePart runs the AT+CMGS prompt sequence for a single TPDU. Returns mr.
func (f *Facade) sendOnePart(ctx context.Context, p sms.Part) (int, error) {
	line := fmt.Sprintf("AT+CMGS=%d", p.TPDULen)
	body := p.HexPDU
	req := atexec.Request{
		Line:    line,
		Timeout: 30 * time.Second,
		Run: func(w io.Writer, frames func() (atproto.Frame, error)) (atexec.Response, error) {
			if _, err := w.Write([]byte(line + "\r\n")); err != nil {
				return atexec.Response{}, err
			}
			for {
				fr, err := frames()
				if err != nil {
					return atexec.Response{}, err
				}
				switch fr.Kind {
				case atproto.KindPrompt:
					goto sendBody
				case atproto.KindFinal:
					return atexec.Response{Final: fr}, nil
				}
			}
		sendBody:
			if _, err := w.Write([]byte(body)); err != nil {
				return atexec.Response{}, err
			}
			if _, err := w.Write([]byte{0x1A}); err != nil {
				return atexec.Response{}, err
			}
			var resp atexec.Response
			for {
				fr, err := frames()
				if err != nil {
					return resp, err
				}
				if fr.Kind == atproto.KindIntermediate {
					resp.Lines = append(resp.Lines, fr.Line)
					continue
				}
				if fr.Kind == atproto.KindFinal {
					resp.Final = fr
					return resp, nil
				}
			}
		},
	}
	resp, err := f.ex.Exec(ctx, req)
	if err != nil {
		return 0, err
	}
	switch resp.Final.Final {
	case atproto.FinalOK:
		return parseCMGS(resp.Lines)
	case atproto.FinalCMSError:
		return 0, fmt.Errorf("CMS ERROR %d (%s)", resp.Final.Code, atproto.CMSMeaning(resp.Final.Code))
	case atproto.FinalCMEError:
		return 0, fmt.Errorf("CME ERROR %d (%s)", resp.Final.Code, atproto.CMEMeaning(resp.Final.Code))
	default:
		return 0, fmt.Errorf("unexpected final: %s", resp.Final.Line)
	}
}

func parseCMGS(lines []string) (int, error) {
	for _, l := range lines {
		if len(l) > 7 && l[:7] == "+CMGS: " {
			var n int
			_, err := fmt.Sscanf(l[7:], "%d", &n)
			return n, err
		}
	}
	return 0, errors.New("no +CMGS line in response")
}

// GetInbound retrieves a single inbound SMS by ID.
func (f *Facade) GetInbound(ctx context.Context, id string) (Inbound, error) {
	in, err := f.st.GetInbound(ctx, id)
	if err != nil {
		return Inbound{}, err
	}
	return toInbound(in), nil
}

// DeleteSMS removes an SMS record (inbound or outbound) by ID. Returns nil if
// the record does not exist (idempotent). An error is only returned on DB failure.
func (f *Facade) DeleteSMS(ctx context.Context, id string) error {
	if err := f.st.DeleteInbound(ctx, id); err != nil {
		return err
	}
	return f.st.DeleteOutbound(ctx, id)
}

// GetOutbound retrieves a single outbound SMS by ID.
func (f *Facade) GetOutbound(ctx context.Context, id string) (Outbound, error) {
	o, err := f.st.GetOutbound(ctx, id)
	if err != nil {
		return Outbound{}, err
	}
	return toOutbound(o), nil
}

// ListInbound returns recent inbound SMS messages.
func (f *Facade) ListInbound(ctx context.Context, since string, limit int) ([]Inbound, error) {
	rows, err := f.st.ListInbound(ctx, store.InboundFilter{SinceID: since, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Inbound, len(rows))
	for i, r := range rows {
		out[i] = toInbound(r)
	}
	return out, nil
}

// ListOutbound returns recent outbound SMS messages.
func (f *Facade) ListOutbound(ctx context.Context, since string, limit int) ([]Outbound, error) {
	rows, err := f.st.DB().QueryContext(ctx, `
		SELECT id FROM sms_outbound WHERE id > ? ORDER BY created_at DESC, id DESC LIMIT ?
	`, since, defaultLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Store uses one SQLite connection. Close the ID query before fetching
	// full rows or the nested reads wait forever for that same connection.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var out []Outbound
	for _, id := range ids {
		o, err := f.st.GetOutbound(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, toOutbound(o))
	}
	return out, nil
}

func toInbound(in store.Inbound) Inbound {
	return Inbound{
		ID:         in.ID,
		From:       in.FromAddr,
		Body:       in.Body,
		Encoding:   in.Encoding,
		Parts:      in.Parts,
		Incomplete: in.Incomplete,
		SMSCTime:   in.SMSCTime,
		ReceivedAt: in.ReceivedAt,
	}
}

func toOutbound(o store.Outbound) Outbound {
	r := Outbound{
		ID: o.ID, To: o.ToAddr, Body: o.Body, State: o.State,
		Encoding: o.Encoding, ErrorCode: o.ErrorCode,
		ErrorDetail: o.ErrorDetail, CreatedAt: o.CreatedAt,
	}
	for _, mr := range o.MRs {
		r.Parts = append(r.Parts, OutboundPart{MR: mr})
	}
	return r
}

func outboundJSON(o Outbound) string {
	// keep it minimal — used only for idem replay
	parts := ""
	for i, p := range o.Parts {
		if i > 0 {
			parts += ","
		}
		parts += fmt.Sprintf(`{"mr":%d}`, p.MR)
	}
	return fmt.Sprintf(`{"id":%q,"state":%q,"parts":[%s],"encoding":%q}`, o.ID, o.State, parts, o.Encoding)
}
