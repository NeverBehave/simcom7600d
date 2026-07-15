package store

import (
	"context"
	"database/sql"
	"time"
)

type RetentionPolicy struct {
	EventsBefore time.Time
	SMSBefore    time.Time
	CallsBefore  time.Time
	PartsBefore  time.Time
}

type RetentionResult struct {
	Events int64
	SMS    int64
	Calls  int64
	Parts  int64
}

func (r RetentionResult) Total() int64 { return r.Events + r.SMS + r.Calls + r.Parts }

// SweepRetention deletes only completed historical records. Live calls and
// outbound messages that may still receive state updates are retained even if
// their timestamps cross a configured cutoff.
func (s *Store) SweepRetention(ctx context.Context, policy RetentionPolicy) (result RetentionResult, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if !policy.EventsBefore.IsZero() {
		result.Events, err = deleteBefore(ctx, tx, `DELETE FROM events WHERE ts < ?`, policy.EventsBefore)
		if err != nil {
			return result, err
		}
	}
	if !policy.SMSBefore.IsZero() {
		var inbound, outbound int64
		inbound, err = deleteBefore(ctx, tx, `DELETE FROM sms_inbound WHERE received_at < ?`, policy.SMSBefore)
		if err != nil {
			return result, err
		}
		outbound, err = deleteBefore(ctx, tx, `DELETE FROM sms_outbound WHERE created_at < ? AND state NOT IN ('queued','submitted','accepted')`, policy.SMSBefore)
		if err != nil {
			return result, err
		}
		result.SMS = inbound + outbound
	}
	if !policy.CallsBefore.IsZero() {
		result.Calls, err = deleteBefore(ctx, tx, `DELETE FROM calls WHERE started_at < ? AND state NOT IN ('ringing','dialing','alerting','active','held')`, policy.CallsBefore)
		if err != nil {
			return result, err
		}
	}
	if !policy.PartsBefore.IsZero() {
		result.Parts, err = deleteBefore(ctx, tx, `DELETE FROM sms_inbound_parts WHERE received_at < ?`, policy.PartsBefore)
		if err != nil {
			return result, err
		}
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func deleteBefore(ctx context.Context, tx *sql.Tx, query string, cutoff time.Time) (int64, error) {
	res, err := tx.ExecContext(ctx, query, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
