# Web UI — Design

**Date:** 2026-05-11
**Status:** Draft (pre-implementation)
**Builds on:** [`2026-05-10-openapi-typescript-sdk-design.md`](2026-05-10-openapi-typescript-sdk-design.md)

## 1. Goal

Add a single-operator web UI for the SIM7600D control daemon that exposes
every `/v1/*` operation through a polished, user-friendly interface, consumes
the existing `@sim7600d/client` TypeScript SDK (so types cannot drift from the
running daemon), and is embedded directly in the `sim7600d` Go binary so that
the daemon ships as a single artifact and serves the UI at `/` by default.

Concretely:

- `make ui-dev` runs Vite with HMR and proxies API traffic to `:8080`.
- `make ui-build` produces `web/ui/dist/` consumed by `//go:embed`.
- `make build` builds the UI first, then the Go binary, so a fresh
  `./build/sim7600d` always ships with the matching bundle.
- `GET /` serves the SPA; `GET /v1/*` and `GET /openapi.json` are unaffected.

## 2. Non-goals

- Multi-user, multi-tenant, or role-based access. One operator, one shared
  bearer token (same model as today).
- Server protocol changes. No SSE, no WebSocket, no new endpoints. Live state
  is achieved by polling `GET /v1/events?since=...` and invalidating queries.
- Theme picker, internationalization, settings page, chart library, virtual
  scrolling, Storybook, or component library beyond shadcn/ui copy-ins.
- E2E browser testing (Playwright). Type-check, lint, and unit tests only in v1.
- Modifying `web/client/`. The SDK is a stable upstream dependency.

## 3. Top-level decisions

| Question | Decision | Why |
|---|---|---|
| Framework | React 18 + TypeScript + Vite | User preference; smallest viable React tooling. |
| Styling | Tailwind v4 | User preference. |
| Components | shadcn/ui (Radix-based copy-ins) | Polished, accessible, no runtime lock-in. |
| Routing | react-router-dom v6 | Deep links to specific SMS / call / event filters. |
| Server state | TanStack Query v5 | Caches, polls, invalidates. Standard pairing. |
| Client state | Zustand | Tiny slice — auth token, ephemeral UI flags. |
| SDK consumption | `file:../client` workspace dep | Single source of truth; no duplicated types. |
| Auth UX | Login screen + `?token=` shortcut | No leakage by default; convenience for bookmarks. |
| Live updates | Poll `/v1/events`, isolate behind `useLiveEvents` | No new server work; SSE-swap is one file. |
| Admin gating | Probe-and-disable on 404 | Mirrors server flag state without a new endpoint. |
| Page model | Sidebar + routes (5 areas) | API surface is too wide for a single screen. |
| Asset delivery | `//go:embed dist/*` in `web/ui/static.go` | Single binary; no second deploy artifact. |
| Dist tracking | `dist/` gitignored except a sentinel `index.html` | `go:embed` needs a file; CI/Make builds the real bundle. |
| Dev loop | Vite on :5173 with `/v1` proxy to :8080 | HMR, no Go rebuild for UI iteration. |

## 4. Directory layout

```
web/
  client/                    # unchanged — generated SDK
  ui/                        # NEW
    package.json             # depends on "@sim7600d/client": "file:../client"
    vite.config.ts           # proxy /v1, /openapi.json → 127.0.0.1:8080 in dev
    tsconfig.json
    index.html
    components.json          # shadcn/ui config
    src/
      main.tsx               # bootstrap: QueryClientProvider + RouterProvider
      App.tsx                # auth gate; renders LoginScreen or AppShell
      api/
        client.ts            # constructs Sim7600Client from current token
        queries.ts           # one hook per resource (useStatus, useSmsList, ...)
        live.ts              # useLiveEvents — polling now, SSE-ready boundary
      auth/
        AuthProvider.tsx     # token storage + 401 logout + ?token= bootstrap
        LoginScreen.tsx
      components/
        ui/                  # shadcn/ui copy-ins (Button, Dialog, Input, ...)
        AppShell.tsx         # sidebar + topbar + <Outlet/>
        StatusBadge.tsx, RelativeTime.tsx, EmptyState.tsx, CodeBlock.tsx
      routes/
        dashboard/index.tsx
        sms/index.tsx        # list + detail + Compose drawer
        calls/index.tsx      # list + detail + Dialer drawer + DTMF
        events/index.tsx     # filterable log, auto-tail
        admin/index.tsx      # reconcile / queue / vacuum / AT console / reset
      lib/
        idempotency.ts       # crypto.randomUUID()-based key
        cn.ts, format.ts
    static.go                # package uistatic — owns //go:embed dist/*
    dist/
      index.html             # sentinel only; real bundle overwrites
    .gitignore               # ignore dist/* except dist/index.html
```

