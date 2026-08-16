# Glyphflow v2 review TODO

Items are ordered by severity within each category. Each item maps to `REPORT.md`.

## Security

- [x] **Medium — SEC-01: Upgrade `golang.org/x/text` to v0.39.0 or later**
  - Verification: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./...`; `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./...`; `cd backend && /tmp/glyphflow-tools/govulncheck ./...`
  - Result: PASS — backend tests, vet, and vulnerability scan report no vulnerabilities.
  - Implementation commit: `00e97d0d857aa49c7c95b76ab835df6ab73c2940`
- [x] **Medium — SEC-02: Signal audit persistence failures**
  - Verification: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./...`; `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./...`
  - Result: PASS — append failures return to callers and production wiring emits structured logs with an error counter.
  - Implementation commit: `bf42d24ac5f24c121d0c0ee756f88ee93964f8a9`
- [x] **Medium — SEC-03: Reduce the system `user` role to least privilege**
  - Verification: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./...`; `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./...`
  - Result: PASS — default users retain read and run execution access; infrastructure mutation permissions move to the seeded operator role.
  - Implementation commit: `fcbe016c9f826e74e9e40768a3296fa5d0b0596a`
- [x] **Low — SEC-04: Add Go and npm vulnerability scans to the security gate**
  - Verification: `GOVULNCHECK_BIN=/tmp/glyphflow-tools/govulncheck ./scripts/security-check.sh`
  - Result: PASS — Go vulnerability scan reports zero vulnerabilities and npm audit reports zero vulnerabilities.
  - Implementation commit: `946ef4ebc1f0f88e2f06ec39b2b66d86d6b18bb7`

## Bugs

- [x] **High — BUG-01: Restore the mobile menu and drawer scrim**
  - Verification: `cd frontend && npm test -- --run`; `cd frontend && npm run typecheck`; `cd frontend && npm run build`
  - Result: PASS — the duplicate hide rule was removed and frontend tests, typecheck, and build pass.
  - Implementation commit: `3eeecc575c82452e3683ac6502ea4486bb01ac3d`
- [x] **High — BUG-02: Allow parent-cascade deletion of schedule versions**
  - Verification: `GOFLAGS=-buildvcs=false ./scripts/release-check.sh`; `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/store -run 'Schedule|Canonical'`
  - Result: PASS — migration 016 keeps updates immutable and allows parent schedule deletion.
  - Implementation commit: `bf42d24ac5f24c121d0c0ee756f88ee93964f8a9`
- [x] **Medium — BUG-03: Restore CSRF state after logout**
  - Verification: `cd frontend && npm test -- --run`; `cd frontend && npm run typecheck`
  - Result: PASS — logout triggers authentication bootstrap before the next login.
  - Implementation commit: `3eeecc575c82452e3683ac6502ea4486bb01ac3d`
- [x] **Medium — BUG-04: Align run-log rendering with `logs.read`**
  - Verification: `cd frontend && npm test -- --run`; `cd frontend && npm run typecheck`
  - Result: PASS — log panels render only when `logs.read` is present; the permission contract test passes.
  - Implementation commit: `3eeecc575c82452e3683ac6502ea4486bb01ac3d`
- [x] **Medium — BUG-05: Contain task editor tables**
  - Verification: `cd frontend && npm test -- --run`; `cd frontend && npm run build`
  - Result: PASS — all task-editor key/value tables reuse `.gf-table-wrap`.
  - Implementation commit: `3eeecc575c82452e3683ac6502ea4486bb01ac3d`
  - Follow-up implementation commit: `5923c8b7f4d53e5c20ddb967089de9237488e28d`
- [x] **Medium — BUG-06: Give schedule controls accessible names**
  - Verification: `cd frontend && npm test -- --run`; `cd frontend && npm run typecheck`
  - Result: PASS — schedule controls use explicit labels and TaskPicker exposes valid combobox/listbox relationships.
  - Implementation commit: `3eeecc575c82452e3683ac6502ea4486bb01ac3d`
- [x] **Medium — BUG-07: Isolate PostgreSQL test data**
  - Verification: `GOFLAGS=-buildvcs=false ./scripts/release-check.sh`; `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./...`
  - Result: PASS — the release gate creates a clean migrated database, fixture cleanup is ordered and asserted, and the PostgreSQL suite passes.
  - Implementation commit: `bf42d24ac5f24c121d0c0ee756f88ee93964f8a9`
- [x] **Low — BUG-08: Repair the README roadmap link**
  - Verification: `test -f internal/v2-review/TODO.md`; `git diff --check`
  - Result: PASS — README now links to the active v2 review TODO.
  - Implementation commit: `02fcaff863703da0e4d71390af8ad38fdcb4a014`

## New Features

- [x] **Medium — FEATURE-01: Show the placement blocker for waiting runs**
  - Verification: `GOFLAGS=-buildvcs=false ./scripts/release-check.sh`; `cd frontend && npm test -- --run src/run-pages.test.ts`; `cd frontend && npm run typecheck`
  - Result: PASS — waiting run list/detail records now expose a read-time blocker derived from current pool, selector, heartbeat, capacity, and resource state; terminal runs omit it.
  - Implementation commit: `b8d2d2c10e07434dd77e165f4aaa9caad9367547`

## Enhancements

- [x] **High — ENH-01: Add a focused browser acceptance suite**
  - Verification: `./dev_run.sh`; `cd frontend && npm run test:browser`
  - Result: PASS — all seven checks pass against `http://localhost:5173`: login/session bootstrap, logout-login CSRF, admin route, 390px navigation, 320px task editor, schedule accessible names, and run logs.
  - Implementation commit: `240cd6efc0a0d2971c4a28e65656b1fef9482228`
  - Follow-up implementation commit: `4dbf696359ef33ccf9836d3562cbcd62d968167e`
  - Follow-up implementation commit: `5923c8b7f4d53e5c20ddb967089de9237488e28d`
