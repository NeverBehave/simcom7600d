# OpenAPI Spec & TypeScript SDK — Design

**Date:** 2026-05-10
**Status:** Draft (pre-implementation)
**Builds on:** [`2026-05-10-sim7600-control-api-design.md`](2026-05-10-sim7600-control-api-design.md)

## 1. Goal

Replace the hand-written chi handlers in `internal/api/` with a code-first
OpenAPI 3.1 surface, then generate a typed TypeScript SDK from the resulting
spec into `web/client/`. The Go types in the handler layer become the single
source of truth; the spec and the SDK are derived artifacts that cannot drift
from the running daemon.

Concretely:

- `make openapi` regenerates `api/openapi.json` from Go.
- `make sdk` regenerates `web/client/src/` from `api/openapi.json`.
- `GET /openapi.json` (unauthenticated, mounted outside `/v1/`) serves the spec
  the daemon was built with.
- A drift test starts the daemon in-process and asserts that the served spec
  is byte-identical to the committed `api/openapi.json`.

## 2. Non-goals

- Changing the wire format. Existing JSON shapes (paths, fields, casing,
  timestamps, error envelope) are preserved exactly. This refactor tightens
  types; it does not redesign the API.
- Touching `internal/modem`, `internal/store`, the reconciler, or any layer
  below the HTTP boundary. The facade interface stays intact.
- Generating clients for other languages. v1 ships TypeScript only.
- Hosting Swagger UI / Redoc inside the daemon. The spec is served raw; the
  user can render it however they like.
- Frontend application code. `web/` exists only as the SDK's home; no
  framework, no build tool beyond `tsc`.
- API versioning beyond what already exists (`/v1/`). The SDK targets v1.

## 3. Top-level decisions