## 5. Auth flow

1. On every page load, `AuthProvider` first reads `?token=…` from `window.location`.
   If present, it stores the token, then `history.replaceState`'s the param out
   of the URL.
2. Otherwise it reads `sim7600d_token` from `localStorage` (or `sessionStorage`
   if the user previously chose "do not remember on this device").
3. With no token, `<App/>` renders `<LoginScreen/>` and nothing else.
4. With a token, the SDK is constructed via `createClient({ baseUrl, headers:
   { Authorization: 'Bearer ' + token } })` and `<App/>` renders
   `<AppShell/><Outlet/>`.
5. A TanStack Query default `onError` (via `QueryCache` and `MutationCache`
   handlers) checks `error.status === 401` and calls `auth.logout()`, which
   clears storage and triggers a re-render to `<LoginScreen/>`.
6. `<LoginScreen/>` validates the pasted token by issuing a single
   `GET /v1/status` before persisting. 401 → inline error. 200 → store.

`baseUrl` defaults to `window.location.origin` (so the bundled deployment
"just works") and is overridable via `VITE_API_BASE` for the Vite dev server
(which proxies anyway, so this is mostly a documentation knob).

## 6. Live updates

`useLiveEvents()` is a single hook owned in `src/api/live.ts`. Behaviour:

- Holds a `lastEventId` ref (seeded from the most recent event seen on first load).
- Polls `GET /v1/events?since=<lastEventId>` every 2.5 seconds **only when**
  `document.visibilityState === 'visible'`. Hidden-tab polling stops.
- On a successful response, for each event it calls a small registered
  reducer that maps event `kind` → list of query keys to invalidate, e.g.:
  - `sms.arrived`, `sms.delivered`, `sms.failed` → `['sms']`
  - `call.ringing`, `call.answered`, `call.ended`, `call.updated` → `['calls']`, `['call', id]`
  - `modem.status` → `['status']`
- It updates `lastEventId` to the highest returned id.
- The events page subscribes to the same hook for display and appends events
  to a local ring buffer (cap 1000); older entries pushed out.

The "SSE later" swap: replace the polling body of `useLiveEvents` with an
`EventSource`. Reducer + invalidation surface is identical, so consumers do
not change.

## 7. Pages

### 7.1 Dashboard (`/`)
Status tiles: registration state, RSSI (text + small custom Tailwind bars,
no chart library), operator, IMEI, ICCID. Recent-events feed (last 20).
Quick actions: "Send SMS" (opens SMS Compose drawer), "Dial" (opens Calls
Dialer drawer).

### 7.2 SMS (`/sms`, `/sms/:id`)
Split-view. Left: list with filters (direction, free-text search, state),
infinite scroll backed by the `since` cursor. Right: detail pane (full body,
state transitions, timestamps, retry/delete actions). Floating "Compose"
button opens a drawer with `to`, `body`, and a generated idempotency key
(visible, copyable). Optimistic insert of the outbound message; query
invalidation reconciles state.

### 7.3 Calls (`/calls`, `/calls/:id`)
Split-view list + detail. Active calls pinned at the top of the list with
inline action buttons (Answer, Reject, Hangup) per state. Detail pane shows
direction, peer, state timeline, and — during an active call — a DTMF keypad
that dispatches `POST /v1/calls/{id}/dtmf` per press. Dialer drawer for
outbound: number input + Dial; on success, navigate to the new call's detail.

### 7.4 Events (`/events`)
Full-page filterable log. Kind multi-select, free-text grep over JSON,
`since` cursor input. Auto-tail toggle (on by default) auto-scrolls as new
events arrive. Click a row to expand the raw payload (CodeBlock).

### 7.5 Admin (`/admin`)
A grid of cards. Each card maps to one admin endpoint:

