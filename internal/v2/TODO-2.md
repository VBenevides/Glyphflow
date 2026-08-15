# Glyphflow UI and desktop worker TODO

This file is an implementation handoff. Complete the work in order. Do not check a box until its implementation and listed verification pass.

## Final result

- [ ] **Frontend: make Glyphflow use the AI Platform visual theme and page organization**
  - The source of truth is `local/ai-platform/frontend` plus every image in `local/screenshots`.
  - Keep Glyphflow's existing React 18, React Router, TanStack Query, Vite, Vitest, and Lucide stack.
  - Copy the visual language and layout patterns. Do not copy AI Platform routes, APIs, mock data, product names, or its entire Tailwind/shadcn dependency tree.
- [ ] **Worker: add a tray-first desktop GUI**
  - The desktop worker starts hidden in the system tray.
  - Clicking the tray icon or its **Open** menu item shows and focuses the window.
  - Closing or minimizing the window hides it back to the tray. It must not stop the worker.
  - Only the tray menu's **Exit** action stops the worker and closes the application.
  - The window shows the redacted NATS JetStream endpoint, the live parallel-execution capacity, and a scrollable read-only log terminal with **All** and **Stderr** filters.

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

- [ ] Extend `frontend/src/theme.ts` from `light | dark` to `light | dark | neon`.
- [ ] Keep the existing storage key `glyphflow:theme`.
- [ ] Update `resolveTheme` so only `light`, `dark`, and `neon` are accepted. Unknown or absent values must still resolve from the system preference, then fall back to light.
- [ ] Keep `applyTheme` responsible for one DOM representation: `document.documentElement.dataset.theme = theme`.
- [ ] Update `frontend/public/theme-prepaint.js` with the same accepted values. The prepaint script and React code must never disagree, because disagreement causes a flash of the wrong theme.
- [ ] Update `frontend/src/theme.test.ts` to cover:
  - stored light;
  - stored dark;
  - stored neon;
  - invalid stored value;
  - no stored value with dark system preference;
  - applying each accepted theme.

### 2.2 Exact visual tokens

- [ ] In `frontend/src/index.css`, keep the existing `--gf-*` names so current components inherit the new appearance without a rewrite.
- [ ] Translate the reference values from `local/ai-platform/frontend/src/styles.css` into the `--gf-*` variables. Use these mappings:

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

- [ ] Copy the light, dark, and neon color values directly from the reference theme blocks. Adapt only the variable names and selector form:
  - light: `:root, :root[data-theme='light']`;
  - dark: `:root[data-theme='dark']`;
  - neon: `:root[data-theme='neon']`.
- [ ] Add sidebar-specific action/accent/border tokens instead of hard-coded sidebar colors:
  - `--gf-sidebar-action`;
  - `--gf-sidebar-action-text`;
  - `--gf-sidebar-accent`;
  - `--gf-sidebar-border`.
- [ ] Keep the reference radius at `0.75rem`.
- [ ] Use the reference background effects in dark and neon only if they do not reduce text contrast. Light mode should remain close to the screenshots' flat pale-lilac background.
- [ ] Do not add remote font requests. Use the existing system font stack. A missing web font must not block or shift the UI.
- [ ] Preserve `prefers-reduced-motion`, visible keyboard focus, minimum 320 px width, and light/dark `color-scheme` behavior.

### 2.3 Shared component finish

- [ ] Restyle existing selectors instead of duplicating components:
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
- [ ] Match the screenshots:
  - card borders are subtle, not shadow-heavy;
  - table headers are compact sentence case, not large uppercase labels;
  - primary buttons use purple with white text;
  - secondary buttons use the card background and border;
  - inputs have a small shadow only when it helps separation;
  - destructive, warning, success, and info states retain distinct accessible colors;
  - page headers have a bottom divider and 16 px bottom padding;
  - desktop content padding is 24 px;
  - mobile content padding is 16 px.
- [ ] Do not globally change HTML semantics. Tables remain tables, links remain links, and form labels remain associated with controls.