- [x] **Medium — ENH-02: Gate releases on a migrated PostgreSQL suite**
  - Verification: `GOFLAGS=-buildvcs=false ./scripts/release-check.sh`
  - Result: PASS — release checks run unit tests, apply every migration to a clean PostgreSQL database, execute the PostgreSQL suite, and tear the database down.
  - Implementation commit: `522859380f74699a97736e9aef80d0b3b1274122`
- [ ] **Medium — ENH-03: Complete the existing desktop tray matrix**
  - Test supported Linux and Windows environments from `internal/v2/TODO-2.md`.
  - Verification: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race -tags workerui ./cmd/worker`
  - Result: PASS — worker UI compiles and tests on Linux; blocked: full acceptance requires Windows WebView2 and a supported Linux StatusNotifierWatcher with valid enrollment, while this sandbox reports DBus `Operation not permitted`.
- [x] **Low — ENH-04: Remove deprecated Vite transform options**
  - Verification: `cd frontend && npm test -- --run`; `cd frontend && npm run build`
  - Result: PASS — Vitest no longer loads the React transform plugin, and test/build output is warning-free.
  - Implementation commit: `3eeecc575c82452e3683ac6502ea4486bb01ac3d`

## TODO-2 improvement carry-forward

The implementation handoff in [`internal/v2/TODO-2.md`](../v2/TODO-2.md) is now part of this review plan. The grouped entries below preserve every product improvement from that handoff; the source file remains the detailed acceptance checklist.

### Completed implementation improvements

- [x] **Frontend visual system (TODO-2 Phases 2–4):** Light, Dark, and Neon tokens; AI Platform-inspired cards, tables, forms, dialogs, status pills, pagination, loading/empty/error states, responsive 248/64 px grouped sidebar, Scheduler badge, workspace groups, permission-aware counts, active-route treatment, collapsed tooltips, account footer, three-choice Appearance dialog, mobile drawer focus handling, overview metrics/activity/quick links, admin users and roles, audit, settings/editor, and account tab layouts.
- [x] **Worker lifecycle (TODO-2 Phase 5):** Shared cancellable runner, headless entry point, safe stdout/stderr writer plumbing, preserved enrollment/recovery/consumer/heartbeat/shutdown behavior, and stage-labelled startup errors.
- [x] **Bounded worker status/log model (TODO-2 Phase 6):** Redacted endpoint, live configured capacity, ordered stdout/stderr entries, partial-line and CRLF handling, blank-line preservation, concurrency safety, 5,000-line retention, reset semantics, and text-only rendering.
- [x] **Tray-first desktop worker (TODO-2 Phase 7):** Wails worker UI, embedded Glyphflow assets, hidden startup window, Open/Exit tray menu, close/minimize-to-tray state machine, explicit one-shot shutdown, read-only snapshot API, accessible All/Stderr filters, polling/visibility handling, near-bottom auto-follow, and narrow-window layout.
- [x] **Desktop fallback and branding:** Linux StatusNotifierWatcher fallback to a normal window, platform-relative Wails asset route, runner identity display, and embedded Glyphflow icon.
- [x] **Release and compatibility work (TODO-2 Phase 8):** Pinned Wails dependency, desktop and headless artifacts with stable enrollment names, release compile checks, and documented WebView2/GTK/WebKit/headless requirements.
- [x] **Control-plane reliability improvements recorded by TODO-2:** runner/task-version deletion conflict messages, task soft deletion and retry blocking, pool archival, runner archival/cancellation and unique enrollment IDs, role count/table readability, and overdue dispatched-run reconciliation.

### Remaining TODO-2 acceptance improvements

- [x] **Baseline and reference capture (TODO-2 Phase 1):** record frontend/worker baselines, screenshot pattern notes, and the pre-edit worktree check.
  - Verification: `npm test -- --run`; `npm run typecheck`; `npm run lint`; `npm run build`; `GOCACHE=/tmp/glyphflow-gocache go test ./cmd/worker ./internal/worker ./internal/config ./internal/queue`; `GOCACHE=/tmp/glyphflow-gocache go vet ./...`; opened all seven reference screenshots.
  - Result: PASS — frontend tests (30 files, 60 tests), typecheck, lint, build, focused worker tests, and vet passed; screenshot sizes and recurring visual patterns are recorded in `REPORT.md`.
  - Implementation commit: `acf9d69a60db314bd848befa9a168d0009fbe4f2`
- [ ] **Full frontend acceptance (TODO-2 Phase 4.4 and 9.2):** compare all visible routes against screenshots at 100% zoom and 1650–1910, 1024, 768, 390, and 320 px; verify Light/Dark/Neon plus loading, empty, error/retry, forbidden, not-found, login, registration, dialogs, and live-log states; verify 320 px containment, 200% zoom, keyboard/focus/drawer behavior, long values, and Glyphflow-only labels/data.
- [ ] **Linux build portability (TODO-2 Phase 8):** encode the legacy GTK3 build tag when the supported distro requires it.
- [ ] **Native desktop tray matrix (TODO-2 Phase 9.3 / ENH-03):** on supported Linux and Windows, verify hidden startup, tray Open, minimize, close, endpoint redaction, live capacity changes, ordered All/Stderr logs, 5,000-line retention, invalid configuration handling, idle/active Exit shutdown, and headless launch without GUI dependencies.
- [ ] **Close-out (TODO-2 Phase 9.4):** check the source handoff's parent completion boxes only after every relevant acceptance item above passes.