| Question | Decision | Why |
|---|---|---|
| Generator (Go) | [huma v2](https://huma.rocks) with `humachi` adapter | Code-first OAS 3.1, keeps chi router and existing middleware, validation via struct tags, Go types stay the source of truth. |
| Generator (TS) | [`@hey-api/openapi-ts`](https://heyapi.dev) | OAS 3.1 native; emits a typed client with operations as methods (`client.smsSend({...})`) and standalone TS types; ESM, framework-agnostic. |
| Spec format | OpenAPI 3.1, JSON | 3.1 is JSON-Schema 2020-12 compatible; JSON because tooling support is best and diffs are stable. |
| Spec location | `api/openapi.json` (committed) | Source of truth for SDK regeneration; reviewable in PRs. |
| Served at | `GET /openapi.json` (unauthenticated, mounted on the root router, not under `/v1/`) | Matches user choice. Doesn't leak secrets — the spec describes shape, not data. |
| SDK location | `web/client/` (own `package.json`, name `@sim7600d/client`, ESM only) | Self-contained, can be `npm link`'d into a frontend later without dragging the daemon repo along. |
| SDK auth | Bearer token interceptor passed at construction: `new Sim7600Client({ baseUrl, token })` | Mirrors how every `/v1/` route is auth'd today. |
| SDK build | Source-only (`src/` committed). Consumers run their own bundler/tsc. CI runs `tsc --noEmit` to type-check. | Avoids checked-in build artifacts and a verify step. Easy to add `dist/` later if a published-package shape is needed. |
| Versioning | SDK `version` mirrors daemon Git short SHA at build time | Regenerated each build; no human bookkeeping. |
| Operation IDs | Stable, snake_case-aware camelCase: `smsSend`, `smsList`, `callsDial`, `callsAnswer`, `adminAt` | These become SDK method names; lock them now. |

## 4. Architecture

```
                    ┌──────────────────────────────────────────────┐
                    │  internal/api  (Go)                          │
                    │                                              │
                    │   chi.Router  ←  humachi.New()  ←  huma.API  │
                    │       │                              │       │
                    │       │     register operations:     │       │
                    │       │     huma.Register(api,       │       │
                    │       │       Operation{...},        │       │
                    │       │       handlerFn)             │       │
                    │       │                              │       │
                    │   handlerFn(ctx, *Input) (*Output, error)    │
                    │       │                                      │
                    │       ▼                                      │
                    │   modem.Modem facade  (UNCHANGED)            │
                    └──────────────────────────────────────────────┘
                                │
                                │  build time:
                                │  go run ./cmd/openapi-dump > api/openapi.json
                                ▼
                       api/openapi.json  (committed)
                                │
                                │  build time:
                                │  npx @hey-api/openapi-ts -i api/openapi.json
                                ▼
                       web/client/src/  (TS source, committed)
                                │
                                │  build time:
                                │  npm --prefix web/client run build
                                ▼
                       web/client/dist/  (tsc output, committed)
```

The HTTP request path at runtime is unchanged from §6.7 of the parent design:
chi router, `/v1/` route group, bearer-token middleware, then a handler.
The handler shape changes from `http.HandlerFunc` to a typed
`func(ctx, *Input) (*Output, error)` registered through huma; chi still owns
routing and middleware.

## 5. Refactor scope

`internal/api/` today (~600 lines across 8 files): chi handlers, ad-hoc
validation, `map[string]any` for some responses, hand-written error envelope.

After:

- One `Input` struct per operation. Fields tagged with huma's `path`,
  `query`, `header`, `cookie`, `required`, `format`, `enum`, `pattern`,
  `minimum`/`maximum`, `doc` etc. The struct embeds the body for POSTs.
- One `Output` struct per operation. The body is in a `Body` field; status
  codes and content types are declared on the `Operation`.
- Validation that's currently inline (`normalizePhone`, `body required`,
  `digits regex`) splits into:
  - **Format-level** (e.g. `pattern`) → declared on the struct, runs in huma.
  - **Domain-level** (E.164 normalization via `phonenumbers`) → stays in the
    handler body since it requires Go logic, runs after huma validation.
- The `errors.go` envelope is replaced by a custom huma error transformer
  that emits the existing shape (`{"error":{"code","message","details"}}`)
  with the same HTTP status codes — so wire compatibility holds.
- `events.go` and `inboundJSON()` lose their `map[string]any` returns in
  favour of explicit response structs. This is the only wire-shape-touching
  change, and it's invisible (same field names, same JSON output).

The `Admin` config struct, the auth middleware, the modem facade, and every
package below `internal/api/` are untouched.

### 5.1 Operation inventory

`GET /openapi.json` is **not** a huma operation — it's a plain chi handler
that serves the file embedded via `//go:embed api/openapi.json`. Registering
it as a huma op would be circular (the spec lists itself) and would still
require choosing between "committed JSON" and "live registrations" as the
served bytes. Embedding the committed file gives one obvious answer: the
spec served is exactly the spec the SDK was built against.

| OpID | Method/Path | Auth | Notes |
|---|---|---|---|
| `statusGet` | GET `/v1/status` | bearer | Query: `refresh=1`. |
| `smsList` | GET `/v1/sms` | bearer | Query: `direction`, `since`, `limit`. Response is the **discriminated union** of inbound/outbound items per the existing shape — modeled in OAS 3.1 as `oneOf` on `items[]`. |
| `smsSend` | POST `/v1/sms` | bearer | Body: `to`, `body`, `delivery_report`. Header: `Idempotency-Key`. Query: `raw=1`. 202. |
| `smsGet` | GET `/v1/sms/{id}` | bearer | Path: ULID. |
| `smsDelete` | DELETE `/v1/sms/{id}` | bearer | 204. |
| `callsList` | GET `/v1/calls` | bearer | Query: `since`, `limit`. |
| `callsDial` | POST `/v1/calls` | bearer | Body: `to`. Header: `Idempotency-Key`. Query: `raw=1`. 201. |
| `callsGet` | GET `/v1/calls/{id}` | bearer | |
| `callsAnswer` | POST `/v1/calls/{id}/answer` | bearer | |
| `callsReject` | POST `/v1/calls/{id}/reject` | bearer | |
| `callsHangup` | POST `/v1/calls/{id}/hangup` | bearer | |
| `callsDTMF` | POST `/v1/calls/{id}/dtmf` | bearer | Body: `digits` (pattern `[0-9A-D*#]+`), `duration_ms`. 204. |
| `eventsList` | GET `/v1/events` | bearer | Query: `since` (int64), `kind`, `limit`. |
| `adminReconcile` | POST `/v1/admin/reconcile` | bearer | 204. |
| `adminQueue` | GET `/v1/admin/queue` | bearer | Body is open `additionalProperties`; intentional — diagnostic. |
| `adminAT` | POST `/v1/admin/at` | bearer | Flag-gated; only registered if `cfg.AllowATPassthrough`. |
| `adminATReset` | POST `/v1/admin/at-reset` | bearer | Flag-gated; only registered if `cfg.AllowModemReset`. |
| `adminVacuum` | POST `/v1/admin/vacuum` | bearer | 204. |

Flag-gated operations are *omitted from the spec entirely* when their flag is
false — the daemon's running configuration determines the spec it serves.
The committed `api/openapi.json` is built with both flags **on** so the SDK
always exposes them; consumers see a 503 if the running daemon disabled them.

### 5.2 Error envelope

huma's default error type is replaced by:

```go
type APIError struct {
    Status  int    `json:"-"`
    Code    string `json:"code"`
    Message string `json:"message"`
    Details any    `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Message }
