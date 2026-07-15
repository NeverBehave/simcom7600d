package store

import (
	"context"
	"time"
)

type InboundPart struct {
	Ref        int
	Total      int
	Seq        int
	FromAddr   string
	SMSCTime   time.Time
	Body       string
	Encoding   string
	RawPDU     string
	ReceivedAt time.Time
}

func (s *Store) PutPart(ctx context.Context, p InboundPart) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sms_inbound_parts(ref, total, seq, from_addr, smsc_ts, body, encoding, raw_pdu, received_at)
		VALUES(?,?,?,?,NULLIF(?,''),?,?,?,?)
		ON CONFLICT(from_addr, ref, seq) DO UPDATE SET
		  total=excluded.total, body=excluded.body, encoding=excluded.encoding,
		  raw_pdu=excluded.raw_pdu, received_at=excluded.received_at
	`,
		p.Ref, p.Total, p.Seq, p.FromAddr, nullTime(p.SMSCTime),
		p.Body, p.Encoding, p.RawPDU, p.ReceivedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetPartsForReassembly(ctx context.Context, from string, ref int) ([]InboundPart, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ref, total, seq, from_addr, COALESCE(smsc_ts,''), body, encoding, raw_pdu, received_at
		FROM sms_inbound_parts WHERE from_addr=? AND ref=? ORDER BY seq ASC
	`, from, ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InboundPart
	for rows.Next() {
		var p InboundPart
		var smsc, rec string
		if err := rows.Scan(&p.Ref, &p.Total, &p.Seq, &p.FromAddr, &smsc, &p.Body, &p.Encoding, &p.RawPDU, &rec); err != nil {
			return nil, err
		}
		if smsc != "" {
			p.SMSCTime, _ = time.Parse(time.RFC3339Nano, smsc)
		}
		p.ReceivedAt, _ = time.Parse(time.RFC3339Nano, rec)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeleteParts(ctx context.Context, from string, ref int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sms_inbound_parts WHERE from_addr=? AND ref=?`, from, ref)
	return err
}

// PartsOlderThan returns reassembly groups whose oldest received_at is before t.
// Used by the 24h promotion job.
func (s *Store) PartsOlderThan(ctx context.Context, t time.Time) ([]InboundPart, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ref, total, seq, from_addr, COALESCE(smsc_ts,''), body, encoding, raw_pdu, received_at
		FROM sms_inbound_parts WHERE received_at < ? ORDER BY from_addr, ref, seq
	`, t.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InboundPart
	for rows.Next() {
		var p InboundPart
		var smsc, rec string
		if err := rows.Scan(&p.Ref, &p.Total, &p.Seq, &p.FromAddr, &smsc, &p.Body, &p.Encoding, &p.RawPDU, &rec); err != nil {
			return nil, err
		}
		if smsc != "" {
			p.SMSCTime, _ = time.Parse(time.RFC3339Nano, smsc)
		}
		p.ReceivedAt, _ = time.Parse(time.RFC3339Nano, rec)
		out = append(out, p)
	}
	return out, rows.Err()
}
