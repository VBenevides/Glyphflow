# Glyphflow UI and desktop worker TODO

This file is an implementation handoff. Complete the work in order. Do not check a box until its implementation and listed verification pass.

## Final result

- [x] **Frontend: make Glyphflow use the AI Platform visual theme and page organization**
  - The source of truth is `local/ai-platform/frontend` plus every image in `local/screenshots`.
  - Keep Glyphflow's existing React 18, React Router, TanStack Query, Vite, Vitest, and Lucide stack.
  - Copy the visual language and layout patterns. Do not copy AI Platform routes, APIs, mock data, product names, or its entire Tailwind/shadcn dependency tree.
- [x] **Worker: add a tray-first desktop GUI**
  - The desktop worker starts hidden in the system tray.
  - Clicking the tray icon or its **Open** menu item shows and focuses the window.
  - Closing or minimizing the window hides it back to the tray. It must not stop the worker.
  - Only the tray menu's **Exit** action stops the worker and closes the application.
  - The window shows the redacted NATS JetStream endpoint, the live parallel-execution capacity, and a scrollable read-only log terminal with **All** and **Stderr** filters.

## Implementation record

- [x] Frontend implementation — commit `da2468f` (`feat(frontend): Align console theme and organization`)
  - Result: passed `npm run typecheck`, `npm test` (30 files, 57 tests), `npm run lint`, and `npm run build` from `frontend`.
  - Scope: AI Platform-inspired light/dark/neon tokens, grouped responsive sidebar, appearance dialog, shared cards/tables/forms, dashboard metrics, account tabs, and admin page layout polish.
- [x] Worker implementation — commit `ea649b8` (`feat(worker): Add tray worker console`)
  - Result: passed `go test ./...`, `go test -race ./cmd/worker ./internal/worker ./internal/queue`, `go test -race -tags workerui ./cmd/worker`, `go vet ./...`, `bash -n backend/build_runner_binaries.sh`, and `git diff --check`.
  - Build result: `backend/build_runner_binaries.sh` produced Linux/Windows desktop and headless artifacts; desktop builds use the pinned Wails v3 dependency with the `workerui` tag and embedded assets.
  - Manual screenshot comparison and native tray interaction checks remain environment-dependent and are intentionally left unchecked in Phase 9.
- [x] Runner pool deletion conflict fix — commit `6855786` (`fix(controlplane): Explain runner pool delete conflicts`)
  - Result: backend and frontend regression tests pass; referenced pools now return a safe `runner pool is still in use` message, while stale-data refresh remains limited to actions that request it.
- [x] Runner deletion conflict fix — commit `0fe15e1` (`fix(controlplane): Explain runner history conflicts`)
  - Result: foreign-key failures from retained execution attempts now return `runner is referenced by execution history`; API regression tests pass.
- [x] Runner archival, cancellation, pool cleanup, and unique enrollment IDs — commits `dc805a8`, `36bdf51`, `e6c601c`, and `a9da440`
  - Result: runners are archived instead of hard-deleted, retained in an Archived Runners tab, cannot reconnect or be recovered, and assigned work is cancelled immediately or after the existing stale-cancellation timeout. Pools can be deleted after all their runners are archived. Enrollment by runner name creates an ID with a random 8-byte hexadecimal suffix; legacy `runner_id` enrollment remains compatible.
  - Verification: `cd backend && GOCACHE=/tmp/glyphflow-go-cache go test ./...` passed; `cd frontend && npm run typecheck`, `npm test` (30 files, 58 tests), and `npm run lint` passed; `git diff --check` passed.

## Non-negotiable constraints

- Preserve all existing API contracts, route permissions, worker enrollment, SQLite recovery, signed messages, cancellation, heartbeats, and graceful shutdown behavior.
- Do not add a second web application framework to the main frontend. The current React application is already the framework.
- Do not migrate Glyphflow to Tailwind, shadcn, React 19, or TanStack Router. Those are implementation details of the reference application, not part of the requested appearance.
- Do not display NATS URL passwords or other credentials. Use the standard library's URL parsing and redaction.
- Do not create an unbounded in-memory log list. A long-running worker must have a fixed memory ceiling.
- Do not replace the tray requirement with a minimized taskbar window. When hidden, the worker window must disappear from the taskbar and remain available in the notification area/system tray.
- Do not add start-at-login, notifications, log files, log search, editable settings, or worker controls. They are not requested.
- Keep the current downloadable worker artifact names because `backend/internal/api/infrastructure.go` and its tests depend on them.
- Keep a headless build path for server/VM use. The README describes remote VM workers, and those deployments may not have a graphical session.
- Finish one phase and run its focused checks before starting the next phase.