func (e *APIError) GetStatus() int { return e.Status }

// huma calls this; we wrap to match the existing envelope.
huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
    return &apiErrorEnvelope{Error: APIError{Status: status, Code: codeFor(status), Message: message}}
}
```

Wire-shape stays `{"error":{"code","message","details"}}`. The `code` mapping
table from §9.6 of the parent design is honored unchanged.

## 6. Repository layout (target)

```
/Users/xl/Documents/simcom7600/
├─ cmd/
│  ├─ sim7600d/                ← unchanged
│  └─ openapi-dump/            ← NEW: prints api/openapi.json to stdout
│     └─ main.go
├─ internal/
│  ├─ api/                     ← REFACTORED: huma operations
│  │  ├─ api.go                  router + huma registration
│  │  ├─ errors.go               huma error transformer
│  │  ├─ status.go               operation + I/O structs
│  │  ├─ sms.go                  same
│  │  ├─ calls.go                same
│  │  ├─ events.go               same
│  │  ├─ admin.go                same (flag-gated registration)
│  │  ├─ openapi.go              NEW: //go:embed api/openapi.json + serve
│  │  └─ *_test.go               existing tests adapted
│  └─ … (everything else unchanged)
├─ api/
│  └─ openapi.json             ← NEW, committed, regenerated by `make openapi`
├─ web/
│  └─ client/                  ← NEW
│     ├─ package.json            name "@sim7600d/client", ESM, sideEffects:false
│     ├─ tsconfig.json           target ES2022, moduleResolution bundler
│     ├─ openapi-ts.config.ts    @hey-api/openapi-ts config
│     ├─ src/                    GENERATED — do not hand-edit
│     │  ├─ types.gen.ts
│     │  ├─ services.gen.ts
│     │  ├─ schemas.gen.ts
│     │  └─ client.gen.ts
│     ├─ index.ts                public surface: re-exports + Sim7600Client
│     └─ README.md               install + usage snippet
└─ Makefile                    ← extended: openapi, sdk, sdk-verify
```

The `api/` directory at repo root is new. Spec lives there (not in `docs/`)
because it's a build artifact, not documentation.

## 7. Build pipeline

### 7.1 Targets

```makefile
# Regenerate api/openapi.json from the running daemon's registered ops.
openapi:
	$(GO) run ./cmd/openapi-dump > api/openapi.json

# Regenerate web/client/src from api/openapi.json.
sdk: openapi
	npm --prefix web/client install --no-audit --no-fund
	npm --prefix web/client run gen
	npm --prefix web/client run typecheck

# CI guard: regenerate and fail if anything moved.
sdk-verify: sdk
	git diff --exit-code api/openapi.json web/client/src
```

`cmd/openapi-dump` builds the same `api.NewRouter()` it would in production
(with both admin flags forced on for spec coverage), then writes
`huma.OpenAPI().YAML()`-style JSON to stdout. No HTTP server starts.

### 7.2 Drift test

`internal/api/openapi_drift_test.go` (Go test, no build tag):

1. Build the huma API with both admin flags on, exactly as
   `cmd/openapi-dump` does.
2. Marshal the live registrations to JSON.
3. Compare to the contents of `api/openapi.json` on disk.
4. Fail if not byte-identical, with the diff in the failure message.

This catches the "I added an op and forgot `make openapi`" case at `go test`
time without depending on Make, npm, or starting an HTTP server. (The served
endpoint just returns the embedded file bytes; testing the embed → response
path is a separate, trivial test.)

### 7.3 SDK shape

Public surface (`web/client/index.ts`):

```ts
import { client as fetchClient } from "./src/client.gen";
import * as services from "./src/services.gen";
import type * as types from "./src/types.gen";

export type * from "./src/types.gen";