Exit condition: changing only shared tokens/classes makes every route look related to the reference application without changing route behavior.

---

## Phase 3 — Rebuild the shell organization to match the reference sidebar

Edit `frontend/src/shell.tsx` and its CSS. Preserve permission filtering, route matching, the mobile focus trap, Escape handling, local-storage collapse state, and logout behavior.

### 3.1 Desktop sidebar

- [ ] Use a 248 px expanded sidebar and a 64 px collapsed sidebar.
- [ ] Make the sidebar use theme tokens. Light mode must use the light sidebar shown in `overview.png`; it must no longer be permanently navy.
- [ ] Brand row:
  - reuse `BrandMark`;
  - show `Glyphflow`;
  - show `SCHEDULER CONSOLE` in small uppercase tracked text;
  - move the collapse/expand button into the right side of this row;
  - remove the floating bottom-left collapse button.
- [ ] Add a purple module badge below the brand row with the label `Scheduler` and a suitable existing Lucide icon.
- [ ] Add the eyebrow label `WORKSPACE` above navigation groups.
- [ ] Keep the current domain groups and permission-aware routes:
  - Operations;
  - Infrastructure;
  - Security;
  - Administration.
- [ ] Render each group parent like the reference:
  - chevron;
  - group icon;
  - group name;
  - visible-route count aligned to the right.
- [ ] Render children indented beneath the group with a vertical guide border.
- [ ] Active child route uses the sidebar accent background and purple icon/text treatment. Hover must be visible in every theme.
- [ ] Keep the group containing the current route expanded after navigation.
- [ ] In collapsed mode:
  - retain only recognizable icons;
  - keep accessible names and `title` tooltips;
  - do not render clipped text;
  - keep the current route visually identifiable.

### 3.2 Sidebar footer and theme chooser

- [ ] Keep the current account link and sign-out action.
- [ ] Replace the two-state theme toggle with a button that opens the existing `Dialog` component.
- [ ] The dialog title is `Appearance`.
- [ ] Render three segmented choices: **Light**, **Dark**, and **Neon**.
- [ ] Selecting a choice applies and stores it immediately. Add a **Done** button that closes the dialog.
- [ ] Do not copy the reference app's Chat or User settings tabs. Glyphflow already has routed account pages, and fake tabs would add no function.
- [ ] The account row shows the display name and username/email with ellipsis when needed.

### 3.3 Mobile behavior

- [ ] Below 768 px, keep the existing drawer pattern.
- [ ] The menu button opens the full expanded navigation regardless of the stored desktop collapse state.
- [ ] Preserve:
  - focus enters the drawer;
  - Tab stays trapped inside it;
  - Escape closes it;
  - route navigation closes it;
  - body scrolling is restored after close;
  - focus returns to the menu button.
- [ ] The drawer scrim and all sidebar text must meet contrast requirements in light, dark, and neon themes.

### 3.4 Tests

- [ ] Update `frontend/src/shell.test.ts` for the revised group organization and collapse control.
- [ ] Add one test proving only permitted routes contribute to each group count.
- [ ] Add one render-level assertion that the theme choices contain Light, Dark, and Neon.
- [ ] Do not add snapshot tests for the entire shell. Test behavior and accessible names.

Exit condition: at desktop width, the shell structure closely matches `overview.png`; at mobile width, all existing accessible drawer behavior remains intact.

---

## Phase 4 — Match page organization through shared primitives

### 4.1 `frontend/src/components.tsx`

- [ ] Keep existing component names and call signatures unless an optional prop is enough.
- [ ] `PageHeader`:
  - preserve title, description, and action;
  - use the reference divider and compact typography;
  - add an optional `meta` slot for small page badges;
  - do not show a fake `Live data` badge by default.
- [ ] `MetricCard`:
  - add optional Lucide `icon` and tone props;
  - keep callers that only pass label/value/detail working;
  - render the icon in the compact bordered square shown in the screenshots.
- [ ] `DataTable`:
  - keep its accessible caption;
  - keep horizontal scrolling on narrow screens;
  - use compact headers and row separators from the reference;
  - do not introduce a table library.
