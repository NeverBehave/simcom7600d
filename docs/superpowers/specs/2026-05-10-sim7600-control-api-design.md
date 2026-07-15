# SIM7600 Control API — Design

**Date:** 2026-05-10
**Status:** Draft (pre-implementation)
**Hardware:** LILYGO SIM7600G-H R2 (ESP32 + 18650), USB-attached to a NixOS host; examples use the reserved documentation address `root@203.0.113.10`, with the AT TTY on `/dev/ttyUSB3`.

## 1. Goal

Build a Go HTTP service (`sim7600d`) that exposes the SIM7600 cellular module as a clean REST API for an application backend. v1 covers SMS send/receive and call control (dial, accept, reject, hangup, DTMF). Live call audio is **explicitly out of v1** but the design preserves a clean seam for adding it later.

## 2. Non-goals (v1)

- Live call audio bridging (WebSocket / RTP / SIP). Out of scope; revisited later.
- WebSocket / SSE / webhook event delivery. The service is poll-based; a debug `/v1/events` endpoint is the only "stream-ish" surface.
- Multi-tenancy, RBAC, per-route scopes. One shared bearer token.
- Conversations / threads abstraction; contacts; MMS.
- Deployment plumbing (systemd unit, NixOS module, packaging). Single binary; the operator runs it however they like.
- Cross-arch builds beyond what `go build` produces from a Mac (`darwin/arm64` for dev, `linux/amd64` for the remote host).

## 3. Constraints discovered during exploration

- USB PID is `1e0e:9011` (SIMCom composite: RNDIS pair + 5 AT/diag/NMEA/modem TTYs). **No USB-audio class interface in this descriptor.** Switching to a different PID and bringing up ALSA on NixOS is the prerequisite for live audio over USB; tracked as a future-work item, not done here.
- Host has no `ModemManager`, `oFono`, `NetworkManager`, ALSA, or `socat` installed. Project assumes only what we add.
- AT port responds at 115200, USB; baud is informational on USB CDC. The TTY is shared by all consumers — we own arbitration.

## 4. Top-level decisions

| Question | Decision | Why |
|---|---|---|
| Audio in v1? | No. Control plane only. | Hardware doesn't expose audio over USB on current PID; bringing it up is its own project. |
| Event delivery? | None pushed. Debug `/v1/events?since=` only. | Caller is an app backend that owns its own UX; SQLite plus polling is sufficient and simplest. |
| State store? | SQLite via `modernc.org/sqlite` (pure Go, no cgo). Single file. | Right scale; cgo-free build; trivial backup. |
| Modem protocol? | Raw AT over TTY. **Not** ModemManager / oFono / Gammu / QMI / MBIM. | Voice + SMS are AT's home turf. QMI/MBIM are data-plane protocols. ModemManager hides AT and we'd still need it for SIMCom-specific commands. Raw AT keeps debugging direct. |
| Dev rig? | TCP-PTY bridge to local Mac (path B) + cross-build for verification (path A). | Fastest inner loop; truthful integration via real binary on remote when needed. |
| Retention? | Off by default. Keep history forever. `idem_cache` is the sole exception (24h sweep, operational not history). | Single modem, low volume, history is valuable. |
| Concurrency model | Single goroutine owns the TTY; commands enqueued via channel; URCs demultiplexed in the same read loop. | Mirrors the hardware: one AT command in flight at a time. Channels give cancellation and queue visibility for free. |
| Authoritative state | The modem is authoritative for live state (active calls, SIM contents, registration). SQLite is authoritative for history and ack'd intents. A reconciler bridges the two. | Avoids the classic "DB says active, modem says idle" drift bugs. |
| API style | REST under `/v1/`, JSON, bearer token. | Matches "app backend" framing the operator described. |

## 5. Architecture

