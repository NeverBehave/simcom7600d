-- 0001_init.sql

CREATE TABLE kv (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,        -- JSON
  updated_at TEXT NOT NULL
);
-- Keys: status, sim, network, battery, modem, epoch, boot_at

CREATE TABLE events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         TEXT NOT NULL,        -- RFC3339, UTC
  kind       TEXT NOT NULL,        -- sms.arrived, call.ringing, call.ended, modem.reset, reconcile.diff, ...
  ref_kind   TEXT,                 -- 'sms' | 'call' | NULL
  ref_id     TEXT,                 -- ULID of related row
  raw        TEXT,                 -- raw URC line(s)
  detail     TEXT                  -- JSON details
);
CREATE INDEX events_ts ON events(ts);
CREATE INDEX events_kind_ts ON events(kind, ts);
CREATE INDEX events_ref ON events(ref_kind, ref_id);

CREATE TABLE sms_inbound (
  id            TEXT PRIMARY KEY,                 -- ULID
  from_addr     TEXT NOT NULL,                    -- E.164 if parseable
  body          TEXT NOT NULL,
  encoding      TEXT NOT NULL,                    -- gsm7 | ucs2 | 8bit
  parts         INTEGER NOT NULL DEFAULT 1,
  incomplete    INTEGER NOT NULL DEFAULT 0,       -- 1 if promoted with missing parts
  smsc_ts       TEXT,
  received_at   TEXT NOT NULL,
  dedupe_key    TEXT NOT NULL UNIQUE,             -- hash(from, smsc_ts, body)
  raw_pdus      TEXT NOT NULL                     -- JSON [hex_pdu]
);
CREATE INDEX sms_inbound_received ON sms_inbound(received_at DESC, id DESC);
CREATE INDEX sms_inbound_from ON sms_inbound(from_addr, received_at DESC);

CREATE TABLE sms_inbound_parts (
  ref          INTEGER NOT NULL,                  -- UDH reference
  total        INTEGER NOT NULL,
  seq          INTEGER NOT NULL,
  from_addr    TEXT NOT NULL,
  smsc_ts      TEXT,
  body         TEXT NOT NULL,
  encoding     TEXT NOT NULL,
  raw_pdu      TEXT NOT NULL,
  received_at  TEXT NOT NULL,
  PRIMARY KEY (from_addr, ref, seq)
);

CREATE TABLE sms_outbound (
  id              TEXT PRIMARY KEY,
  to_addr         TEXT NOT NULL,
  body            TEXT NOT NULL,
  encoding        TEXT NOT NULL,
  parts           INTEGER NOT NULL,
  state           TEXT NOT NULL,                  -- queued|submitted|accepted|delivered|failed|indeterminate
  mrs             TEXT NOT NULL DEFAULT '[]',     -- JSON [int]
  delivery_report INTEGER NOT NULL DEFAULT 0,
  error_code      TEXT,
  error_detail    TEXT,
  idem_key        TEXT,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  delivered_at    TEXT
);
CREATE INDEX sms_outbound_created ON sms_outbound(created_at DESC, id DESC);
CREATE INDEX sms_outbound_state  ON sms_outbound(state) WHERE state IN ('queued','submitted','accepted');
CREATE UNIQUE INDEX sms_outbound_idem ON sms_outbound(idem_key) WHERE idem_key IS NOT NULL;

CREATE TABLE calls (
  id            TEXT PRIMARY KEY,
  direction     TEXT NOT NULL,                    -- in | out
  remote_addr   TEXT NOT NULL,
  state         TEXT NOT NULL,                    -- ringing|dialing|alerting|active|ended|missed|rejected
  end_reason    TEXT,
  started_at    TEXT NOT NULL,
  answered_at   TEXT,
  ended_at      TEXT,
  duration_ms   INTEGER,
  idem_key      TEXT,
  error_code    TEXT,
  error_detail  TEXT
);
CREATE INDEX calls_started ON calls(started_at DESC, id DESC);
CREATE INDEX calls_open    ON calls(state) WHERE state IN ('ringing','dialing','alerting','active');
CREATE UNIQUE INDEX calls_idem ON calls(idem_key) WHERE idem_key IS NOT NULL;

CREATE TABLE idem_cache (
  key         TEXT PRIMARY KEY,
  scope       TEXT NOT NULL,                      -- 'sms' | 'call'
  ref_id      TEXT NOT NULL,
  response    TEXT NOT NULL,                      -- JSON of original 2xx body
  status      INTEGER NOT NULL,
  created_at  TEXT NOT NULL
);
CREATE INDEX idem_created ON idem_cache(created_at);

CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
