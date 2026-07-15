// Package store wraps SQLite (modernc.org/sqlite) with intent-named queries.
// SetMaxOpenConns(1) intentionally serializes writes; reads share the conn.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db *sql.DB
}

// Open opens (and creates if needed) the SQLite database at path. Use ":memory:"
// for tests. Migrations are applied transactionally before returning.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		if err := prepareDBFile(path); err != nil {
			return nil, err
		}
		dsn = path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=secure_delete(FAST)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if path != ":memory:" {
		if err := restrictDBFiles(path); err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}

func prepareDBFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open database file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close database file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure database file: %w", err)
	}
	return nil
}

func restrictDBFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("secure database file %s: %w", candidate, err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	applied := map[int]bool{}
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	type mig struct {
		ver  int
		name string
	}
	var migs []mig
	for _, e := range entries {
		var v int
		_, err := fmt.Sscanf(e.Name(), "%04d_", &v)
		if err != nil {
			continue
		}
		migs = append(migs, mig{ver: v, name: e.Name()})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].ver < migs[j].ver })

	for _, m := range migs {
		if applied[m.ver] {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		// Run statements one at a time to surface clearer errors.
		for _, stmt := range splitSQL(string(body)) {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("migration %s: %w", m.name, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, m.ver, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func splitSQL(body string) []string {
	// Trivial splitter: split on `;\n`. Sufficient for our hand-written migrations.
	return strings.Split(body, ";\n")
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`)
	var v int
	return v, row.Scan(&v)
}

// --- KV ----------------------------------------------------------------

func (s *Store) PutKV(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO kv(key, value, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetKV(ctx context.Context, key string) (string, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key = ?`, key)
	var v string
	switch err := row.Scan(&v); err {
	case nil:
		return v, true, nil
	case sql.ErrNoRows:
		return "", false, nil
	default:
		return "", false, err
	}
}

// --- Events ------------------------------------------------------------

type Event struct {
	ID      int64
	TS      time.Time
	Kind    string
	RefKind string
	RefID   string
	Raw     string
	Detail  string // JSON
}

type EventFilter struct {
	SinceID int64
	Kind    string
	Limit   int
}

func (s *Store) AppendEvent(ctx context.Context, e Event) (int64, error) {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO events(ts, kind, ref_kind, ref_id, raw, detail)
		VALUES(?, ?, NULLIF(?,''), NULLIF(?,''), NULLIF(?,''), NULLIF(?,''))
	`, e.TS.Format(time.RFC3339Nano), e.Kind, e.RefKind, e.RefID, e.Raw, e.Detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListEvents(ctx context.Context, f EventFilter) ([]Event, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id, ts, kind, COALESCE(ref_kind,''), COALESCE(ref_id,''), COALESCE(raw,''), COALESCE(detail,'')
	      FROM events WHERE 1=1`
	var args []any
	if f.SinceID > 0 {
		q += ` AND id > ?`
		args = append(args, f.SinceID)
	}
	if f.Kind != "" {
		q += ` AND kind = ?`
		args = append(args, f.Kind)
	}
	if f.SinceID > 0 {
		q += ` ORDER BY id ASC LIMIT ?`
	} else {
		// Initial page loads should show what just happened, not the oldest
		// records in an unbounded diagnostic table.
		q += ` ORDER BY id DESC LIMIT ?`
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Kind, &e.RefKind, &e.RefID, &e.Raw, &e.Detail); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}
