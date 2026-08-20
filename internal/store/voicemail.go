package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrVoicemailDeleted = errors.New("store: voicemail was deleted locally")

// Voicemail is a carrier voicemail persisted locally with its playable audio.
// SourceID is the stable carrier/IMAP identifier used to make synchronization
// idempotent.
type Voicemail struct {
	ID          string
	SourceID    string
	FromAddr    string
	ReceivedAt  time.Time
	DurationMS  int
	ReadAt      time.Time
	ContentType string
	Audio       []byte
	AudioBytes  int
	Transcript  string
	SourceSMSID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type VoicemailFilter struct {
	UnreadOnly bool
	Limit      int
}

// UpsertVoicemail stores a fetched voicemail and replaces the carrier-owned
// metadata/audio when the same source message is fetched again. Local read
// state is preserved across a refresh.
func (s *Store) UpsertVoicemail(ctx context.Context, item Voicemail) (Voicemail, error) {
	var tombstoned int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM voicemail_tombstones WHERE source_id=?`, item.SourceID).Scan(&tombstoned); err != nil {
		return Voicemail{}, err
	}
	if tombstoned != 0 {
		return Voicemail{}, ErrVoicemailDeleted
	}
	now := time.Now().UTC()
	if item.ID == "" {
		item.ID = NewULID()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO voicemails(id, source_id, from_addr, received_at, duration_ms,
		  read_at, content_type, audio, transcript, source_sms_id, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET
		  from_addr=excluded.from_addr, received_at=excluded.received_at,
		  duration_ms=excluded.duration_ms, content_type=excluded.content_type,
		  audio=excluded.audio, transcript=excluded.transcript,
		  source_sms_id=excluded.source_sms_id, updated_at=excluded.updated_at
	`, item.ID, item.SourceID, item.FromAddr, formatDBTime(item.ReceivedAt), item.DurationMS,
		formatOptionalDBTime(item.ReadAt), item.ContentType, item.Audio, item.Transcript,
		item.SourceSMSID, formatDBTime(item.CreatedAt), formatDBTime(item.UpdatedAt))
	if err != nil {
		return Voicemail{}, err
	}
	return s.GetVoicemailBySourceID(ctx, item.SourceID, true)
}

func (s *Store) ListVoicemails(ctx context.Context, filter VoicemailFilter) ([]Voicemail, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id, source_id, from_addr, received_at, duration_ms,
		       COALESCE(read_at,''), content_type, length(audio),
		       COALESCE(transcript,''), COALESCE(source_sms_id,''), created_at, updated_at
		FROM voicemails`
	if filter.UnreadOnly {
		query += ` WHERE read_at IS NULL`
	}
	query += ` ORDER BY received_at DESC, id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Voicemail
	for rows.Next() {
		item, err := scanVoicemailMetadata(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetVoicemail(ctx context.Context, id string, includeAudio bool) (Voicemail, error) {
	return s.getVoicemail(ctx, `id=?`, id, includeAudio)
}

func (s *Store) GetVoicemailBySourceID(ctx context.Context, sourceID string, includeAudio bool) (Voicemail, error) {
	return s.getVoicemail(ctx, `source_id=?`, sourceID, includeAudio)
}

func (s *Store) getVoicemail(ctx context.Context, predicate string, value any, includeAudio bool) (Voicemail, error) {
	audioColumn := `length(audio)`
	if includeAudio {
		audioColumn = `audio`
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_id, from_addr, received_at, duration_ms,
		       COALESCE(read_at,''), content_type, `+audioColumn+`,
		       COALESCE(transcript,''), COALESCE(source_sms_id,''), created_at, updated_at
		FROM voicemails WHERE `+predicate, value)
	if includeAudio {
		return scanVoicemailWithAudio(row)
	}
	return scanVoicemailMetadata(row)
}

func (s *Store) SetVoicemailRead(ctx context.Context, id string, read bool) (Voicemail, error) {
	var readAt any
	if read {
		readAt = formatDBTime(time.Now().UTC())
	}
	result, err := s.db.ExecContext(ctx, `UPDATE voicemails SET read_at=?, updated_at=? WHERE id=?`,
		readAt, formatDBTime(time.Now().UTC()), id)
	if err != nil {
		return Voicemail{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Voicemail{}, sql.ErrNoRows
	}
	return s.GetVoicemail(ctx, id, false)
}

func (s *Store) DeleteVoicemail(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sourceID string
	if err := tx.QueryRowContext(ctx, `SELECT source_id FROM voicemails WHERE id=?`, id).Scan(&sourceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO voicemail_tombstones(source_id, deleted_at) VALUES(?, ?)`, sourceID, formatDBTime(time.Now().UTC())); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM voicemails WHERE id=?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

type rowScanner interface {
	Scan(...any) error
}

func scanVoicemailMetadata(row rowScanner) (Voicemail, error) {
	var item Voicemail
	var received, readAt, created, updated string
	err := row.Scan(&item.ID, &item.SourceID, &item.FromAddr, &received, &item.DurationMS,
		&readAt, &item.ContentType, &item.AudioBytes, &item.Transcript, &item.SourceSMSID,
		&created, &updated)
	if err != nil {
		return Voicemail{}, err
	}
	item.Audio = nil
	parseVoicemailTimes(&item, received, readAt, created, updated)
	return item, nil
}

func scanVoicemailWithAudio(row rowScanner) (Voicemail, error) {
	var item Voicemail
	var received, readAt, created, updated string
	err := row.Scan(&item.ID, &item.SourceID, &item.FromAddr, &received, &item.DurationMS,
		&readAt, &item.ContentType, &item.Audio, &item.Transcript, &item.SourceSMSID,
		&created, &updated)
	if err != nil {
		return Voicemail{}, err
	}
	parseVoicemailTimes(&item, received, readAt, created, updated)
	item.AudioBytes = len(item.Audio)
	return item, nil
}

func parseVoicemailTimes(item *Voicemail, received, readAt, created, updated string) {
	item.ReceivedAt, _ = time.Parse(time.RFC3339Nano, received)
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if readAt != "" {
		item.ReadAt, _ = time.Parse(time.RFC3339Nano, readAt)
	}
}

func formatDBTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalDBTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return formatDBTime(value)
}