export class Sim7600Client {
  constructor(opts: { baseUrl: string; token: string; fetch?: typeof fetch }) {
    fetchClient.setConfig({
      baseUrl: opts.baseUrl,
      headers: { Authorization: `Bearer ${opts.token}` },
      fetch: opts.fetch,
    });
  }
  status   = services.statusGet;
  sms      = { list: services.smsList, send: services.smsSend, get: services.smsGet, delete: services.smsDelete };
  calls    = { list: services.callsList, dial: services.callsDial, get: services.callsGet,
               answer: services.callsAnswer, reject: services.callsReject, hangup: services.callsHangup,
               dtmf: services.callsDTMF };
  events   = { list: services.eventsList };
  admin    = { reconcile: services.adminReconcile, queue: services.adminQueue,
               at: services.adminAT, atReset: services.adminATReset, vacuum: services.adminVacuum };
}
```

A consumer writes:

```ts
import { Sim7600Client } from "@sim7600d/client";

const sim = new Sim7600Client({ baseUrl: "http://127.0.0.1:8080", token: "..." });
const { data, error } = await sim.sms.send({ body: { to: "+15551234567", body: "hi" } });
if (error) throw new Error(error.error.message);
console.log(data.id, data.state);
```

Errors come through as the typed `APIError` envelope; no exceptions thrown by
the SDK — `{ data, error }` discrimination follows the @hey-api convention.

## 8. Testing

| Layer | Test |
|---|---|
| Operation registration | Unit test per file: build a `humatest.New(...)` API, register the op, hit it with a fake `modem.Modem`. (Replaces existing `*_test.go` shape with minimal churn — they already use a `fakeModem`.) |
| Error envelope | Table test: every status in §9.6 of the parent design produces the exact `{"error":{"code","message"}}` body. |
| Drift | `openapi_drift_test.go` per §7.2. |
| `cmd/openapi-dump` | Smoke test: runs the binary, parses output as JSON, asserts at least the expected `paths` keys. |
| SDK type-check | `web/client` `tsc --noEmit` runs in CI. (Compiling is itself the test for "the spec is well-formed enough to generate from".) |
| SDK runtime | A small node test (under `web/client/test/`) that hits a `httptest.NewServer` of `api.NewRouter` and exercises `sms.send` + `sms.list`. Verifies the auth header, base URL, and error mapping wire correctly. |

Hardware-dependent tests (existing `-tags hardware`) are unaffected.

## 9. Migration order

The refactor is mechanical. Order matters only because each handler's tests
need to pass before moving on, so we don't accumulate broken state.

1. Add huma + humachi to `go.mod`.
2. Set up `internal/api/api.go` to construct a `humachi.New` over the chi
   router. **No operations registered yet.** Existing `chi` `r.Get/.Post`
   calls coexist; both styles work side-by-side during migration.
3. Migrate `statusGet` (smallest, no body). Verify tests pass. Remove old
   chi handler.
4. Migrate `smsSend`, `smsList`, `smsGet`, `smsDelete`.
5. Migrate `callsList`, `callsDial`, `callsGet`, `callsAnswer`, `callsReject`,
   `callsHangup`, `callsDTMF`.
6. Migrate `eventsList`.
7. Migrate `adminReconcile`, `adminQueue`, `adminAT`, `adminATReset`,
   `adminVacuum`.
8. Wire huma error transformer; remove `errors.go`'s `writeError`/`writeJSON`.
9. Add `cmd/openapi-dump`. Generate `api/openapi.json`. Commit.
10. Add `internal/api/openapi.go` (embed + handler). Mount on root router
    before `/v1/`.
11. Add drift test. Verify it passes.
12. Scaffold `web/client/`. Generate SDK. Commit `src/` only.
13. Add SDK runtime test (node, against `httptest.NewServer`).
14. Wire `Makefile` targets (`openapi`, `sdk`, `sdk-verify`).

Each step is its own commit, each commit passes `go test ./...`.

## 10. Future seams (preserved)

- **WebSocket events / live audio**: huma supports SSE; WebSocket lives outside
  the OpenAPI spec by convention. When live audio lands (per §15 of the parent
  design), the WS upgrade endpoint stays out of the SDK; types for its message
  envelope live in `web/client/realtime.ts` hand-written next to the generated
  code.
- **More clients (Python, Rust)**: nothing in the design assumes TypeScript;
  any OAS 3.1 generator can target the same `api/openapi.json`.
- **API v2**: `/v2/` would be a sibling chi route group with its own huma API;
  the SDK gains a `Sim7600ClientV2` namespace. `/openapi.json` becomes a
  `/openapi/v1.json` + `/openapi/v2.json` split.
- **Frontend app under `web/`**: `web/client/` is a sibling-friendly package;
  `web/app/` (or wherever) imports it via workspace.
