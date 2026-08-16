# Glyphflow v2 review

Audit date: 2026-08-16  
Revision: `391feaa` on `version/v0.1.0`

## Result

Glyphflow has a broad, coherent scheduler platform. The normal backend, frontend, integration, and release checks pass. The npm audit also passes.

The current revision is not ready for a v2 release. Mobile navigation is unavailable, and PostgreSQL rejects every schedule deletion. Other risks affect authentication, authorization, accessibility, audit reliability, and dependency security.

No critical issue was found. The previous `internal/v2/TODO.md` items are complete. `internal/v2/TODO-2.md` still contains valid manual acceptance work.

## Scope and method

The review covered the repository, the three v2 documents, generated contracts, migrations, tests, scripts, and both worker entry points. TokenSave indexed 367 files and guided the first structural pass. Direct source inspection verified each reported issue.

The live review used `./dev_run.sh`, Chromium, PostgreSQL, NATS, and the API at the documented local ports. It covered:

- 21 authenticated routes in Light, Dark, and Neon at desktop width: 63 route-theme checks.
- 21 routes at 320, 390, 768, 1024, and 1650 pixels: 105 route-width checks.
- Admin login, password-registration settings, registration, logout, and repeated login.
- A regular-user task creation and manual-run workflow.
- Permission-denied pages for regular-user access to audit and administration routes.
- Dialog Escape behavior and focus return.
- Focused PostgreSQL repository tests against fresh migrated databases.

The review did not run the native desktop tray matrix. The automated desktop build passed, but window, tray, long-log, and active-exit behavior still need a supported desktop session.

## Previous v2 status

`internal/v2/REPORT.md` is now historical. Its high-risk execution, protocol, authentication, and integration defects have corresponding completed items in `internal/v2/TODO.md`.

`internal/v2/TODO-2.md` accurately records the remaining manual work, and `internal/v2-review/TODO.md` now carries its implementation improvements and open acceptance work forward by phase. This review completed broad route, theme, viewport, registration, and role checks. It did not complete 200% zoom, every keyboard path, screenshot comparison, or the desktop tray matrix.

## Available features

### Control plane and data

- One Go control-plane process serves HTTP, scheduling, dispatch, event ingestion, retention, health, and heartbeat loops.
- PostgreSQL stores identities, RBAC, SSO, audit data, immutable task and schedule versions, runs, runners, resources, and durable execution history.
- NATS JetStream carries signed orders, events, cancellation, control messages, and dead-letter traffic.
- The execution protocol uses Ed25519 envelopes, domain separation, sequence validation, replay controls, and persisted worker keys.
- Run processing supports cron policies, retry, reconciliation, cancellation, concurrency, resource leases, selectors, secrets, environment values, and global variables.
- Password and OIDC authentication use sessions, CSRF protection, PKCE, nonce validation, rate limits, and role-based permissions.
- Administration covers users, roles, password settings, OIDC providers, audit events, runners, pools, resources, and execution status.

### Workers

- The headless worker uses an embedded enrollment package, a durable SQLite inbox and outbox, direct process execution, and streamed logs.
- The shared worker runtime supports parallel task execution, capacity changes, heartbeat recovery, cancellation, and clean shutdown.
- The Wails desktop worker supplies a tray-first window, bounded status and log models, and packaged platform assets.

### Web UI

- React, TypeScript, Vite, React Router, TanStack Query, Vitest, and shared native controls form the web client.
- The UI has dashboard, task, schedule, run, log, runner, pool, resource, execution-status, audit, user, role, SSO, authentication, and account screens.
- Light, Dark, and Neon themes load across all inspected routes.
- Loading, empty, denied, not-found, validation, retry, dialog, and correlation-ID states exist.
- Task versions, run attempts, logs, schedule previews, global variables, capacity controls, and audit details are exposed.

## Live workflow result

| Workflow | Result | Evidence |
|---|---|---|
| Admin login | PASS | The default development administrator opened all administration routes. |
| Enable password registration | PASS | Registration was initially disabled. The administrator enabled it and selected the `user` role. |
| Create regular user | PASS | `review-user-20260816@example.com` registered and received the configured role. |
| Regular-user authorization | PARTIAL | Admin and audit routes denied access, but the default user role has broad management rights. See SEC-03. |
| Create a task | PASS | The user created `glyphflow-review-task-20260816` with `/bin/echo` in the default pool. |
| Start a manual run | PARTIAL | The run entered `WAITING` because the runner was offline. Its log panels then returned 403. See BUG-04. |
| Logout, then login | FAIL | The immediate login returned `cross-site request rejected`. Reloading `/login` repaired the flow. See BUG-03. |
| Mobile navigation | FAIL | No navigation control was visible or exposed at 390 pixels. See BUG-01. |
| Appearance dialog | PASS | Escape closed the dialog and returned focus to the Appearance button. |