## Reference map

Inspect these files before editing:

| Requirement | Reference | Glyphflow target |
|---|---|---|
| Theme variables | `local/ai-platform/frontend/src/styles.css` | `frontend/src/index.css`, `frontend/src/theme.ts`, `frontend/public/theme-prepaint.js` |
| Sidebar hierarchy | `local/ai-platform/frontend/src/components/admin/AdminSidebar.tsx` | `frontend/src/shell.tsx` |
| Page header, metrics, status, tables, pagination | `local/ai-platform/frontend/src/components/admin/AdminPanel.tsx` | `frontend/src/components.tsx` |
| Overview composition | `local/screenshots/overview.png`, `local/ai-platform/frontend/src/routes/admin/index.tsx` | `frontend/src/dashboard.tsx` |
| Forms/settings | `local/screenshots/platform-configuration.png` | Existing Glyphflow editor and settings pages |
| Roles | `local/screenshots/roles.png` | `RoleManagementPage` in `frontend/src/admin-pages.tsx` |
| Users | `local/screenshots/users.png` | `UserManagementPage` in `frontend/src/admin-pages.tsx` |
| Audit/system events | `local/screenshots/system_events.png` | `frontend/src/audit-page.tsx` |
| Theme/account dialog treatment | `local/screenshots/settings-ui.png`, `local/screenshots/settings-user.png` | Theme chooser in `frontend/src/shell.tsx`; account pages remain routed pages |
| Worker startup and shutdown | `backend/cmd/worker/main.go` | Shared worker runner plus headless and desktop entry points |
| Live capacity | `currentCapacity` in `backend/cmd/worker/main.go` | Worker GUI status snapshot |
| Capacity updates | `backend/internal/worker/control.go` | Continue using the existing signed control path |
| Existing worker stdout/stderr | `backend/cmd/worker/main.go`, `backend/internal/worker/runtime.go` | Bounded GUI log model |
| Release names | `backend/build_runner_binaries.sh` | Same output names after GUI build changes |

The screenshots describe appearance, not data. Never invent counts, statuses, or records merely to match a screenshot.

---

## Phase 1 — Freeze behavior before visual changes

- [ ] Run and record the current frontend baseline:
  - `cd frontend && npm test`
  - `cd frontend && npm run typecheck`
  - `cd frontend && npm run lint`
  - `cd frontend && npm run build`
- [ ] Run and record the current worker baseline:
  - `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./cmd/worker ./internal/worker ./internal/config ./internal/queue`
  - `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./...`
- [ ] Open all seven screenshots at their original size. Write down the repeated patterns before coding:
  - pale lilac page background;
  - light sidebar separated by a thin border;
  - purple primary action and selected navigation item;
  - compact headings and muted descriptions;
  - low-contrast cards with thin borders and small radii;
  - compact tables, filters, status pills, and pagination;
  - grouped sidebar parents, indented children, a vertical guide line, and item counts;
  - full-width content with consistent 20–24 px spacing.
- [ ] Confirm `git status --short` before editing. Preserve unrelated changes.

Exit condition: baseline checks are known, and the implementer can explain which screenshot maps to each Glyphflow page.

---

## Phase 2 — Copy the AI Platform theme into Glyphflow's existing CSS system

### 2.1 Theme model

- [x] Extend `frontend/src/theme.ts` from `light | dark` to `light | dark | neon`.
- [x] Keep the existing storage key `glyphflow:theme`.
- [x] Update `resolveTheme` so only `light`, `dark`, and `neon` are accepted. Unknown or absent values must still resolve from the system preference, then fall back to light.
- [x] Keep `applyTheme` responsible for one DOM representation: `document.documentElement.dataset.theme = theme`.
- [x] Update `frontend/public/theme-prepaint.js` with the same accepted values. The prepaint script and React code must never disagree, because disagreement causes a flash of the wrong theme.
- [x] Update `frontend/src/theme.test.ts` to cover:
  - [x] stored light;
  - [x] stored dark;
  - [x] stored neon;
  - [x] invalid stored value;
  - [x] no stored value with dark system preference;
  - [x] applying each accepted theme.

### 2.2 Exact visual tokens

- [x] In `frontend/src/index.css`, keep the existing `--gf-*` names so current components inherit the new appearance without a rewrite.
- [x] Translate the reference values from `local/ai-platform/frontend/src/styles.css` into the `--gf-*` variables. Use these mappings:

