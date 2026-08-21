-- 0002_voicemail.sql

CREATE TABLE voicemails (
  id            TEXT PRIMARY KEY,
  source_id     TEXT NOT NULL UNIQUE,
  from_addr     TEXT NOT NULL,
  received_at   TEXT NOT NULL,
  duration_ms   INTEGER NOT NULL DEFAULT 0,
  read_at       TEXT,
  content_type  TEXT NOT NULL,
  audio         BLOB NOT NULL,
  transcript    TEXT,
  source_sms_id TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
CREATE INDEX voicemails_received ON voicemails(received_at DESC, id DESC);
CREATE INDEX voicemails_unread ON voicemails(received_at DESC) WHERE read_at IS NULL;

CREATE TABLE voicemail_tombstones (
  source_id  TEXT PRIMARY KEY,
  deleted_at TEXT NOT NULL
);
