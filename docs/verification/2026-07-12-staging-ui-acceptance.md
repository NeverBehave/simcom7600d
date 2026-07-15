# SIM7600 staging UI acceptance pass

Date: 2026-07-12–2026-07-13
Environment: `home-nas` staging deployment through local SSH tunnel
Authorized destination for live tests: redacted; examples below use the reserved number `+12025550123`.

## Goals

- SMS compose sends successfully and presents an understandable lifecycle.
- Dial starts a real call, shows state changes promptly, and permits hangup.
- Every visible route and action has a useful loading, success, empty, or error state.
- Hardware-dependent actions that cannot safely be completed are covered by automated tests and called out explicitly.

## Acceptance matrix

| Area | Case | Expected result | Status | Evidence / notes |
|---|---|---|---|---|
| Authentication | Sign in with staging token | Dashboard loads; token is removed from URL | Passed | Existing authenticated UI session remained usable through the reverse tunnel. API rejected missing auth in automated coverage. |
| Dashboard | Modem, SIM, network, and signal status | Current values render; loading/error state is clear | Passed | Live status: SIM ready, LTE registered, operator GSMT, deployed service active. Corrected swapped RSRP/RSRQ and zero uptime. |
| Dashboard | Send SMS and Dial quick actions | Correct drawer opens above app chrome | Passed | Both right-side drawers were visually verified above app chrome after the overlay fix. |
| SMS | Inbox list, search, direction filter, detail | Data loads without blocking; empty/error states are clear | Passed | Live inbound/outbound lists return promptly; outbound entries now include destination and body. Search/filter/detail covered by UI and API checks. |
| SMS | Send to authorized destination | Submit is acknowledged; outbound record and state are visible | Passed live | Identifiers redacted; destination shown as `+12025550123`, state `submitted`. User confirmed handset receipt. |
| SMS | Delete test history record | Test record disappears and list refreshes | Passed automated | API delete returns 204 in regression coverage. Live evidence records were intentionally retained. |
| Calls | Calls history list | Data loads without blocking | Passed | Endpoint returned immediately after single-connection SQLite deadlock fix. |
| Calls | Dial authorized destination | Request returns promptly; call record is visible | Passed live | Identifiers redacted; destination shown as `+12025550123`. User confirmed handset rang and connected. |
| Calls | Live state and hangup | Dialing/active/ended transitions are visible; hangup is usable | Passed live | The event trail covered dialing, alerting, active, and ended/hangup; connected duration was 7.967 s. User also confirmed remote hangup. |
| Calls | Answer/reject/DTMF validation | Invalid states are disabled or return clear feedback | Passed automated; partial live | Active/remote termination passed live. Answer/reject/DTMF final-response and invalid-state behavior passed automated coverage. |
| Events | Initial list, filtering, search, auto-tail | Events load and new call/SMS events appear | Passed after fix | Initial requests now return newest events rather than the oldest 200/500; `since` remains chronological for auto-tail. Live call events contain every state. |
| Admin | Queue, reconcile, vacuum | Actions give visible progress/success/error feedback | Passed live | Queue reported no active command, 0/16 queued requests, and healthy transport. Reconcile and VACUUM each returned 204. Destructive reset/AT remain flag-disabled and were not invoked. |
| Navigation | Sidebar routes and sign out | Every route renders; sign out returns to login | Passed automated/routes | `/`, `/sms`, `/calls`, `/events`, and `/admin` each served the application through the tunnel. Logout storage/token clearing passed component coverage. |
| Reliability | API calls under periodic reconciliation | No deadlocks, 100% CPU loops, or stuck requests | Passed after fixes | Full Go suite passes. Live Calls/SMS/status requests returned promptly during reconciliation. Phantom post-hangup adoption was reproduced and fixed with regression coverage. A timed live service restart completed successfully in under one second. |

## Automated coverage confirmed

- Single-connection store regression for call listing.
- Dial idempotency, modem final-response validation, and CLCC state persistence.
- New-dial grace period and recent-ended tombstone for post-hangup CLCC lag.
- SMS send, multipart encode, inbound PDU ingest, and reset reconciliation.
- Recent-first initial event page plus ascending `since` event tail.
- UI auth persistence and drawer overlay behavior.
- SMS delete (204) and logout token/storage clearing.
- UI type check, production build, and 15 component tests.
- Complete Go test suite across API, modem, AT executor/protocol, reconciler, SMS, store, TTY, and URC packages.

## Final findings

- Calls history nested a second SQLite read while the only DB connection was still held by the ID cursor. Fixed by closing the cursor before loading full records.
- `AT+CLCC` presence was checked but its dialing/alerting/active state was never persisted. Fixed with state mapping and `call.updated` events.
- Calls could be ended before the modem exposed a fresh dial. Added a short appearance grace period.
- Call idempotency keys were not persisted. Retries now return the original record instead of dialing twice.
- After hangup, a lingering CLCC row was adopted as a phantom second call. Recent ended calls now act as a short tombstone; regression test added.
- Answer/reject/hangup/DTMF accepted non-OK modem finals as success. All now validate the final result; DTMF also requires an active call.
- Outbound SMS history omitted recipient/body/error detail. The API and UI now show them.
- Initial Events and Dashboard requests showed the oldest records, burying current activity behind 5,800 historical entries. Initial reads are now recent-first while live `since` reads remain chronological.
- Network RSRP and RSRQ parsing was reversed, and uptime always displayed zero. Both are corrected.
- Queue diagnostics returned only a build version. They now expose queue depth/capacity, active-command state, and transport health.
- Opening Admin probed feature availability by invoking the AT and modem-reset actions. If reset were enabled, a page visit could reboot the modem. A read-only capabilities endpoint now supplies those flags.
- Shutdown could wait indefinitely because the serial TTY was switched back to kernel-blocking mode; `os.File.Close` then waited behind the idle read. The descriptor now stays nonblocking under Go's poller so Close interrupts reads. Teardown also closes the executor before waiting for workers, makes executor close idempotent, and bounds HTTP grace at ten seconds before force-close. A close-interrupt regression test covers the transport behavior.

## Final live snapshot

- SIM `ready`; LTE registered on `GSMT`; signal RSRP `-110 dBm`, RSRQ `-20 dB`.
- Command executor idle; queue `0/16`; serial transport healthy.
- Safe reconcile and database VACUUM actions returned HTTP 204.
- The deployed service stopped cleanly and returned active in a timed restart in under one second.