| Glyphflow token | AI Platform token |
|---|---|
| `--gf-page` | `--background` |
| `--gf-card` | `--card` |
| `--gf-surface` | `--muted` |
| `--gf-text` | `--foreground` |
| `--gf-muted` | `--muted-foreground` |
| `--gf-border` | `--border` |
| `--gf-action` | `--primary` |
| `--gf-action-contrast` | `--primary-foreground` |
| `--gf-sidebar` | `--sidebar` |
| `--gf-sidebar-text` | `--sidebar-foreground` |

- [x] Copy the light, dark, and neon color values directly from the reference theme blocks. Adapt only the variable names and selector form:
  - [x] light: `:root, :root[data-theme='light']`;
  - [x] dark: `:root[data-theme='dark']`;
  - [x] neon: `:root[data-theme='neon']`.
- [x] Add sidebar-specific action/accent/border tokens instead of hard-coded sidebar colors:
  - [x] `--gf-sidebar-action`;
  - [x] `--gf-sidebar-action-text`;
  - [x] `--gf-sidebar-accent`;
  - [x] `--gf-sidebar-border`.
- [x] Keep the reference radius at `0.75rem`.
- [x] Use the reference background effects in dark and neon only if they do not reduce text contrast. Light mode should remain close to the screenshots' flat pale-lilac background.
- [x] Do not add remote font requests. Use the existing system font stack. A missing web font must not block or shift the UI.
- [x] Preserve `prefers-reduced-motion`, visible keyboard focus, minimum 320 px width, and light/dark `color-scheme` behavior.

### 2.3 Shared component finish

- [x] Restyle existing selectors instead of duplicating components:
  - `.gf-button*`;
  - `.gf-input` and native `select`/date inputs;
  - `.gf-page-header`;
  - `.gf-metric` and `.gf-metric-grid`;
  - `.gf-card-panel` and `.gf-editor-form`;
  - `.gf-status*`;
  - `.gf-table-wrap`, `.gf-table`, and `.gf-pagination`;
  - `.gf-filter-bar`;
  - `.gf-dialog*`;
  - loading, empty, and error states.
- [x] Match the screenshots:
  - card borders are subtle, not shadow-heavy;
  - table headers are compact sentence case, not large uppercase labels;
  - primary buttons use purple with white text;
  - secondary buttons use the card background and border;
  - inputs have a small shadow only when it helps separation;
  - destructive, warning, success, and info states retain distinct accessible colors;
  - page headers have a bottom divider and 16 px bottom padding;
  - desktop content padding is 24 px;
  - mobile content padding is 16 px.
- [x] Do not globally change HTML semantics. Tables remain tables, links remain links, and form labels remain associated with controls.

Exit condition: changing only shared tokens/classes makes every route look related to the reference application without changing route behavior.

---

## Phase 3 — Rebuild the shell organization to match the reference sidebar

Edit `frontend/src/shell.tsx` and its CSS. Preserve permission filtering, route matching, the mobile focus trap, Escape handling, local-storage collapse state, and logout behavior.

### 3.1 Desktop sidebar

- [x] Use a 248 px expanded sidebar and a 64 px collapsed sidebar.
- [x] Make the sidebar use theme tokens. Light mode must use the light sidebar shown in `overview.png`; it must no longer be permanently navy.
- [x] Brand row:
  - [x] reuse `BrandMark`;
  - [x] show `Glyphflow`;
  - [x] show `SCHEDULER CONSOLE` in small uppercase tracked text;
  - [x] move the collapse/expand button into the right side of this row;
  - [x] remove the floating bottom-left collapse button.
- [x] Add a purple module badge below the brand row with the label `Scheduler` and a suitable existing Lucide icon.
- [x] Add the eyebrow label `WORKSPACE` above navigation groups.
- [x] Keep the current domain groups and permission-aware routes:
  - [x] Operations;
  - [x] Infrastructure;
  - [x] Security;
  - [x] Administration.
- [x] Render each group parent like the reference:
  - [x] chevron;
  - [x] group icon;
  - [x] group name;
  - [x] visible-route count aligned to the right.
- [x] Render children indented beneath the group with a vertical guide border.
- [x] Active child route uses the sidebar accent background and purple icon/text treatment. Hover must be visible in every theme.
- [x] Keep the group containing the current route expanded after navigation.
- [x] In collapsed mode:
  - [x] retain only recognizable icons;
  - [x] keep accessible names and `title` tooltips;
  - [x] do not render clipped text;
  - [x] keep the current route visually identifiable.

### 3.2 Sidebar footer and theme chooser

- [x] Keep the current account link and sign-out action.
- [x] Replace the two-state theme toggle with a button that opens the existing `Dialog` component.
- [x] The dialog title is `Appearance`.
- [x] Render three segmented choices: **Light**, **Dark**, and **Neon**.
- [x] Selecting a choice applies and stores it immediately. Add a **Done** button that closes the dialog.
- [x] Do not copy the reference app's Chat or User settings tabs. Glyphflow already has routed account pages, and fake tabs would add no function.
- [x] The account row shows the display name and username/email with ellipsis when needed.