- [ ] `Pagination`:
  - keep Previous/Next behavior and accessible navigation labeling;
  - style it as the bordered footer row in the screenshots;
  - do not add a page-size selector until the API and page state support it.
- [ ] `StatusPill`:
  - keep normalization centralized;
  - render a subtle border/background per status tone;
  - keep readable text in all three themes.
- [ ] Keep `Dialog` keyboard focus management and Escape behavior unchanged.
- [ ] Update focused component and accessibility tests for optional icon/tone/meta rendering.

### 4.2 Overview

Edit `frontend/src/dashboard.tsx` without changing endpoints or permissions.

- [ ] Use the existing query results to organize the page into:
  1. page header;
  2. metric row for active runs, due schedules, and offline runners when permitted;
  3. recent audit activity section when permitted;
  4. quick links.
- [ ] Use server totals when returned. Do not use a page's visible row count as a system-wide total.
- [ ] Preserve independent loading/error behavior. One failed widget must not erase successful widgets.
- [ ] At wide desktop widths, use up to four metric columns. Collapse to two and then one as available width decreases.

### 4.3 Screenshot-mapped pages

- [ ] `UserManagementPage` in `frontend/src/admin-pages.tsx`:
  - match the header, compact filter, table container, status pills, and actions seen in `users.png`;
  - keep current endpoints, permissions, dialogs, and session actions;
  - do not create summary metrics unless they can be computed accurately from returned totals.
- [ ] `RoleManagementPage` in `frontend/src/admin-pages.tsx`:
  - match the filter/action/table organization in `roles.png`;
  - keep seeded roles immutable and existing permission behavior;
  - allow permission pills to wrap without expanding the page horizontally.
- [ ] `frontend/src/audit-page.tsx`:
  - match `system_events.png`: header, filters, bordered table, compact status, and pagination;
  - retain all current filters and safe/redacted detail rendering;
  - long audit content must wrap inside its cell instead of widening the whole viewport.
- [ ] Existing settings/editor pages:
  - use grouped bordered sections like `platform-configuration.png`;
  - retain native controls and existing validation;
  - do not rename backend fields to AI Platform names.
- [ ] `frontend/src/account-pages.tsx`:
  - style Profile, Password, Identities, and Sessions links as a segmented tab row similar to `settings-user.png`;
  - preserve URLs and browser navigation;
  - do not turn routed sections into local-only fake tabs.

### 4.4 Full route pass

- [ ] Open every visible route in light, dark, and neon modes.
- [ ] Fix shared CSS first. Add page-specific CSS only when the page has a genuinely unique structure.
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

- [ ] Move the body of the current worker startup into a shared function in `backend/cmd/worker/run.go` with this responsibility:
  - accept a parent `context.Context`;
  - accept explicit stdout and stderr `io.Writer` values;
  - accept a small status sink used to publish the endpoint and current capacity;
  - return an `error` instead of calling `os.Exit`;
  - block until context cancellation, exactly as the current main function does;
  - close JetStream before waiting for background goroutines;
  - close the local store exactly once;
  - preserve all enrollment, key persistence, recovery, consumer, outbox, heartbeat, and control-message behavior.
- [ ] Keep `needsRunnerEnrollment` in the same package and preserve its existing tests.
- [ ] Wrap returned errors with useful startup stage names, but do not include credentials or private keys.
- [ ] Do not change the order of security checks or durable writes merely to make the GUI easier.

### 5.2 Headless entry point

- [ ] Keep a small headless `main` behind `//go:build !workerui`.
- [ ] It must:
  - create the existing SIGINT/SIGTERM context;
  - call the shared runner with `os.Stdout` and `os.Stderr`;
  - print a final error to stderr;
  - exit non-zero on startup/runtime failure.
- [ ] `go run ./cmd/worker` and normal `go test ./...` must continue to compile without Wails, WebKit, GTK, or a graphical session.

### 5.3 Writer plumbing inside the runtime

