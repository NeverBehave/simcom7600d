package store

import (
	"context"
	"database/sql"
	"time"
)

type IdemRecord struct {
	Key       string
	Scope     string
	RefID     string
	Response  string // JSON body
	Status    int
	CreatedAt time.Time
}

func (s *Store) PutIdem(ctx context.Context, r IdemRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO idem_cache(key, scope, ref_id, response, status, created_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(key) DO NOTHING
	`, r.Key, r.Scope, r.RefID, r.Response, r.Status, r.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetIdem(ctx context.Context, key string) (IdemRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT key, scope, ref_id, response, status, created_at FROM idem_cache WHERE key=?
	`, key)
	var r IdemRecord
	var c string
	switch err := row.Scan(&r.Key, &r.Scope, &r.RefID, &r.Response, &r.Status, &c); err {
	case nil:
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		return r, true, nil
	case sql.ErrNoRows:
		return IdemRecord{}, false, nil
	default:
		return IdemRecord{}, false, err
	}
}

func (s *Store) SweepIdem(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM idem_cache WHERE created_at < ?`, before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