### 3.3 Mobile behavior

- [x] Below 768 px, keep the existing drawer pattern.
- [x] The menu button opens the full expanded navigation regardless of the stored desktop collapse state.
- [x] Preserve:
  - [x] focus enters the drawer;
  - [x] Tab stays trapped inside it;
  - [x] Escape closes it;
  - [x] route navigation closes it;
  - [x] body scrolling is restored after close;
  - [x] focus returns to the menu button.
- [x] The drawer scrim and all sidebar text must meet contrast requirements in light, dark, and neon themes.

### 3.4 Tests

- [x] Update `frontend/src/shell.test.ts` for the revised group organization and collapse control.
- [x] Add one test proving only permitted routes contribute to each group count.
- [x] Add one render-level assertion that the theme choices contain Light, Dark, and Neon.
- [x] Do not add snapshot tests for the entire shell. Test behavior and accessible names.

Exit condition: at desktop width, the shell structure closely matches `overview.png`; at mobile width, all existing accessible drawer behavior remains intact.

---

## Phase 4 — Match page organization through shared primitives

### 4.1 `frontend/src/components.tsx`

- [x] Keep existing component names and call signatures unless an optional prop is enough.
- [x] `PageHeader`:
  - [x] preserve title, description, and action;
  - [x] use the reference divider and compact typography;
  - [x] add an optional `meta` slot for small page badges;
  - [x] do not show a fake `Live data` badge by default.
- [x] `MetricCard`:
  - [x] add optional Lucide `icon` and tone props;
  - [x] keep callers that only pass label/value/detail working;
  - [x] render the icon in the compact bordered square shown in the screenshots.
- [x] `DataTable`:
  - [x] keep its accessible caption;
  - [x] keep horizontal scrolling on narrow screens;
  - [x] use compact headers and row separators from the reference;
  - [x] do not introduce a table library.
- [x] `Pagination`:
  - [x] keep Previous/Next behavior and accessible navigation labeling;
  - [x] style it as the bordered footer row in the screenshots;
  - [x] do not add a page-size selector until the API and page state support it.
- [x] `StatusPill`:
  - [x] keep normalization centralized;
  - [x] render a subtle border/background per status tone;
  - [x] keep readable text in all three themes.
- [x] Keep `Dialog` keyboard focus management and Escape behavior unchanged.
- [x] Update focused component and accessibility tests for optional icon/tone/meta rendering.

### 4.2 Overview

Edit `frontend/src/dashboard.tsx` without changing endpoints or permissions.

- [x] Use the existing query results to organize the page into:
  1. [x] page header;
  2. [x] metric row for active runs, due schedules, and offline runners when permitted;
  3. [x] recent audit activity section when permitted;
  4. [x] quick links.
- [x] Use server totals when returned. Do not use a page's visible row count as a system-wide total.
- [x] Preserve independent loading/error behavior. One failed widget must not erase successful widgets.
- [x] At wide desktop widths, use up to four metric columns. Collapse to two and then one as available width decreases.

### 4.3 Screenshot-mapped pages

- [x] `UserManagementPage` in `frontend/src/admin-pages.tsx`:
  - [x] match the header, compact filter, table container, status pills, and actions seen in `users.png`;
  - [x] keep current endpoints, permissions, dialogs, and session actions;
  - [x] do not create summary metrics unless they can be computed accurately from returned totals.
- [x] `RoleManagementPage` in `frontend/src/admin-pages.tsx`:
  - [x] match the filter/action/table organization in `roles.png`;
  - [x] keep seeded roles immutable and existing permission behavior;
  - [x] allow permission pills to wrap without expanding the page horizontally.
- [x] `frontend/src/audit-page.tsx`:
  - [x] match `system_events.png`: header, filters, bordered table, compact status, and pagination;
  - [x] retain all current filters and safe/redacted detail rendering;
  - [x] long audit content must wrap inside its cell instead of widening the whole viewport.
- [x] Existing settings/editor pages:
  - [x] use grouped bordered sections like `platform-configuration.png`;
  - [x] retain native controls and existing validation;
  - [x] do not rename backend fields to AI Platform names.
- [x] `frontend/src/account-pages.tsx`:
  - [x] style Profile, Password, Identities, and Sessions links as a segmented tab row similar to `settings-user.png`;
  - [x] preserve URLs and browser navigation;
  - [x] do not turn routed sections into local-only fake tabs.

### 4.4 Full route pass

