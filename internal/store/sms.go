package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"modernc.org/sqlite"
)

var ErrDuplicate = errors.New("store: duplicate")

type Inbound struct {
	ID         string
	FromAddr   string
	Body       string
	Encoding   string
	Parts      int
	Incomplete bool
	SMSCTime   time.Time
	ReceivedAt time.Time
	DedupeKey  string
	RawPDUs    string // JSON array
}

type Outbound struct {
	ID             string
	ToAddr         string
	Body           string
	Encoding       string
	Parts          int
	State          string
	MRs            []int
	DeliveryReport bool
	ErrorCode      string
	ErrorDetail    string
	IdemKey        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeliveredAt    time.Time
}

type InboundFilter struct {
	From    string
	SinceID string
	Limit   int
	Cursor  string
}

type OutboundFilter struct {
	State   string
	SinceID string
	Limit   int
	Cursor  string
}

// InsertInbound writes an inbound SMS row. Returns ErrDuplicate if dedupe_key
// collides with an existing row.
func (s *Store) InsertInbound(ctx context.Context, in Inbound) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sms_inbound(id, from_addr, body, encoding, parts, incomplete,
		                       smsc_ts, received_at, dedupe_key, raw_pdus)
		VALUES(?, ?, ?, ?, ?, ?, NULLIF(?,''), ?, ?, ?)
	`,
		in.ID, in.FromAddr, in.Body, in.Encoding, in.Parts, boolToInt(in.Incomplete),
		nullTime(in.SMSCTime), in.ReceivedAt.UTC().Format(time.RFC3339Nano),
		in.DedupeKey, in.RawPDUs,
	)
	if err != nil && isUniqueErr(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) ListInbound(ctx context.Context, f InboundFilter) ([]Inbound, error) {
	limit := defaultLimit(f.Limit)
	q := `SELECT id, from_addr, body, encoding, parts, incomplete,
	             COALESCE(smsc_ts,''), received_at, dedupe_key, raw_pdus
	      FROM sms_inbound WHERE 1=1`
	var args []any
	if f.From != "" {
		q += ` AND from_addr = ?`
		args = append(args, f.From)
	}
	if f.SinceID != "" {
		q += ` AND id > ?`
		args = append(args, f.SinceID)
	}
	q += ` ORDER BY received_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Inbound
	for rows.Next() {
		var in Inbound
		var rec, smsc string
		var inc int
		if err := rows.Scan(&in.ID, &in.FromAddr, &in.Body, &in.Encoding, &in.Parts, &inc, &smsc, &rec, &in.DedupeKey, &in.RawPDUs); err != nil {
			return nil, err
		}
		in.Incomplete = inc != 0
		in.ReceivedAt, _ = time.Parse(time.RFC3339Nano, rec)
		if smsc != "" {
			in.SMSCTime, _ = time.Parse(time.RFC3339Nano, smsc)
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

func (s *Store) InsertOutbound(ctx context.Context, o Outbound) error {
	mrs, _ := json.Marshal(o.MRs)
	if mrs == nil {
		mrs = []byte("[]")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sms_outbound(id, to_addr, body, encoding, parts, state, mrs,
		                        delivery_report, error_code, error_detail, idem_key,
		                        created_at, updated_at, delivered_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?,''), NULLIF(?,''), NULLIF(?,''),
		       ?, ?, NULLIF(?,''))
	`,
		o.ID, o.ToAddr, o.Body, o.Encoding, o.Parts, o.State, string(mrs),
		boolToInt(o.DeliveryReport), o.ErrorCode, o.ErrorDetail, o.IdemKey,
		o.CreatedAt.UTC().Format(time.RFC3339Nano),
		o.UpdatedAt.UTC().Format(time.RFC3339Nano),
		nullTime(o.DeliveredAt),
	)
	if err != nil && isUniqueErr(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) SetOutboundState(ctx context.Context, id, state string, mrs []int, errCode, errDetail string) error {
	mrsJSON, _ := json.Marshal(mrs)
	if mrsJSON == nil {
		mrsJSON = []byte("[]")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE sms_outbound SET state=?, mrs=?, error_code=NULLIF(?,''), error_detail=NULLIF(?,''),
		                       updated_at=?
		WHERE id=?
	`, state, string(mrsJSON), errCode, errDetail, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *Store) MarkDelivered(ctx context.Context, mr int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sms_outbound SET state='delivered', delivered_at=?, updated_at=?
		WHERE state IN ('submitted','accepted')
		  AND EXISTS (SELECT 1 FROM json_each(mrs) WHERE value = ?)
	`, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), mr)
	return err
}

func (s *Store) GetOutbound(ctx context.Context, id string) (Outbound, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, to_addr, body, encoding, parts, state, mrs,
		       delivery_report, COALESCE(error_code,''), COALESCE(error_detail,''),
		       COALESCE(idem_key,''), created_at, updated_at, COALESCE(delivered_at,'')
		FROM sms_outbound WHERE id=?
	`, id)
	var o Outbound
	var mrs string
	var dr int
	var c, u, d string
	if err := row.Scan(&o.ID, &o.ToAddr, &o.Body, &o.Encoding, &o.Parts, &o.State, &mrs, &dr,
		&o.ErrorCode, &o.ErrorDetail, &o.IdemKey, &c, &u, &d); err != nil {
		return Outbound{}, err
	}
	o.DeliveryReport = dr != 0
	_ = json.Unmarshal([]byte(mrs), &o.MRs)
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
	o.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
	if d != "" {
		o.DeliveredAt, _ = time.Parse(time.RFC3339Nano, d)
	}
	return o, nil
}

// GetInbound retrieves a single inbound SMS by primary key.
func (s *Store) GetInbound(ctx context.Context, id string) (Inbound, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, from_addr, body, encoding, parts, incomplete,
		       COALESCE(smsc_ts,''), received_at, dedupe_key, raw_pdus
		FROM sms_inbound WHERE id=?
	`, id)
	var in Inbound
	var rec, smsc string
	var inc int
	if err := row.Scan(&in.ID, &in.FromAddr, &in.Body, &in.Encoding, &in.Parts, &inc,
		&smsc, &rec, &in.DedupeKey, &in.RawPDUs); err != nil {
		return Inbound{}, err
	}
	in.Incomplete = inc != 0
	in.ReceivedAt, _ = time.Parse(time.RFC3339Nano, rec)
	if smsc != "" {
		in.SMSCTime, _ = time.Parse(time.RFC3339Nano, smsc)
	}
	return in, nil
}

// DeleteInbound removes an inbound SMS row. Returns nil if the row does not
// exist (idempotent).
func (s *Store) DeleteInbound(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sms_inbound WHERE id=?`, id)
	return err
}

// DeleteOutbound removes an outbound SMS row. Returns nil if the row does not
// exist (idempotent).
func (s *Store) DeleteOutbound(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sms_outbound WHERE id=?`, id)
	return err
}

func (s *Store) GetOutboundByIdem(ctx context.Context, key string) (Outbound, error) {
	if key == "" {
		return Outbound{}, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `SELECT id FROM sms_outbound WHERE idem_key=?`, key)
	var id string
	if err := row.Scan(&id); err != nil {
		return Outbound{}, err
	}
	return s.GetOutbound(ctx, id)
}

// helpers

func defaultLimit(n int) int {
	if n <= 0 || n > 500 {
		return 50
	}
	return n
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func isUniqueErr(err error) bool {
	var sqe *sqlite.Error
	if errors.As(err, &sqe) {
		// 2067 = SQLITE_CONSTRAINT_UNIQUE; 1555 = SQLITE_CONSTRAINT_PRIMARYKEY
		c := sqe.Code()
		return c == 2067 || c == 1555 || c == 19
	}
	return strings.Contains(err.Error(), "UNIQUE")
}