- [ ] Replace direct `fmt.Printf` calls in `backend/internal/worker/runtime.go` with an `io.Writer` field on `OrderRuntime` and one small helper method.
- [ ] Default a nil writer safely so existing unit tests and alternate constructors do not panic.
- [ ] Pass the shared runner's stdout writer into `OrderRuntime`.
- [ ] Keep errors on stderr in the command package. Do not classify ordinary lifecycle messages as stderr merely to color them red.
- [ ] Do not print task stdout/stderr locally. Task output must continue through the signed log-chunk event path; the GUI terminal is for worker process logs.

Exit condition: focused and race tests pass, and the headless worker behaves exactly as it did before the GUI work.

---

## Phase 6 — Add a bounded, concurrency-safe GUI log/status model

Create `backend/cmd/worker/log_buffer.go` and `log_buffer_test.go`. Keep this independent of Wails so it can be tested headlessly.

### 6.1 Data contract

- [ ] Define a log entry with JSON fields:
  - monotonically increasing `sequence`;
  - UTC `timestamp` in RFC3339 with fractional seconds;
  - `stream`, exactly `stdout` or `stderr`;
  - `text` containing one displayed line without a trailing newline.
- [ ] Define a snapshot with:
  - redacted `natsEndpoint`;
  - integer `parallelExecutions`;
  - `entries` newer than a requested sequence;
  - `reset` when the caller's sequence is older than the retained buffer.
- [ ] Label the second field **Parallel executions** in the UI. Its value is the worker's current configured capacity, not active process count.

### 6.2 Log writer behavior

- [ ] Provide separate stdout and stderr writers backed by one shared buffer.
- [ ] Make writes safe when many execution goroutines log concurrently. Verify with `go test -race`.
- [ ] Correctly join partial writes until a newline arrives.
- [ ] Preserve blank lines.
- [ ] Normalize CRLF to one displayed line ending.
- [ ] Bound retained history to the newest 5,000 lines. Document this fixed ceiling in one `ponytail:` comment and point to a persistent log store as the upgrade path only if operators later need longer history.
- [ ] Optionally mirror each write to the original stdout/stderr in headless/dev use. A missing Windows console must not cause an error.
- [ ] Never parse or reinterpret ANSI escape sequences. Display log text as text, never `innerHTML`.

### 6.3 Status behavior

- [ ] Set the endpoint after `config.FromEnv(config.Worker)` succeeds.
- [ ] Parse the NATS URL with `net/url` and expose `parsed.Redacted()`. If parsing unexpectedly fails, expose a non-secret placeholder and send the parse error to stderr.
- [ ] Set parallel executions from `currentCapacity.Load()` after initial capacity resolution.
- [ ] Because `worker.ApplyRunnerControl` already updates the shared atomic value, read that same atomic for every GUI snapshot. Do not add a second capacity variable or a new environment setting.

### 6.4 Unit tests

- [ ] Test stdout and stderr classification.
- [ ] Test partial-line joining and CRLF normalization.
- [ ] Test chronological sequence across both streams.
- [ ] Test the 5,000-line bound and `reset` behavior.
- [ ] Test concurrent writers under the race detector.
- [ ] Test URL credential redaction, including a URL with username/password.
- [ ] Test that capacity snapshots observe later atomic updates.

Exit condition: the model is fully testable without a window and cannot grow without bound.

---

## Phase 7 — Add the tray-first Wails desktop shell

### 7.1 Framework decision

- [ ] Use Wails v3 for the desktop-only `workerui` build. This choice is specific to the requirement: its official APIs provide a cross-platform system tray, hidden windows, cancellable close hooks, and window-minimize events.
- [ ] Pin the Go module and CLI to the same tested prerelease. At the time this plan was written, use `v3.0.0-alpha2.118`; do not use an unpinned `latest` in release scripts.
- [ ] Record the pin in `backend/go.mod`/`backend/go.sum` and build documentation.
- [ ] Keep all Wails imports behind the `workerui` build tag so headless builds remain free of native GUI requirements.
- [ ] Relevant official references:
  - <https://v3.wails.io/features/menus/systray/>
  - <https://v3.wails.io/reference/window/>
  - <https://v3.wails.io/reference/events/>
  - <https://v3.wails.io/concepts/lifecycle/>
  - <https://v3.wails.io/guides/build/cross-platform/>