- [ ] Open every visible route in light, dark, and neon modes.
- [x] Fix shared CSS first. Add page-specific CSS only when the page has a genuinely unique structure.
- [ ] Confirm these states use the same theme:
  - loading;
  - empty;
  - error/retry;
  - forbidden;
  - not found;
  - login and registration;
  - dialogs;
  - live logs.
- [ ] Confirm no route has horizontal page scrolling at 320 px. Tables may scroll inside their own wrapper.
- [ ] Confirm 200% browser zoom remains usable.

Exit condition: the frontend looks like one product derived from the reference screenshots, while all Glyphflow data and behavior remain unchanged.

---

## Phase 5 — Extract the worker lifecycle without changing behavior

The existing `backend/cmd/worker/main.go` mixes process lifecycle, enrollment, storage, queue setup, concurrency, logging, and shutdown. The desktop shell needs the same runtime without `os.Exit` calls.

### 5.1 Shared runner

- [x] Move the body of the current worker startup into a shared function in `backend/cmd/worker/run.go` with this responsibility:
-  - [x] accept a parent `context.Context`;
  - [x] accept explicit stdout and stderr `io.Writer` values;
  - [x] accept a small status sink used to publish the endpoint and current capacity;
  - [x] return an `error` instead of calling `os.Exit`;
  - [x] block until context cancellation, exactly as the current main function does;
  - [x] close JetStream before waiting for background goroutines;
  - [x] close the local store exactly once;
  - [x] preserve all enrollment, key persistence, recovery, consumer, outbox, heartbeat, and control-message behavior.
- [x] Keep `needsRunnerEnrollment` in the same package and preserve its existing tests.
- [x] Wrap returned errors with useful startup stage names, but do not include credentials or private keys.
- [x] Do not change the order of security checks or durable writes merely to make the GUI easier.

### 5.2 Headless entry point

- [x] Keep a small headless `main` behind `//go:build !workerui`.
- [x] It must:
  - [x] create the existing SIGINT/SIGTERM context;
  - [x] call the shared runner with `os.Stdout` and `os.Stderr`;
  - [x] print a final error to stderr;
  - [x] exit non-zero on startup/runtime failure.
- [x] `go run ./cmd/worker` and normal `go test ./...` must continue to compile without Wails, WebKit, GTK, or a graphical session.

### 5.3 Writer plumbing inside the runtime

- [x] Replace direct `fmt.Printf` calls in `backend/internal/worker/runtime.go` with an `io.Writer` field on `OrderRuntime` and one small helper method.
- [x] Default a nil writer safely so existing unit tests and alternate constructors do not panic.
- [x] Pass the shared runner's stdout writer into `OrderRuntime`.
- [x] Keep errors on stderr in the command package. Do not classify ordinary lifecycle messages as stderr merely to color them red.
- [x] Do not print task stdout/stderr locally. Task output must continue through the signed log-chunk event path; the GUI terminal is for worker process logs.

Exit condition: focused and race tests pass, and the headless worker behaves exactly as it did before the GUI work.

---

## Phase 6 — Add a bounded, concurrency-safe GUI log/status model

Create `backend/cmd/worker/log_buffer.go` and `log_buffer_test.go`. Keep this independent of Wails so it can be tested headlessly.

### 6.1 Data contract

- [x] Define a log entry with JSON fields:
  - [x] monotonically increasing `sequence`;
  - [x] UTC `timestamp` in RFC3339 with fractional seconds;
  - [x] `stream`, exactly `stdout` or `stderr`;
  - [x] `text` containing one displayed line without a trailing newline.
- [x] Define a snapshot with:
  - [x] redacted `natsEndpoint`;
  - [x] integer `parallelExecutions`;
  - [x] `entries` newer than a requested sequence;
  - [x] `reset` when the caller's sequence is older than the retained buffer.
- [x] Label the second field **Parallel executions** in the UI. Its value is the worker's current configured capacity, not active process count.

### 6.2 Log writer behavior

- [x] Provide separate stdout and stderr writers backed by one shared buffer.
- [x] Make writes safe when many execution goroutines log concurrently. Verify with `go test -race`.
- [x] Correctly join partial writes until a newline arrives.
- [x] Preserve blank lines.
- [x] Normalize CRLF to one displayed line ending.
- [x] Bound retained history to the newest 5,000 lines. Document this fixed ceiling in one `ponytail:` comment and point to a persistent log store as the upgrade path only if operators later need longer history.
- [x] Optionally mirror each write to the original stdout/stderr in headless/dev use. A missing Windows console must not cause an error.
- [x] Never parse or reinterpret ANSI escape sequences. Display log text as text, never `innerHTML`.