The review changed development data as requested. Password registration remains enabled. The review user, task, and waiting run remain in the development database.

## Security findings

### SEC-01 — Reachable `golang.org/x/text` denial of service

- **Severity Level:** Medium.
- **Description:** The backend resolves `golang.org/x/text` v0.37.0. Invalid UTF-8 can make affected normalization code loop forever.
- **Evidence:** `govulncheck ./...` reported reachable advisory `GO-2026-5970` through `pgxpool.New`. `backend/go.mod:35` selects v0.37.0. The [official Go advisory](https://pkg.go.dev/vuln/GO-2026-5970) fixes versions before v0.39.0.
- **Suggested Modification:** Upgrade `golang.org/x/text` to v0.39.0 or later. Run the complete backend and integration suites again.

### SEC-02 — Audit persistence failures are silent

- **Severity Level:** Medium.
- **Description:** Security audit events can be lost without an operator signal when PostgreSQL rejects or cannot store an event.
- **Evidence:** `backend/internal/api/audit.go:267` discards the error from `repository.Append`. A focused PostgreSQL test produced a foreign-key write error without an application-level signal.
- **Suggested Modification:** Record append failures through the existing structured logger and metric path. Add one test that asserts the failure signal.

### SEC-03 — The default regular-user role can manage infrastructure

- **Severity Level:** Medium.
- **Description:** A self-registered user can manage tasks, resources, and runners when registration assigns the system `user` role.
- **Evidence:** `backend/internal/platform/rbac.go:16` grants `tasks.manage`, `resources.manage`, and `runners.manage`. The review account received Operations and Infrastructure controls immediately after registration.
- **Suggested Modification:** Remove management permissions from the system `user` role. Use an explicit operator role for infrastructure changes.

### SEC-04 — The repository security check omits vulnerability scanners

- **Severity Level:** Low.
- **Description:** `scripts/security-check.sh` passed while `govulncheck` found a reachable advisory.
- **Evidence:** The security script returned PASS. `govulncheck ./...` returned exit code 3 for SEC-01. `npm audit --omit=dev` found zero vulnerabilities.
- **Suggested Modification:** Add `govulncheck ./...` and `npm audit --omit=dev` to the existing security script or release gate.

## Bugs

### BUG-01 — Mobile users cannot open navigation

- **Severity Level:** High.
- **Description:** The final CSS cascade hides the mobile menu and drawer scrim at all widths.
- **Evidence:** `frontend/src/index.css:245` and the first mobile query enable the controls. `frontend/src/index.css:377` hides them again, and the final mobile query does not restore them. Chromium showed no navigation button at 390 pixels.
- **Suggested Modification:** Delete the later duplicate hide rule, or restore both displays in the final mobile query. Keep one mobile rule set.

### BUG-02 — PostgreSQL blocks schedule deletion

- **Severity Level:** High.
- **Description:** Deleting a schedule cascades to its versions. The immutable-version trigger rejects those deletes.
- **Evidence:** `backend/internal/store/schedules.go:182` deletes the parent. `backend/migrations/001_canonical.sql:434` blocks `DELETE` on `schedule_versions`. The focused repository test failed with `schedule_versions is immutable`.
- **Suggested Modification:** Add a migration that blocks schedule-version updates only. Preserve parent cascade deletion and add a real PostgreSQL delete test.

### BUG-03 — Immediate login after logout fails CSRF validation

- **Severity Level:** Medium.
- **Description:** Logout clears the CSRF cookie, but the SPA keeps its previous CSRF state until a page reload.
- **Evidence:** `backend/internal/api/session_cookies.go:27-32` clears `glyphflow_csrf`. `frontend/src/shell.tsx:75` clears the profile without restoring public configuration. The next login returned `cross-site request rejected`; reload then succeeded.
- **Suggested Modification:** Refresh the authentication context after logout so the public configuration request issues a new CSRF cookie. Add a logout-login browser test.

### BUG-04 — Regular-user run pages request forbidden logs forever

- **Severity Level:** Medium.
- **Description:** Users can read and start runs, but the run page always opens log streams that require `logs.read`.
- **Evidence:** `UserPermissionCatalog` omits `logs.read`. The backend protects run logs with that permission. The review run showed two repeating `Log stream failed (403)` panels.
- **Suggested Modification:** Align the permission contract once. Grant run readers log access, or hide log panels unless `logs.read` is present.

### BUG-05 — Task editor tables overflow narrow pages

- **Severity Level:** Medium.
- **Description:** The task editor renders bare tables with a 42-rem minimum width. Their containers do not own horizontal scrolling.
- **Evidence:** `frontend/src/index.css:149` sets the minimum width. `frontend/src/task-editor.tsx` does not use the existing `.gf-table-wrap`. Chromium measured about 663 pixels of content at a 320-pixel viewport.
- **Suggested Modification:** Wrap the three editor tables with the existing `.gf-table-wrap` element. Do not add a second table component.

### BUG-06 — Schedule controls have no accessible names

- **Severity Level:** Medium.
- **Description:** Schedule inputs and selects expose empty names to the accessibility tree.
- **Evidence:** `FieldLabel` places an interactive tooltip button inside a wrapping label. Chromium reported unnamed task, name, offset, cron, policy, deadline, and concurrency controls.
- **Suggested Modification:** Use stable input IDs and explicit `htmlFor` labels. Keep tooltip buttons outside labels. Give TaskPicker complete combobox and listbox relationships.

### BUG-07 — PostgreSQL tests contaminate one another

- **Severity Level:** Medium.
- **Description:** The full database-backed suite can claim stale rows created by another test and report false dispatch failures.
- **Evidence:** Fresh focused dispatch and reconciliation tests passed. The full suite claimed a leftover `resource-run-*` row. Resource test cleanup deletes the resource before its run and ignores cleanup errors.
- **Suggested Modification:** Delete dependent runs before resources and assert cleanup errors. Use a fresh migrated database in the release gate.

### BUG-08 — The roadmap link is broken

- **Severity Level:** Low.
- **Description:** The README links to a file that does not exist.
- **Evidence:** `README.md:152` links to `internal/TODO.md`. Current plans are under `internal/v2` and `internal/v2-review`.
- **Suggested Modification:** Link the README to the active review TODO file.

## Potential new feature

### FEATURE-01 — Explain why a run is waiting

- **Severity Level:** Medium.
- **Description:** A waiting run shows no placement blocker. Operators must infer that its runner or resource is unavailable.
- **Evidence:** The review run remained `WAITING` with no runner and no explanation while the only runner was offline.
- **Suggested Modification:** Return the existing dispatcher rejection reason and show it beside `WAITING`. Do not create a second scheduling model.

## Enhancements

### ENH-01 — Add one browser acceptance suite

- **Severity Level:** High.
- **Description:** Unit and static tests did not detect the mobile menu, CSRF, log permission, overflow, or accessible-name defects.
- **Evidence:** The frontend has 58 passing tests, but the five defects reproduced in Chromium.
- **Suggested Modification:** Automate only critical workflows: login, logout-login, role routes, mobile navigation, task editing, run logs, and named form controls.

### ENH-02 — Make migrated PostgreSQL tests a release gate

- **Severity Level:** Medium.
- **Description:** Normal `go test ./...` skips repository behavior when `DATABASE_URL` is absent.
- **Evidence:** The normal suite passed. An isolated migrated database exposed BUG-02 and BUG-07.
- **Suggested Modification:** Start one clean PostgreSQL service, apply migrations, and run repository tests after unit tests.

### ENH-03 — Complete desktop worker acceptance

- **Severity Level:** Medium.
- **Description:** The desktop build passes, but real tray and window behavior remains unverified.
- **Evidence:** The open matrix in `internal/v2/TODO-2.md` covers tray clicks, minimize, close, logs, capacity, invalid configuration, exit, and headless launch.
- **Suggested Modification:** Run that existing matrix on supported Linux and Windows systems. Record results without creating another checklist.

### ENH-04 — Remove deprecated Vite transform options

- **Severity Level:** Low.
- **Description:** Frontend tests and builds print deprecation warnings for the esbuild and oxc transform options.
- **Evidence:** `npm test` and `npm run build` pass but print the warnings.
- **Suggested Modification:** Update the current Vite configuration to its supported option names during the next dependency update.

## Verification

| Command or check | Result |
|---|---|
| `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./...` | PASS |
| `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./...` | PASS |
| `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./...` | PASS |
| `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -tags=integration ./internal/integration` | PASS; external TLS NATS and PostgreSQL cases skipped without their variables. |
| `cd frontend && npm test` | PASS; 30 files and 58 tests. |
| `cd frontend && npm run lint` | PASS |
| `cd frontend && npm run build` | PASS; main JavaScript 356.78 kB, 106.78 kB gzip. |
| `./scripts/security-check.sh` | PASS |
| `./scripts/release-check.sh` | PASS |
| `npm audit --omit=dev --audit-level=low` | PASS; zero vulnerabilities. |
| `govulncheck ./...` | FAIL; one reachable advisory, SEC-01. |
| Fresh migrated PostgreSQL focused tests | Two dispatch tests PASS; schedule deletion and audit fixture tests FAIL. |
| Fresh migrated PostgreSQL full suite | FAIL; BUG-02, invalid audit fixture, and BUG-07. |
| `./dev_run.sh` and Chromium workflow | STARTED and exercised successfully; findings recorded above. |

The audit fixture failure is not a product finding by itself. `backend/internal/store/audit_test.go` uses actor ID `actor` without creating the required user row.

TokenSave reduced the initial structural review by about 117,333 tokens across the recorded queries. Its branch graph fell back to `main`, so direct inspection validated all evidence on `version/v0.1.0`.