- **Reconcile** — single button; on success shows last-run summary.
- **Queue** — live diagnostic metrics (re-fetched on `useLiveEvents` tick).
- **Vacuum** — single button + confirm.
- **AT Console** — input + history of (cmd → lines → final code). Disabled
  with an inline tooltip ("Disabled: server started without
  `--allow-at-passthrough`") when the first probe call returns 404.
- **Modem Reset** — confirm dialog. Same probe-and-disable treatment.

The probe: on Admin mount, fire `POST /v1/admin/at` with an obviously-invalid
body (e.g., `{cmd: ""}`) and `POST /v1/admin/at-reset` (empty body). A 404
means the endpoint is not registered; any other status means it is. Results
cached for the session.

### 7.6 Login (`/login` — rendered when no token)
Single bearer-token textarea, "Remember on this device" checkbox (default
on), Submit button. Submit → validate via `GET /v1/status` → persist → reload.

## 8. Go server changes

### 8.1 New package `uistatic`

`web/ui/static.go`:

```go
package uistatic

import (
    "embed"
    "io/fs"
    "net/http"
    "strings"
)

//go:embed dist
var distFS embed.FS

func Handler() http.Handler {
    sub, _ := fs.Sub(distFS, "dist")
    fileServer := http.FileServer(http.FS(sub))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // SPA fallback: any path without a file extension serves index.html.
        if !strings.Contains(r.URL.Path, ".") && r.URL.Path != "/" {
            r.URL.Path = "/"
        }
        // Cache hashed assets aggressively, index.html never.
        if strings.HasPrefix(r.URL.Path, "/assets/") {
            w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
        } else {
            w.Header().Set("Cache-Control", "no-cache")
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

The `embed.FS` source lives at `web/ui/static.go` because `//go:embed`
patterns must be resolvable relative to the Go file. The package is imported
by `internal/api/api.go`.

### 8.2 Router mount

In `internal/api/api.go`, after the existing `/v1/*` registrations and the
`/openapi.json` handler:

```go
r.Handle("/*", uistatic.Handler())
```

Chi matches the more specific routes first, so `/v1/...` and `/openapi.json`
are unaffected. The catch-all serves the SPA for everything else, including
deep links like `/sms/abc123` (which the SPA fallback rewrites to
`index.html` so the React router takes over client-side).

### 8.3 Sentinel `dist/index.html`

`//go:embed dist` fails compilation if `dist/` is missing or empty. A
committed sentinel file at `web/ui/dist/index.html` (~20 lines, plain HTML,
"UI not built — run `make ui-build`") guarantees the binary always links.
`.gitignore` excludes the rest of `dist/`:

```
web/ui/dist/*
!web/ui/dist/index.html
```

Real builds overwrite `index.html` and create `dist/assets/...`.

### 8.4 Test

`internal/api/uistatic_test.go` (or extending an existing api test):

- `GET /` → 200, `Content-Type: text/html`, body contains the sentinel marker
  ("UI not built" in the default repo state, or the bundle's marker once
  built).
- `GET /v1/status` without auth → 401 (regression — UI mount did not break auth).
- `GET /openapi.json` → 200 (regression).

## 9. Build pipeline

Additions to the top-level `Makefile`:

```
ui-deps:
	npm --prefix web/ui install --no-audit --no-fund

ui-dev: ui-deps
	npm --prefix web/ui run dev

ui-build: ui-deps
	npm --prefix web/ui run build

ui-verify: ui-deps
	npm --prefix web/ui run typecheck
	npm --prefix web/ui run lint

build: ui-build
	$(GO) build $(LDFLAGS) -o build/sim7600d ./cmd/sim7600d
```

The `build` target depends on `ui-build` so a `make build` from a fresh
clone produces a binary that embeds the freshly built UI rather than the
sentinel. CI runs `make sdk-verify && make ui-verify && make build &&
make test`.

`web/ui/vite.config.ts` proxy configuration:

```ts
server: {
  proxy: {
    '/v1':           { target: 'http://127.0.0.1:8080', changeOrigin: true },
    '/openapi.json': { target: 'http://127.0.0.1:8080', changeOrigin: true },
  },
},
build: {
  outDir: 'dist',
  emptyOutDir: true,
  assetsDir: 'assets',
},
```

## 10. Testing

- **Vitest + React Testing Library** for the load-bearing isolated units:
  - `AuthProvider` — `?token=` bootstrap, URL strip, 401 logout, storage
    selection (local vs session).
  - `useLiveEvents` — visibility gating, `since` advancement, kind →
    invalidation routing (with a mock `QueryClient`).
  - `lib/idempotency` — UUID v4 format.
- **Go test** as described in §8.4.
- **CI**: `make sdk-verify && make ui-verify && make build && make test`.
  `sdk-verify` already catches OpenAPI drift; `ui-verify` catches type and
  lint drift; the build step proves the bundle still links into the binary.

## 11. Out of scope (explicit YAGNI)

- Per-user accounts, RBAC, audit logging in the UI.
- Theme picker, dark/light toggle (Tailwind v4 + system preference is fine).
- i18n.
- Charts. Status bars are hand-drawn divs.
- Virtualized lists. The cap of 1000 events + simple DOM is sufficient.
- Service worker, offline mode, PWA install.
- Storybook.
- Playwright / browser-driven E2E.

## 12. Open questions

None at design time. Implementation may surface ergonomic choices (exact
drawer behavior, list pagination size, etc.) that are local enough to be
made during the plan/PR.