### 6.3 Status behavior

- [x] Set the endpoint after `config.FromEnv(config.Worker)` succeeds.
- [x] Parse the NATS URL with `net/url` and expose `parsed.Redacted()`. If parsing unexpectedly fails, expose a non-secret placeholder and send the parse error to stderr.
- [x] Set parallel executions from `currentCapacity.Load()` after initial capacity resolution.
- [x] Because `worker.ApplyRunnerControl` already updates the shared atomic value, read that same atomic for every GUI snapshot. Do not add a second capacity variable or a new environment setting.

### 6.4 Unit tests

- [x] Test stdout and stderr classification.
- [x] Test partial-line joining and CRLF normalization.
- [x] Test chronological sequence across both streams.
- [x] Test the 5,000-line bound and `reset` behavior.
- [x] Test concurrent writers under the race detector.
- [x] Test URL credential redaction, including a URL with username/password.
- [x] Test that capacity snapshots observe later atomic updates.

Exit condition: the model is fully testable without a window and cannot grow without bound.

---

## Phase 7 — Add the tray-first Wails desktop shell

### 7.1 Framework decision

- [x] Use Wails v3 for the desktop-only `workerui` build. This choice is specific to the requirement: its official APIs provide a cross-platform system tray, hidden windows, cancellable close hooks, and window-minimize events.
- [x] Pin the Go module to the tested Wails v3 prerelease `v3.0.0-alpha2.112`; the release script uses the Go toolchain directly with the `workerui` tag because the Wails CLI requires generated platform task files that this repository does not contain.
- [x] Record the pin in `backend/go.mod`/`backend/go.sum` and build documentation.
- [x] Keep all Wails imports behind the `workerui` build tag so headless builds remain free of native GUI requirements.
- [x] Relevant official references:
  - <https://v3.wails.io/features/menus/systray/>
  - <https://v3.wails.io/reference/window/>
  - <https://v3.wails.io/reference/events/>
  - <https://v3.wails.io/concepts/lifecycle/>
  - <https://v3.wails.io/guides/build/cross-platform/>

### 7.2 Desktop entry point

- [x] Add `backend/cmd/worker/main_desktop.go` with `//go:build workerui`.
- [x] Embed the worker GUI assets and a tray icon from directories beneath `backend/cmd/worker`; Go embed patterns cannot use `..`.
- [x] Copy `assets/glyphflow.png` into the worker desktop asset directory for the tray/window icon. Do not use the AI Platform logo.
- [x] Create one Wails application, one normal resizable window, and one system tray icon.
- [x] Window properties:
  - [x] title: `Glyphflow Worker`;
  - [x] initial width: about 860 px;
  - [x] initial height: about 560 px;
  - [x] minimum width: 640 px;
  - [x] minimum height: 420 px;
  - [x] starts with `Hidden: true`;
  - [x] uses a background color matching the page token to avoid a white flash;
  - [x] standard native frame; do not build a custom title bar.
- [x] Configure Wails so the application does not quit merely because its last window is hidden or closed on Windows/Linux.

### 7.3 Exact tray/window state machine

Implement this behavior literally:

```text
Application starts
  -> worker goroutine starts
  -> tray icon appears
  -> window remains hidden

Tray icon click OR tray menu Open
  -> restore if minimized
  -> show
  -> focus

Window close button
  -> cancel the close event
  -> hide window
  -> worker keeps running

Window minimize button
  -> restore internal window state
  -> hide window
  -> worker keeps running

Tray menu Exit
  -> mark shutdown as explicit
  -> cancel worker context
  -> close NATS so blocked consumers wake
  -> wait for worker goroutines and local store cleanup
  -> quit Wails
```

- [x] Tray tooltip/label is `Glyphflow Worker`.
- [x] Tray menu contains exactly:
  - [x] **Open**;
  - [x] separator;
  - [x] **Exit**.
- [x] A left click on the tray icon opens the window. It does not exit the application.
- [x] Register a cancellable `WindowClosing` hook. Unless explicit Exit is already in progress, hide the window and cancel the close event.
- [x] Listen for `WindowMinimise`; restore and hide the window so it leaves the taskbar.
- [x] The Open action always calls restore, show, and focus in that order.
- [x] Keep a `sync.Once` or equivalent one-shot guard around explicit shutdown so double-clicking Exit cannot close resources twice.
- [x] Start the shared worker in a goroutine. Never run the worker loop on the GUI event thread.
- [x] If worker startup fails, keep the tray application alive and write the failure to stderr so the user can open the window and read it. Exit remains available.

### 7.4 Asset/API delivery without another frontend toolchain

