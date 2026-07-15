package store

import (
	"context"
	"time"
)

type Call struct {
	ID          string
	Direction   string
	RemoteAddr  string
	State       string
	EndReason   string
	StartedAt   time.Time
	AnsweredAt  time.Time
	EndedAt     time.Time
	DurationMS  int
	IdemKey     string
	ErrorCode   string
	ErrorDetail string
}

type CallUpdate struct {
	State       string
	EndReason   string
	AnsweredAt  time.Time
	EndedAt     time.Time
	DurationMS  int
	ErrorCode   string
	ErrorDetail string
}

func (s *Store) InsertCall(ctx context.Context, c Call) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO calls(id, direction, remote_addr, state, end_reason,
		                  started_at, answered_at, ended_at, duration_ms,
		                  idem_key, error_code, error_detail)
		VALUES(?,?,?,?,NULLIF(?,''),?,NULLIF(?,''),NULLIF(?,''),NULLIF(?,0),
		       NULLIF(?,''),NULLIF(?,''),NULLIF(?,''))
	`,
		c.ID, c.Direction, c.RemoteAddr, c.State, c.EndReason,
		c.StartedAt.UTC().Format(time.RFC3339Nano),
		nullTime(c.AnsweredAt), nullTime(c.EndedAt), c.DurationMS,
		c.IdemKey, c.ErrorCode, c.ErrorDetail,
	)
	if err != nil && isUniqueErr(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) SetCallState(ctx context.Context, id string, u CallUpdate) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE calls SET
		  state         = ?,
		  end_reason    = COALESCE(NULLIF(?,''), end_reason),
		  answered_at   = COALESCE(NULLIF(?,''), answered_at),
		  ended_at      = COALESCE(NULLIF(?,''), ended_at),
		  duration_ms   = CASE WHEN ?>0 THEN ? ELSE duration_ms END,
		  error_code    = COALESCE(NULLIF(?,''), error_code),
		  error_detail  = COALESCE(NULLIF(?,''), error_detail)
		WHERE id=?
	`,
		u.State, u.EndReason, nullTime(u.AnsweredAt), nullTime(u.EndedAt),
		u.DurationMS, u.DurationMS, u.ErrorCode, u.ErrorDetail, id,
	)
	return err
}

// SetCallRemoteAddr fills in caller ID when the initial ring indication did
// not include a number and a later +CLIP/CLCC update does.
func (s *Store) SetCallRemoteAddr(ctx context.Context, id, remoteAddr string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE calls SET remote_addr=? WHERE id=?`, remoteAddr, id)
	return err
}

func (s *Store) GetCall(ctx context.Context, id string) (Call, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, direction, remote_addr, state, COALESCE(end_reason,''),
		       started_at, COALESCE(answered_at,''), COALESCE(ended_at,''),
		       COALESCE(duration_ms,0), COALESCE(idem_key,''),
		       COALESCE(error_code,''), COALESCE(error_detail,'')
		FROM calls WHERE id=?
	`, id)
	var c Call
	var s1, a, e string
	if err := row.Scan(&c.ID, &c.Direction, &c.RemoteAddr, &c.State, &c.EndReason,
		&s1, &a, &e, &c.DurationMS, &c.IdemKey, &c.ErrorCode, &c.ErrorDetail); err != nil {
		return Call{}, err
	}
	c.StartedAt, _ = time.Parse(time.RFC3339Nano, s1)
	if a != "" {
		c.AnsweredAt, _ = time.Parse(time.RFC3339Nano, a)
	}
	if e != "" {
		c.EndedAt, _ = time.Parse(time.RFC3339Nano, e)
	}
	return c, nil
}

func (s *Store) ListOpenCalls(ctx context.Context) ([]Call, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, direction, remote_addr, state, COALESCE(end_reason,''),
		       started_at, COALESCE(answered_at,''), COALESCE(ended_at,''),
		       COALESCE(duration_ms,0), COALESCE(idem_key,''),
		       COALESCE(error_code,''), COALESCE(error_detail,'')
		FROM calls WHERE state IN ('ringing','dialing','alerting','active','held')
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Call
	for rows.Next() {
		var c Call
		var s1, a, e string
		if err := rows.Scan(&c.ID, &c.Direction, &c.RemoteAddr, &c.State, &c.EndReason,
			&s1, &a, &e, &c.DurationMS, &c.IdemKey, &c.ErrorCode, &c.ErrorDetail); err != nil {
			return nil, err
		}
		c.StartedAt, _ = time.Parse(time.RFC3339Nano, s1)
		if a != "" {
			c.AnsweredAt, _ = time.Parse(time.RFC3339Nano, a)
		}
		if e != "" {
			c.EndedAt, _ = time.Parse(time.RFC3339Nano, e)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// HasRecentlyEndedCall detects the short interval where some modems continue
// reporting a call in CLCC after ATH/remote hangup has already completed.
// Reconciliation uses this as a tombstone to avoid creating a phantom call.
func (s *Store) HasRecentlyEndedCall(ctx context.Context, direction, remoteAddr string, since time.Time) (bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM calls
			WHERE direction = ?
			  AND REPLACE(remote_addr, '+', '') = REPLACE(?, '+', '')
			  AND state NOT IN ('ringing','dialing','alerting','active','held')
			  AND ended_at >= ?
		)
	`, direction, remoteAddr, since.UTC().Format(time.RFC3339Nano))
	var found int
	if err := row.Scan(&found); err != nil {
		return false, err
	}
	return found != 0, nil
}