### 7.2 Desktop entry point

- [ ] Add `backend/cmd/worker/main_desktop.go` with `//go:build workerui`.
- [ ] Embed the worker GUI assets and a tray icon from directories beneath `backend/cmd/worker`; Go embed patterns cannot use `..`.
- [ ] Copy `assets/glyphflow.png` into the worker desktop asset directory for the tray/window icon. Do not use the AI Platform logo.
- [ ] Create one Wails application, one normal resizable window, and one system tray icon.
- [ ] Window properties:
  - title: `Glyphflow Worker`;
  - initial width: about 860 px;
  - initial height: about 560 px;
  - minimum width: 640 px;
  - minimum height: 420 px;
  - starts with `Hidden: true`;
  - uses a background color matching the page token to avoid a white flash;
  - standard native frame; do not build a custom title bar.
- [ ] Configure Wails so the application does not quit merely because its last window is hidden or closed on Windows/Linux.

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

- [ ] Tray tooltip/label is `Glyphflow Worker`.
- [ ] Tray menu contains exactly:
  - **Open**;
  - separator;
  - **Exit**.
- [ ] A left click on the tray icon opens the window. It does not exit the application.
- [ ] Register a cancellable `WindowClosing` hook. Unless explicit Exit is already in progress, hide the window and cancel the close event.
- [ ] Listen for `WindowMinimise`; restore and hide the window so it leaves the taskbar.
- [ ] The Open action always calls restore, show, and focus in that order.
- [ ] Keep a `sync.Once` or equivalent one-shot guard around explicit shutdown so double-clicking Exit cannot close resources twice.
- [ ] Start the shared worker in a goroutine. Never run the worker loop on the GUI event thread.
- [ ] If worker startup fails, keep the tray application alive and write the failure to stderr so the user can open the window and read it. Exit remains available.

### 7.4 Asset/API delivery without another frontend toolchain

- [ ] Use a tiny embedded vanilla HTML/CSS/JavaScript UI under `backend/cmd/worker/ui`.
- [ ] Do not create a second React project and do not add npm dependencies for this three-field window.
- [ ] Serve embedded assets with the Wails asset handler.
- [ ] Expose only a same-origin read-only `/api/snapshot?after=<sequence>` handler to the embedded page.
- [ ] Validate `after` as a non-negative integer. Invalid values return HTTP 400.
- [ ] Return JSON with `Content-Type: application/json` and `Cache-Control: no-store`.
- [ ] Do not bind mutation methods to JavaScript.
- [ ] The snapshot handler is internal to the WebView asset server. Do not open a public TCP listener.

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

- [ ] Reuse the AI Platform light-theme values translated for the main frontend: pale lilac page, white/transparent cards, purple active control, muted labels, thin borders, and `0.75rem` radius.
- [ ] Use system fonts. Do not load fonts from the network.
- [ ] Top area contains the Glyphflow icon, `Glyphflow Worker`, and muted text `Runs in the system tray`.
- [ ] Status area contains exactly two read-only cards:
  - **NATS JetStream endpoint** with the redacted URL;
  - **Parallel executions** with the live integer capacity.
- [ ] Terminal header contains the label **Logs** and two accessible filter buttons:
  - **All**: stdout and stderr in sequence order;
  - **Stderr**: only entries whose stream is `stderr`.
- [ ] The active filter uses purple and has `aria-pressed="true"` or correct tab semantics.
- [ ] Terminal behavior:
  - read-only `<pre>` or equivalent text container;
  - monospace font;
  - selectable text;
  - vertical scrolling;
  - wraps very long lines rather than widening the window;
  - stderr lines use an accessible red/pink tone;
  - polling appends text with `textContent`, never HTML;
  - automatically follows new logs only when the user is already near the bottom;
  - if the user scrolls upward, new logs do not pull them away;
  - changing filters recomputes from the retained browser-side entries and preserves sequence order.