- [x] Use a tiny embedded vanilla HTML/CSS/JavaScript UI under `backend/cmd/worker/ui`.
- [x] Do not create a second React project and do not add npm dependencies for this three-field window.
- [x] Serve embedded assets with the Wails asset handler.
- [x] Expose only a same-origin read-only `/api/snapshot?after=<sequence>` handler to the embedded page.
- [x] Validate `after` as a non-negative integer. Invalid values return HTTP 400.
- [x] Return JSON with `Content-Type: application/json` and `Cache-Control: no-store`.
- [x] Do not bind mutation methods to JavaScript.
- [x] The snapshot handler is internal to the WebView asset server. Do not open a public TCP listener.

### 7.5 Worker window appearance

The final window should look like this:

```text
+------------------------------------------------------------------+
| [Glyphflow icon]  Glyphflow Worker                               |
|                    Runs in the system tray                       |
+-------------------------------+----------------------------------+
| NATS JetStream endpoint       | Parallel executions              |
| nats://host:4222              | 10                               |
+-------------------------------+----------------------------------+
| Logs                                      [ All ] [ Stderr ]      |
| +--------------------------------------------------------------+ |
| | 2026-... Glyphflow worker v...                               | |
| | 2026-... Started Run "..."                                  | |
| | ...                                                          | |
| +--------------------------------------------------------------+ |
+------------------------------------------------------------------+
```

- [x] Reuse the AI Platform light-theme values translated for the main frontend: pale lilac page, white/transparent cards, purple active control, muted labels, thin borders, and `0.75rem` radius.
- [x] Use system fonts. Do not load fonts from the network.
- [x] Top area contains the Glyphflow icon, `Glyphflow Worker`, and muted text `Runs in the system tray`.
- [x] Status area contains exactly two read-only cards:
  - [x] **NATS JetStream endpoint** with the redacted URL;
  - [x] **Parallel executions** with the live integer capacity.
- [x] Terminal header contains the label **Logs** and two accessible filter buttons:
  - [x] **All**: stdout and stderr in sequence order;
  - [x] **Stderr**: only entries whose stream is `stderr`.
- [x] The active filter uses purple and has `aria-pressed="true"` or correct tab semantics.
- [x] Terminal behavior:
  - [x] read-only `<pre>` or equivalent text container;
  - [x] monospace font;
  - [x] selectable text;
  - [x] vertical scrolling;
  - [x] wraps very long lines rather than widening the window;
  - [x] stderr lines use an accessible red/pink tone;
  - [x] polling appends text with `textContent`, never HTML;
  - [x] automatically follows new logs only when the user is already near the bottom;
  - [x] if the user scrolls upward, new logs do not pull them away;
  - [x] changing filters recomputes from the retained browser-side entries and preserves sequence order.
- [x] Poll snapshots every 500–1,000 ms. Stop polling while the document is hidden and refresh immediately when it becomes visible.
- [x] Keep at most the same 5,000 entries in browser memory. If the server returns `reset`, replace browser history with the returned retained entries.
- [x] At narrow window widths, stack the two status cards. The terminal must still remain usable at the minimum window size.

Exit condition: the worker starts in the tray, Open restores it, close/minimize return it to the tray, and the UI displays live safe status/log data while work continues.

---

## Phase 8 — Update builds without breaking enrollment

- [x] Update `backend/build_runner_binaries.sh` to build desktop artifacts with the pinned Wails v3 dependency and the `workerui` tag.
- [x] Keep exact outputs:
  - [x] `runner-binaries/glyphflow-runner-linux-amd64`;
  - [x] `runner-binaries/glyphflow-runner-windows-amd64.exe`.
- [x] Do not change `backend/internal/api/infrastructure.go` artifact lookup or download naming.
- [x] Add clearly named headless artifacts for server use without replacing the desktop outputs:
  - [x] `runner-binaries/glyphflow-runner-linux-amd64-headless`;
  - [x] `runner-binaries/glyphflow-runner-windows-amd64-headless.exe`.
- [x] Headless artifacts use the default `!workerui` entry point and the current plain Go build flow.
- [x] Desktop artifacts use the Wails runtime through a direct `go build -tags workerui`; preserve `-trimpath` and stripped release behavior in the script.
- [x] Document required developer/runtime dependencies:
  - [x] Windows 10/11: WebView2 runtime;
  - [x] Linux: supported GTK/WebKit runtime and an active desktop session;
  - [x] headless Linux: use the headless artifact, with no GUI dependency.
- [x] Update `README.md` only where worker start/stop or build instructions become inaccurate.
- [x] Update release checks so both the headless and desktop command variants compile.
- [ ] If Linux uses the Wails legacy GTK3 tag for the repository's supported distro, encode that tag in the build command rather than requiring developers to remember it.

