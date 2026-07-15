package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openMem(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_FilePermissionsAreOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sim7600d.db")
	s, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestStore_RetentionDeletesHistoryButKeepsLiveOperations(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)

	_, err := s.db.ExecContext(ctx, `INSERT INTO events(ts,kind) VALUES(?, 'old')`, old)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sms_inbound(id,from_addr,body,encoding,received_at,dedupe_key,raw_pdus) VALUES('in-old','+1','body','gsm7',?,'dedupe','[]')`, old)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sms_outbound(id,to_addr,body,encoding,parts,state,created_at,updated_at) VALUES('out-done','+1','body','gsm7',1,'delivered',?,?)`, old, old)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sms_outbound(id,to_addr,body,encoding,parts,state,created_at,updated_at) VALUES('out-live','+1','body','gsm7',1,'submitted',?,?)`, old, old)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO calls(id,direction,remote_addr,state,started_at) VALUES('call-done','in','+1','ended',?)`, old)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO calls(id,direction,remote_addr,state,started_at) VALUES('call-live','in','+1','held',?)`, old)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sms_inbound_parts(ref,total,seq,from_addr,body,encoding,raw_pdu,received_at) VALUES(1,2,1,'+1','part','gsm7','00',?)`, old)
	require.NoError(t, err)

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	result, err := s.SweepRetention(ctx, RetentionPolicy{EventsBefore: cutoff, SMSBefore: cutoff, CallsBefore: cutoff, PartsBefore: cutoff})
	require.NoError(t, err)
	require.Equal(t, RetentionResult{Events: 1, SMS: 2, Calls: 1, Parts: 1}, result)

	for table, want := range map[string]int{"events": 0, "sms_inbound": 0, "sms_outbound": 1, "calls": 1, "sms_inbound_parts": 0} {
		var got int
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&got))
		require.Equal(t, want, got, table)
	}
}

func TestStore_MigrationsApplied(t *testing.T) {
	s := openMem(t)
	v, err := s.SchemaVersion(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, v, 1)
}

func TestStore_KVRoundTrip(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	require.NoError(t, s.PutKV(ctx, "epoch", `{"n":7}`))
	v, ok, err := s.GetKV(ctx, "epoch")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, `{"n":7}`, v)
}

func TestStore_EventInsertAndList(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	id1, err := s.AppendEvent(ctx, Event{TS: time.Now().UTC(), Kind: "sms.arrived", Raw: "RING"})
	require.NoError(t, err)
	id2, err := s.AppendEvent(ctx, Event{TS: time.Now().UTC(), Kind: "call.ringing"})
	require.NoError(t, err)
	require.Greater(t, id2, id1)

	got, err := s.ListEvents(ctx, EventFilter{SinceID: 0, Limit: 10})
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestStore_ListEventsInitialPageIsMostRecent(t *testing.T) {
	s := openMem(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := s.AppendEvent(ctx, Event{Kind: fmt.Sprintf("event.%d", i)})
		require.NoError(t, err)
	}

	got, err := s.ListEvents(ctx, EventFilter{Limit: 2})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "event.4", got[0].Kind)
	require.Equal(t, "event.3", got[1].Kind)

	newer, err := s.ListEvents(ctx, EventFilter{SinceID: 3, Limit: 2})
	require.NoError(t, err)
	require.Equal(t, int64(4), newer[0].ID)
	require.Equal(t, int64(5), newer[1].ID)
}