```
┌──────────────────────────────────────────────────────────────┐
│           sim7600d (single binary, single process)           │
│                                                              │
│  ┌────────────┐  ┌────────────────────────┐  ┌────────────┐  │
│  │ HTTP API   │──│      Modem facade      │──│  SQLite    │  │
│  │ (chi)      │  │  (Calls, SMS, Status)  │  │ (state +   │  │
│  └────────────┘  └────────────────────────┘  │  history)  │  │
│        ▲                  ▲       ▲          └────────────┘  │
│        │                  │       │                          │
│        │           ┌──────┴───┐ ┌─┴────────┐                 │
│        │           │ AT exec  │ │   URC    │                 │
│        │           │ (mutex,  │ │ dispatch │                 │
│        │           │ timeout) │ │ (events) │                 │
│        │           └────┬─────┘ └─────┬────┘                 │
│        │                └─────┬───────┘                      │
│        │              ┌───────┴────────┐                     │
│        │              │  TTY transport │                     │
│        │              │ (io.ReadWriter)│                     │
│        │              └───────┬────────┘                     │
└────────┼──────────────────────┼──────────────────────────────┘
         │                      │
   bearer token            ┌────┴───────────────────┐
   over HTTP               │ /dev/ttyUSB3 (prod)    │
   (localhost              │ or /tmp/sim7600 PTY    │
   default)                │ (dev → socat → SSH →   │
                           │  remote /dev/ttyUSB3)  │
                           └────────────────────────┘
```