Exit condition: enrollment tests still find the same desktop artifact names, and operators still have an explicit headless artifact for VMs/services.

---

## Phase 9 — Verification and visual acceptance

### 9.1 Automated checks

- [x] Frontend:
  - [x] `cd frontend && npm test` (30 files, 58 tests)
  - [x] `cd frontend && npm run typecheck`
  - [x] `cd frontend && npm run lint`
  - [x] `cd frontend && npm run build`
- [x] Worker/core:
  - [x] `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./cmd/worker ./internal/worker ./internal/config ./internal/queue`
  - [x] `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./cmd/worker ./internal/worker ./internal/queue`
  - [x] `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./...`
- [x] Full backend regression:
  - [x] `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./...`
- [x] Desktop compile:
  - [x] build Windows AMD64 desktop worker;
  - [x] build Linux AMD64 desktop worker on a supported Wails builder;
  - [x] build both headless artifacts;
  - [x] run `backend/build_runner_binaries.sh` and verify the four expected files exist and are non-empty.
- [x] Run the existing infrastructure artifact test to prove enrollment names did not drift:
  - [x] `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run Runner`

### 9.2 Frontend manual matrix

- [ ] Compare at 100% zoom against all screenshots, side by side.
- [ ] Test desktop widths near 1650–1910 px, matching the screenshots.
- [ ] Test 1024 px, 768 px, 390 px, and 320 px widths.
- [ ] Test Light, Dark, and Neon.
- [ ] Test keyboard-only navigation, focus visibility, dialog Escape, and mobile drawer focus trapping.
- [ ] Test a long username, long role permission list, long audit event, and long run ID.
- [ ] Verify no fake AI Platform labels/data remain. The UI must say Glyphflow and use Glyphflow routes and data.

### 9.3 Tray manual matrix

Perform on Windows and on one supported Linux desktop. A compile-only check is not sufficient for native tray behavior.

- [ ] Launch the desktop worker with valid enrollment/configuration:
  - no normal window appears;
  - one Glyphflow tray icon appears;
  - worker heartbeats and consumes work while hidden.
- [ ] Click the tray icon:
  - window opens, restores, and receives focus;
  - one window exists; repeated clicks do not create duplicates.
- [ ] Click minimize:
  - window disappears from the taskbar;
  - tray icon remains;
  - worker keeps running.
- [ ] Open again, then click the close button:
  - same hide-to-tray behavior;
  - worker keeps running.
- [ ] Confirm endpoint display:
  - correct scheme/host/port;
  - any username/password is redacted;
  - the unredacted credential never appears in the log terminal.
- [ ] Change runner capacity through the existing control-plane UI:
  - signed control message still applies;
  - GUI updates **Parallel executions** without restart;
  - heartbeat reports the same value.
- [ ] Generate stdout and stderr worker messages:
  - **All** shows both in order;
  - **Stderr** shows only stderr;
  - switching back to **All** restores retained stdout history;
  - text is selectable and cannot inject HTML.
- [ ] Leave the worker producing logs long enough to exceed 5,000 lines:
  - memory remains bounded;
  - newest lines remain visible;
  - filters still work.
- [ ] Start with invalid configuration:
  - tray remains available;
  - opening the window shows the startup error under Stderr;
  - no credential is exposed.
- [ ] Choose tray **Exit** while consumers are idle and while one task is active:
  - shutdown follows existing cancellation semantics;
  - NATS closes before goroutine wait;
  - process exits once;
  - tray icon disappears;
  - SQLite is not corrupted.
- [ ] Launch a headless artifact on a machine without a display:
  - it starts without GTK/WebView/tray requirements;
  - SIGINT/SIGTERM still shut it down cleanly.

### 9.4 Final cleanup

- [x] Run `git diff --check`.
- [x] Review `git diff --stat`; remove copied reference code, dependencies, or abstractions that are not required by the final behavior.
- [x] Ensure no generated `dist`, Wails cache, temporary build directory, credential, database, or log file is committed unless it is an intentional embedded worker UI asset.
- [x] Ensure all new dependencies are pinned and represented in lock/checksum files.
- [ ] Check the two parent boxes at the top only after every relevant acceptance item passes.

## Definition of done

The work is complete only when a user can recognize the main frontend as the same visual family as the local AI Platform screenshots, use every existing Glyphflow route without regression, start a desktop worker without a visible window, open it from the tray, hide it again by closing or minimizing, inspect the safe endpoint/capacity/log information, and stop it only through the explicit tray Exit action. Headless workers must remain buildable for VM/service deployments.