- [ ] Poll snapshots every 500–1,000 ms. Stop polling while the document is hidden and refresh immediately when it becomes visible.
- [ ] Keep at most the same 5,000 entries in browser memory. If the server returns `reset`, replace browser history with the returned retained entries.
- [ ] At narrow window widths, stack the two status cards. The terminal must still remain usable at the minimum window size.

Exit condition: the worker starts in the tray, Open restores it, close/minimize return it to the tray, and the UI displays live safe status/log data while work continues.

---

## Phase 8 — Update builds without breaking enrollment

- [ ] Update `backend/build_runner_binaries.sh` to build desktop artifacts with the pinned Wails v3 CLI and the `workerui` tag.
- [ ] Keep exact outputs:
  - `runner-binaries/glyphflow-runner-linux-amd64`;
  - `runner-binaries/glyphflow-runner-windows-amd64.exe`.
- [ ] Do not change `backend/internal/api/infrastructure.go` artifact lookup or download naming.
- [ ] Add clearly named headless artifacts for server use without replacing the desktop outputs:
  - `runner-binaries/glyphflow-runner-linux-amd64-headless`;
  - `runner-binaries/glyphflow-runner-windows-amd64-headless.exe`.
- [ ] Headless artifacts use the default `!workerui` entry point and the current plain Go build flow.
- [ ] Desktop artifacts use Wails production builds. Preserve `-trimpath`/stripped release behavior through Wails' production task.
- [ ] Document required developer/runtime dependencies:
  - Windows 10/11: WebView2 runtime;
  - Linux: supported GTK/WebKit runtime and an active desktop session;
  - headless Linux: use the headless artifact, with no GUI dependency.
- [ ] Update `README.md` only where worker start/stop or build instructions become inaccurate.
- [ ] Update release checks so both the headless and desktop command variants compile.
- [ ] If Linux uses the Wails legacy GTK3 tag for the repository's supported distro, encode that tag in the build command rather than requiring developers to remember it.

Exit condition: enrollment tests still find the same desktop artifact names, and operators still have an explicit headless artifact for VMs/services.

---

## Phase 9 — Verification and visual acceptance

### 9.1 Automated checks

- [ ] Frontend:
  - `cd frontend && npm test`
  - `cd frontend && npm run typecheck`
  - `cd frontend && npm run lint`
  - `cd frontend && npm run build`
- [ ] Worker/core:
  - `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./cmd/worker ./internal/worker ./internal/config ./internal/queue`
  - `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./cmd/worker ./internal/worker ./internal/queue`
  - `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./...`
- [ ] Full backend regression:
  - `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./...`
- [ ] Desktop compile:
  - build Windows AMD64 desktop worker;
  - build Linux AMD64 desktop worker on a supported Wails builder;
  - build both headless artifacts;
  - run `backend/build_runner_binaries.sh` and verify the four expected files exist and are non-empty.
- [ ] Run the existing infrastructure artifact test to prove enrollment names did not drift:
  - `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run Runner`

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

- [ ] Run `git diff --check`.
- [ ] Review `git diff --stat`; remove copied reference code, dependencies, or abstractions that are not required by the final behavior.
- [ ] Ensure no generated `dist`, Wails cache, temporary build directory, credential, database, or log file is committed unless it is an intentional embedded worker UI asset.
- [ ] Ensure all new dependencies are pinned and represented in lock/checksum files.
- [ ] Check the two parent boxes at the top only after every relevant acceptance item passes.

## Definition of done

The work is complete only when a user can recognize the main frontend as the same visual family as the local AI Platform screenshots, use every existing Glyphflow route without regression, start a desktop worker without a visible window, open it from the tray, hide it again by closing or minimizing, inspect the safe endpoint/capacity/log information, and stop it only through the explicit tray Exit action. Headless workers must remain buildable for VM/service deployments.