Transport is an `io.ReadWriteCloser` regardless of environment. In prod, it's `os.OpenFile("/dev/ttyUSB3", ...)`. In dev, it's the same call against `/tmp/sim7600` (a local PTY created by `socat`, fed by an SSH-forwarded TCP stream from the remote host's real `/dev/ttyUSB3`). No `if dev` branches in code.

## 6. Components

Each component is a separate package with a small interface; testable in isolation.

### 6.1 `internal/ttyx` — transport
- `OpenTTY(path string) (Transport, error)` returning `io.ReadWriteCloser`.
- Sets termios via `unix.IoctlSetTermios` — no shelling out to `stty`.
- Reconnect loop owned here. On EOF/EIO from `Read`, close, sleep, reopen.
- Exposes `Health() <-chan State` (`Up | Flapping | Down`) for the executor.

### 6.2 `internal/atproto` — line-oriented AT codec
- `Scanner` over the transport that emits *frames*: final result code (`OK`, `ERROR`, `+CME ERROR: N`, `+CMS ERROR: N`, `NO CARRIER`, `BUSY`, `NO ANSWER`, `CONNECT`) or an intermediate response line.
- Strips `\r\n`, handles SIM7600's leading-blank-line quirk.
- Pure parsing, stateless. Trivial unit tests with byte buffers.
- Static map of `+CME`/`+CMS` numeric codes to human meanings.

### 6.3 `internal/atexec` — serialized command executor
- Owns the TTY. One goroutine, one command in flight.
- API: `Exec(ctx context.Context, req Request) (Response, error)` where `Request` carries either a one-shot command line *or* a multi-step `run` function (for prompt-driven sequences like `+CMGS`).
- Per-request timeout (table in §10).
- Frames matching a registered URC prefix are forwarded to the URC bus regardless of pending-command state. Final-result tokens are matched only when expected.
- ~50 ms write pacing pause between commands.
- On timeout: send `\x1B`, drain 200 ms, fail the request, declare transport unhealthy if the next command also times out.

### 6.4 `internal/urc` — URC dispatcher
- `Subscribe(prefix string) <-chan Event`, multiple subscribers per prefix.
- Recognised v1 prefixes: `RING`, `+CLIP:`, `+CRING:`, `NO CARRIER`, `BUSY`, `+CMTI:`, `+CMT:`, `+CDS:`, `+CREG:`, `+CGREG:`, `+CEREG:`, `+CPIN:`, `+CUSD:`, `RDY`. Unknown URCs go to a `urc.unknown` debug channel and are logged at info.
- Stamps every URC with a monotonic ID and writes it to `events` before fanning out. Subscribers can resume from an ID after restart.

### 6.5 `internal/modem` — domain facade
The "what the rest of the app sees." Hides AT entirely.

```go
type Modem interface {
    Status(ctx context.Context) (ModemStatus, error)
    SendSMS(ctx context.Context, to, body string, opts SendOpts) (Outbound, error)
    ListInbound(ctx context.Context, q InboundQuery) ([]Inbound, error)
    ListOutbound(ctx context.Context, q OutboundQuery) ([]Outbound, error)
    Dial(ctx context.Context, to string) (Call, error)
    Hangup(ctx context.Context, callID string) error
    Answer(ctx context.Context, callID string) error
    Reject(ctx context.Context, callID string) error
    SendDTMF(ctx context.Context, callID, digits string, durMS int) error
    OnIncomingCall() <-chan IncomingCall
    OnInboundSMS()  <-chan SMSArrived
    OnLifecycle()   <-chan LifecycleEvent
}
```

Boot sequence: `ATE0`, `AT+CMEE=2`, `AT+CMGF=0` (PDU), `AT+CNMI=2,1,0,1,0`, `AT+CLIP=1`, `AT+CRC=1`, `AT+CSCS="UCS2"`.

### 6.6 `internal/store` — SQLite persistence
Tables in §8. Single shared `*sql.DB`, `SetMaxOpenConns(1)`. Migrations embedded with `embed.FS`, applied at startup. Intent-named query methods on `*Store`; no ORM. Test target: in-memory SQLite (`file::memory:?cache=shared`).

### 6.7 `internal/api` — HTTP layer
`chi` router. Handlers are thin: validate → call modem facade → serialize. Bearer-token auth from `auth_token_file`. Default bind `127.0.0.1:8080`. JSON in/out, snake_case fields, RFC3339 UTC timestamps, ULID IDs.

### 6.8 `internal/reconciler`
Bridges modem-truth and DB-truth. Methods: `Boot(ctx)`, `Reconcile(ctx)`. Authoritative reads: `+CLCC` (calls), `+CMGL=4` (SMS), `+CSQ`, `+COPS?`, `+CREG?`, `+CPIN?`, `+CBC`. Triggered on boot, transport reconnect, modem-reset signal, every 30s when idle, every 1s while a call is open. Writes a `reconcile.diff` event whenever it changes the DB.

### 6.9 `internal/simmodem` — in-process modem simulator
A test/dev double that owns one side of a `socketpair` and replays scripted SIM7600 responses. Used in integration tests and as `--tty sim:<scenario>` for hands-off development.

### 6.10 `cmd/sim7600d` — main
Loads config, opens transport, builds executor → urc → store → modem facade → reconciler → api, wires graceful shutdown (stop accepting HTTP, wait up to 30s for in-flight, drain queue, close TTY — does **not** hang up an active call on shutdown).

## 7. Data flow (canonical traces)

### 7.1 Outbound SMS — `POST /v1/sms`

1. Handler validates `to` (E.164 via `nyaruka/phonenumbers`), reads `Idempotency-Key` header.
2. If idem key present and matched in `idem_cache`: return cached body + status, skip work.
3. `modem.SendSMS`:
   1. Pick encoding (GSM-7 if body fits, else UCS-2). Build PDU(s) — multipart with UDH if needed.
   2. INSERT `sms_outbound` row, `state="queued"`.
   3. For each PDU part:
      - `Exec("AT+CMGS=<pdulen>")` → wait for `> ` prompt.
      - Write `<pdu_hex>` + `0x1A` → wait for `+CMGS: <mr>` final.
      - Append `mr` to row's `mrs` JSON.
   4. UPDATE row `state="submitted"` → `"accepted"` once all parts have `mr`.
4. On `+CDS` URC matching a stored `mr`: UPDATE row `state="delivered"`, `delivered_at=now`.

### 7.2 Inbound SMS

`+CMTI: "ME",<idx>` URC arrives.
1. urc dispatcher writes `events(kind="sms.arrived", raw=...)`.
2. Workflow: `Exec("AT+CMGR=<idx>")` retrieves PDU.
3. Decode → if multi-part, write to `sms_inbound_parts`; else go to step 5.
4. If reassembly complete, materialise into `sms_inbound`. Partials stay in `sms_inbound_parts` until 24h, then promoted to `sms_inbound` with `incomplete=true`.
5. Dedupe by `(from_addr, smsc_ts, body_hash)`.
6. INSERT `sms_inbound`.
7. `Exec("AT+CMGD=<idx>")` to free the slot.

### 7.3 Outbound call — `POST /v1/calls`

1. Handler validates `to`, idem-checks, INSERT `calls(direction="out", state="dialing")`.
2. `modem.Dial`: `Exec("ATD<num>;")` (the `;` makes it voice). `OK` here means "dial accepted," not "answered."
3. While the call row stays open: poll `+CLCC` every 1 s. Update `state` from CLCC's per-leg state field. Watch URCs for `NO CARRIER` / `BUSY` / `NO ANSWER`.
4. On terminal cause: UPDATE row `state="ended"`, `end_reason`, `ended_at`, `duration_ms`.

### 7.4 Inbound call

1. `RING` URC: ignored (heartbeat).
2. `+CLIP:` URC: INSERT `calls(direction="in", state="ringing", remote_addr=...)`. Emit `call.ringing` event.
3. Operator action:
   - `POST /v1/calls/<id>/answer` → `Exec("ATA")` → state=`active` (then poll +CLCC).
   - `POST /v1/calls/<id>/reject` → `Exec("ATH")` → state=`rejected`.
   - No action → eventual `NO CARRIER` URC → state=`missed`.

### 7.5 Cross-cutting: command/URC interleaving

The atexec scanner classifies every line by prefix. If the prefix matches a registered URC, route it to urc bus regardless of pending-command state. Final-result tokens (`OK`, `ERROR`, `+CME ERROR:`, `+CMS ERROR:`) are matched only when there's a pending command expecting them; otherwise they go to urc. `NO CARRIER` / `BUSY` / `NO ANSWER` are dual-classified — final code if a call command is pending, otherwise URC.

### 7.6 Concurrency invariant (single TTY, single in-flight)

```
HTTP handlers ─▶ chan request ─▶ ┌──────────────────────────────┐
URC workflows ─▶                 │  atexec.run() (1 goroutine)  │
                                 │   • dequeue req              │
                                 │   • write to TTY             │
                                 │   • read & classify frames   │
                                 │   • frame is URC? → urc.Bus  │
                                 │   • frame is final? → result │
                                 │   • respect ctx + timeout    │
                                 └──────────────────────────────┘
                                          │
                              urc.Bus ────┘ (async fan-out, never blocks)
```

Channels (not mutexes): callers can cancel via `ctx`, queue depth is observable, multi-step transactions naturally hold the goroutine for the whole sequence.

## 8. Persistence schema (SQLite)

WAL mode, `synchronous=NORMAL`, `busy_timeout=5000`. File: `sim7600d.db` (path configurable).

```sql
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
```

### 8.1 Retention defaults

| Table | Default |
|---|---|
| `events` | keep forever |
| `sms_inbound` / `sms_outbound` / `calls` | keep forever |
| `sms_inbound_parts` | promote to `sms_inbound{incomplete=1}` after 24h |
| `idem_cache` | hourly sweep, 24h TTL |

`VACUUM` runs only via `POST /v1/admin/vacuum`.

## 9. HTTP API

All endpoints under `/v1/`. Bearer token (`Authorization: Bearer <token>`), single secret in `auth_token_file`. JSON in/out, snake_case, RFC3339 UTC, ULID IDs. Default bind `127.0.0.1:8080`.

### 9.1 Status

```
GET  /v1/status                    snapshot, served from kv
GET  /v1/status?refresh=1          force on-demand AT poll
```

```json
{
  "modem":   { "model":"SIM7600G-H", "imei":"...", "firmware":"...", "epoch":7 },
  "sim":     { "state":"ready", "iccid":"...", "imsi":"...", "operator":"Helium" },
  "network": { "tech":"LTE", "band":"EUTRAN-BAND2", "rsrp_dbm":-107, "rsrq_db":-10, "csq":20, "registered":true },
  "battery": { "voltage_v":3.95 },
  "uptime_s": 12345,
  "ts":      "2026-05-10T08:31:00Z"
}
```

### 9.2 SMS

```
GET    /v1/sms?direction=&since=&from=&to=&limit=&cursor=
POST   /v1/sms                      send (optional Idempotency-Key)
GET    /v1/sms/<id>
DELETE /v1/sms/<id>                 soft-delete from history; modem unaffected
```

POST request:
```json
{ "to": "+15551234567", "body": "hi", "delivery_report": true }
```

POST response (`202 Accepted`):
```json
{
  "id": "01HV...",
  "state": "submitted",
  "parts": [{"mr": 42}, {"mr": 43}],
  "encoding": "ucs2",
  "ts": "2026-05-10T08:31:00Z"
}
```

Inbound shape (`GET /v1/sms`):
```json
{
  "id": "01HV...",
  "direction": "in",
  "from": "+15551234567",
  "body": "hi back",
  "received_at": "2026-05-10T08:31:00Z",
  "smsc_ts":     "2026-05-10T08:30:58Z",
  "parts": 1,
  "encoding": "gsm7",
  "incomplete": false
}
```

Pagination: cursor on `(received_at desc, id desc)`. `since=<id>` is a "newer than" convenience.

### 9.3 Calls

```
GET   /v1/calls?state=&since=&limit=&cursor=
POST  /v1/calls                       place outbound (optional Idempotency-Key)
GET   /v1/calls/<id>
POST  /v1/calls/<id>/answer
POST  /v1/calls/<id>/reject
POST  /v1/calls/<id>/hangup
POST  /v1/calls/<id>/dtmf             { "digits":"1234#", "duration_ms":100 }
```

POST `/v1/calls` request:
```json
{ "to": "+15551234567" }
```
Response `201`:
```json
{ "id": "01HV...", "direction":"out", "to":"+15551234567", "state":"dialing", "started_at":"..." }
```

DTMF digits validated against `[0-9A-D*#]`.

### 9.4 Events (debug)

```
GET /v1/events?since=<id>&kind=<filter>&limit=200
```
Read-only. Kinds enumerated in §6.4 / §6.8.

### 9.5 Admin

```
POST /v1/admin/reconcile             run reconciler synchronously, return diff
GET  /v1/admin/queue                  atexec depth, in-flight cmd, last 10 timings, transport state, epoch
POST /v1/admin/at                     raw AT passthrough; flag-gated; logs at warn
POST /v1/admin/at-reset               AT+CFUN=1,1; flag-gated
POST /v1/admin/vacuum                 SQLite VACUUM
```

### 9.6 Error envelope

```json
{ "error": { "code": "modem_busy", "message": "...", "details": { } } }
```

Codes:
- `400 invalid_request`, `401 unauthorized`, `404 not_found`, `409 conflict`, `429 too_many_requests`
- `503 modem_not_ready`, `503 modem_resetting`, `503 modem_unavailable`, `504 modem_timeout`
- `502 modem_error` (with `details.at_code` and `details.meaning`)

### 9.7 Conventions

- Phone numbers normalized to E.164 (`nyaruka/phonenumbers`); 400 if unparseable. Non-numeric special destinations (e.g. `611`) require `?raw=1`.
- `Idempotency-Key` accepted on every POST. Cached 24h. Same key → same response (replay, **not** re-execute).
- Read endpoints are safe to poll. Internal rate limiting is not implemented in v1.
- DELETE never touches the modem.

## 10. Error handling, timeouts, and recovery

### 10.1 Layered error model

```
HTTP envelope          ← shape clients see
  └─ Modem facade       ← typed: ErrModemBusy, ErrUnregistered, ErrModemReset, ErrSimNotReady, ...
      └─ atexec         ← ErrTimeout, ErrATFinal{code, raw}, ErrTransportClosed
          └─ ttyx       ← os errors; surfaces transport flaps
```

Each layer translates errors going up; lower-layer errors never leak through HTTP.

### 10.2 AT final → daemon mapping

| AT final | Daemon error | HTTP |
|---|---|---|
| `OK` | nil | 2xx |
| `ERROR` | `ErrATFinal{code:"ERROR"}` | 502 `modem_error` |
| `+CME ERROR: 10` | `ErrSimNotInserted` | 503 `sim_not_ready` |
| `+CME ERROR: 11/12/13` | `ErrSimPin*` | 503 `sim_not_ready` |
| `+CMS ERROR: N` | `ErrSMSFailed{code, meaning}` | 502 with details |
| `NO CARRIER` (during dial) | normal terminal cause | 200, `state=ended,end_reason=no_carrier` |
| `BUSY` | normal terminal cause | 200, `state=ended,end_reason=busy` |
| `NO ANSWER` | normal terminal cause | 200, `state=ended,end_reason=no_answer` |

### 10.3 Per-command timeouts

| Command | Timeout |
|---|---|
| Default (status, query) | 2 s |
| `AT+COPS=...` (operator search) | 60 s |
| `AT+CFUN=1,1` (reboot) | 30 s, then declare modem reset |
| `AT+CMGS=...` (PDU prompt) | 5 s for prompt, 30 s for final |
| `ATD<num>;` | 90 s upper bound on dial→ringing→answer or terminal cause |
| `ATA` / `ATH` | 5 s |
| `AT+VTS=` | 2 s per digit batch |

On timeout: send ESC (`0x1B`), drain 200 ms, fail the request. If the next command also times out, escalate to transport recycle.

### 10.4 Transport health state machine

- `Up` — normal.
- `Flapping` — last command timed out or read returned EAGAIN repeatedly. Drain queue with `503 modem_resetting`, sleep 500 ms, reopen.
- `Down` — three consecutive failed reopens. Stop accepting commands; reopen with exponential backoff capped at 30 s. HTTP returns `503 modem_unavailable` with `Retry-After`. Status endpoint serves last-known `kv` snapshot with `stale=true`.

USB-level disconnect (device disappears from sysfs) is handled the same way — `ttyx` notices via EIO on read, transitions to `Flapping`, reopen loop finds the device when udev re-creates `/dev/ttyUSB3`.

### 10.5 Modem reset detection

Three signals, any one fires:
1. `ttyx` reopen happens.
2. `RDY` URC observed at runtime.
3. `+CPIN: READY` URC after we already had READY in `kv`.

On detection: bump `epoch` in `kv`; drain pending atexec queue with `ErrModemReset`; re-run baseline config; `Reconcile(ctx)` synchronously; resume.

### 10.6 Reconciler

- Triggered on: boot, transport reconnect, modem-reset signal, every 30 s when idle, every 1 s while a call is open.
- Authoritative reads: `+CLCC`, `+CMGL=4`, `+CSQ`, `+COPS?`, `+CREG?`, `+CPIN?`, `+CBC`.
- For each diff applied: writes a `reconcile.diff` event with prior + new state. The history record tells the truth about both observation and correction.
- Boot-time invariant sweeps:
  - `state ∈ {ringing,dialing,active}` AND `created_at < now-1h` → mark ended `end_reason="reconciled_missing"`.
  - `state="submitted"` SMS older than 5 minutes with no `mr` → mark `indeterminate`.

### 10.7 At-most-once outbound semantics

- Outbound SMS that lost its `+CMGS:` ack is not auto-retried. State is `indeterminate` with a note. The carrier's `+CDS` (delivery report) may resolve it later if requested.
- Outbound call that lost its `OK` is not auto-redialed. The next `+CLCC` poll resolves whether a call exists.
- `Idempotency-Key` causes replay of the cached prior response, never a re-execute.

### 10.8 Graceful shutdown

Stop accepting HTTP, wait up to 30 s for in-flight requests, drain atexec queue, close TTY. Active calls are **not** auto-hungup; they survive daemon restart and the reconciler re-attaches via `+CLCC`.

### 10.9 Process-level robustness

- Handler `panic` → recovered, logged, `500 internal_error` with a request id; daemon does not exit.
- atexec / urc panic → daemon exits (these indicate corruption; let the supervisor restart).

## 11. Configuration

`sim7600d.toml` (path passed via `--config`; if omitted, all values come from flags/env with the defaults below). All paths are relative to the working directory unless overridden — the daemon ships as a single binary and assumes nothing about system layout.

```toml
[server]
bind = "127.0.0.1:8080"
# Auth token resolution order: $SIM7600D_AUTH_TOKEN → auth_token_file → generated at first start
# auth_token_file = "auth_token"             # one line, mode 0600

[modem]
tty  = "/dev/ttyUSB3"                        # or /tmp/sim7600 for the dev bridge
baud = 115200                                # informational on USB CDC

[storage]
path = "sim7600d.db"                         # relative to CWD by default

[retention]
events_days = 0                              # 0 = keep forever
sms_days    = 0
calls_days  = 0
idem_hours  = 24

[admin]
at_passthrough    = false
allow_modem_reset = false

[log]
level    = "info"
at_trace = false
```

CLI flags override file values (`--bind`, `--tty`, `--db`, `--at-trace`, …). On first start with no config and no token, the daemon writes `auth_token` next to the DB and logs the value once at info.

## 12. Dev ergonomics

### 12.1 Local dev rig (path B)

Three pieces, one-time per session:

```bash
# Remote: expose /dev/ttyUSB3 over a localhost TCP listener
ssh root@203.0.113.10 \
  'socat TCP-LISTEN:9300,bind=127.0.0.1,reuseaddr,fork \
         FILE:/dev/ttyUSB3,nonblock,raw,echo=0'

# Local Mac: forward the listener through SSH
ssh -N -L 9300:127.0.0.1:9300 root@203.0.113.10

# Local Mac: terminate the TCP into a PTY device file
socat PTY,link=/tmp/sim7600,raw,echo=0,mode=600 TCP:127.0.0.1:9300

# Run the daemon against the local PTY
go run ./cmd/sim7600d --tty /tmp/sim7600 --bind 127.0.0.1:8080 --at-trace
```

`socat` and `openssh` must be installed on the NixOS host (currently absent). Required setup is documented as a prereq, not automated by the project.

Latency over the SSH tunnel (~40–100 ms) is fine for AT commands; would be problematic for live audio (irrelevant for v1).

### 12.2 Verification on remote (path A)

```bash
GOOS=linux GOARCH=amd64 go build -o build/sim7600d ./cmd/sim7600d
scp build/sim7600d root@203.0.113.10:/tmp/sim7600d
ssh root@203.0.113.10 '/tmp/sim7600d --tty /dev/ttyUSB3 --bind 127.0.0.1:8080'
```

No installed paths, no service unit. Run the hardware acceptance suite against this binary periodically.

### 12.3 Hardware-free dev

`go run ./cmd/sim7600d --tty sim:happy_path` runs the daemon against the in-process simulator. Most feature work happens here.

## 13. Testing

### 13.1 Unit (most of the suite, no hardware)

- `atproto`: scanner classifies frames correctly under canned bytes; covers SIM7600 quirks (leading blank line, intermediate-then-URC interleave).
- PDU encoder/decoder: golden tests for GSM-7, UCS-2, multipart with UDH, 7-bit packed-alphabet edge cases.
- `urc`: subscribe/publish/fan-out, prefix matching, monotonic id.
- `store`: every query method against in-memory SQLite.
- `modem`: facade against scripted `atexec` fake.
- `api`: handlers against a `modem.Modem` fake. Auth, validation, idempotency replay, error mapping.

### 13.2 Integration (hardware-free, behind `-tags integration`)

Daemon talks to `simmodem` over a `socketpair`. Scenarios:
- `incoming_call_answered`, `incoming_call_missed`
- `outbound_sms_long_ucs2`, `inbound_sms_multipart_out_of_order`
- `modem_reset_mid_command`
- `tty_flap_during_call`
- `urc_during_pending_command`
- `idempotency_replay_after_crash` (DB persists across daemon restart in a tempdir)

### 13.3 Hardware acceptance (opt-in, `-tags hardware`)

Run against the live module via the dev TTY bridge:
- Status read returns plausible values.
- Send SMS to a known test number; confirm `+CMGS` mr; await delivery report.
- Dial a known test number, auto-hangup after 5 s, verify CDR.
- `AT+CFUN=1,1` reset, observe reconciler completes within 60 s.

Output committed as a markdown artifact for drift tracking.

### 13.4 TDD discipline

Each feature in the implementation plan follows: failing test → minimum impl → refactor. The simulator enables deterministic tests for race-y scenarios that real hardware cannot reproduce on demand.

## 14. Project layout (target)

```
/Users/xl/Documents/simcom7600/
├─ cmd/
│  └─ sim7600d/
│     └─ main.go
├─ internal/
│  ├─ ttyx/
│  ├─ atproto/
│  ├─ atexec/
│  ├─ urc/
│  ├─ modem/
│  ├─ store/
│  │  └─ migrations/
│  ├─ reconciler/
│  ├─ simmodem/
│  └─ api/
├─ docs/
│  ├─ reference/simcom/
│  │  ├─ README.md
│  │  ├─ SIM7500_SIM7600_AT_Command_Manual_v3.00.pdf          ← local, ignored
│  │  └─ SIM7500_SIM7600_AT_Command_Manual_v3.00.generated.md ← local, ignored
│  └─ superpowers/specs/
│     └─ 2026-05-10-sim7600-control-api-design.md   ← this file
├─ tools/
│  └─ extract_simcom_manual.py
├─ go.mod
└─ go.sum
```

## 15. Future work seams (explicitly preserved, not implemented)

- **Live audio (path B from §1)**: requires switching the module's USB PID to one that exposes USB-audio, enabling ALSA on the NixOS host, and adding `internal/audio` (ALSA → PCM resampling → WS frames). The HTTP API gains a `GET /v1/calls/<id>/audio` upgrade-to-WebSocket endpoint. The modem facade gains `AttachAudio(callID)` / `DetachAudio(callID)`. Nothing in v1 closes off this seam.
- **Push notifications**: webhooks or SSE could be added without changing the persistence model — both would consume from the existing `events` table.
- **Multiple modems**: `internal/modem` is interface-driven; a future `Manager` could pool modems and front them under a single API. v1 assumes one modem.
- **Data plane (PDP / QMI)**: out of scope; if added, would be a sibling daemon, not a layer on top of this one.

## 16. Glossary

- **AT command** — text protocol on `/dev/ttyUSB*`. 3GPP-standardised core (TS 27.005, TS 27.007) plus SIMCom extensions.
- **URC** — Unsolicited Result Code. Modem-initiated message (e.g. `RING`, `+CMTI:`).
- **PDU** — Protocol Data Unit. Binary SMS encoding (vs. text mode).
- **MR** — Message Reference. The 1-byte id `+CMGS:` returns; carrier delivery reports correlate via this.
- **CLCC** — `AT+CLCC` "list current calls"; authoritative call-state snapshot.
- **Epoch** — daemon-side counter bumped on modem reset; lets subscribers detect they may have missed events.
