# Changelog

All notable changes to Glyphflow are documented here.

## [0.3.1] - 2026-08-27

### Features

- Added file-backed AES-256-GCM storage for SSO and named task secrets, integrity statuses, existing-key deployment, just-in-time runner environment injection, secret visibility controls, and admin task-usage/deletion protection.
- Added control-plane alert evaluation, PostgreSQL-owned storage-capacity signals, and SQL-backed administrator pagination.

### Bugfixes

- Hardened API response caching, OIDC state entropy and cleanup, session error propagation and revocation after password changes, signed worker cancellation handling, impossible cron-date validation, and queue selection.
- Raised Go dependency security floors, expanded security checks across shipped build tags, and corrected the development storage-capacity environment default.

### Other

- Removed the unused production pipeline and dead helpers, normalized Go module metadata, expanded orchestrator lifecycle coverage, coalesced live-log audit polling, measured frontend startup timing, documented retention and control-plane HA boundaries, and removed the offline API docs CDN dependency.
- Added shared API contract fixtures and bumped the application version to 0.3.1.

## [0.3.0] - 2026-08-27

### Features

- Added seven-day cron schedule projection with deterministic runner-pool placement and exclusive-resource conflict detection.
- Added startup and thirty-minute projection refreshes with atomic retention of the last successful snapshot.
- Added an authenticated schedule-projection API endpoint with task-read authorization.
- Added the Scheduling Gantt with week and daily views, runner/task grouping, hourly divisions, filters, conflict-only mode, runner separators, hover details, and numbered conflict markers.
- Added Overview conflict alerts with a dismissible dialog and persistent notice linking to the Gantt.
- Added a login password-visibility toggle, clearer resource name/ID presentation, and consistent schedule controls and tabs.

### Bugfixes

- Aligned projection timestamps, identifiers, placement fields, and resource fields across the backend API and frontend.
- Preserved every projected occurrence and filtered conflict details consistently with Gantt selections.
- Corrected Gantt label sizing, timeline width, daily axis spacing, clipping, and repeated row labels.
- Added PostgreSQL startup retries and persistent NATS reconnect attempts for control plane and workers.
- Standardized page action spacing and Overview conflict-notice placement.

### Breaking Changes

- Renamed task execution timeout fields to duration fields (`timeout_seconds`/`timeoutSeconds` became `duration_seconds`/`durationSeconds`) across schemas, APIs, protocols, scripts, and the UI.

### Other

- Bumped the application version to 0.3.0.

## [0.2.4] - 2026-08-26

### Features

- Added full and partial deployment bundles with pinned Glyphflow, NATS, and PostgreSQL images, offline archives, manifests, checksums, and topology validation.
- Added direct control-plane container mode for private ingress, including ACA-style routing to port 8080 and private PostgreSQL/NATS network contracts.
- Added runner CPU and memory samples with current values, history charts, and configurable retention.
- Added pending-user approval, user status filtering, human-friendly copyable identifiers, and normalized session device labels.
- Added registered, pending, and session counters, single-role user filtering, contextual metric icons, and elevated permission indicators.
- Ordered custom-role permissions alphabetically by column and row, and standardized console actions, navigation, and refresh controls.

### Bugfixes

- Enforced secure worker endpoints and private PostgreSQL CA trust, and preserved immutable run references.
- Excluded GET audit events by default and retained mutation request/response change details.
- Corrected cron preview wildcard, step, calendar, leap-year, timezone, and DST handling.
- Improved log prefixes and frontend control widths, sidebar footer visibility, and dialog/tooltip layering.
- Improved modal titlebars and scrolling, task name display and archive spacing, table/pagination connections, and page layout consistency.
- Hid accepted audit preflight events by default and recorded before/after values for write operations.

### Other

- Pinned release build inputs, added image provenance/SBOM attestations, expanded release smoke tests, and synchronized 0.2.4 documentation and metadata.

### Breaking Changes

- Version 0.2.4 is clean-install-only and does not upgrade earlier PostgreSQL schemas.
- Outside development, worker control-plane endpoints must use HTTPS and NATS endpoints must use TLS; insecure transport requires explicit development settings.

## [0.2.3] - 2026-08-25

### Features

- Added protected installation-key validation, retention/legal holds, storage-pressure run rejection, and durable dead-letter retry state.
- Added isolated production Compose topology with non-root services, read-only boundaries, private data volumes, and explicit resource limits.

### Bugfixes

- Bounded collection, audit, worker-order, and run-log reads; rejected unsafe pagination offsets and preserved resumable log ordering.
- Gated durable mutation auditing, hardened OIDC endpoint handling, and preserved worker event/recovery delivery across broker interruptions.

### Other

- Hardened production transport/security headers, separated runner event subjects, synchronized release documentation/toolchains, and aligned frontend build metadata.
- Added a provisional, non-guaranteed development profile for local test inputs.

### Breaking Changes

- Production Compose now requires protected file-based secrets and a unique client project name; shared/default deployment inputs are unsupported.
- Runner heartbeats now use dedicated `glyphflow.heartbeats.<runner>` subjects; legacy event-subject routing is rejected.

## [0.2.2] - 2026-08-25

### Features

- Added bounded, low-cardinality operational metrics, threshold alerts, and a protected System Metrics administration page.
- Added encrypted, deduplicated dead-letter records with paginated inspection, audited retry, terminal reconciliation, and least-privilege controls.
- Added bounded SQL run filtering with placement blockers and indexed worker event-outbox scans with safe published-event compaction.
- Added a singleton production operations runbook covering restart, backup/restore, upgrade, and runner re-enrollment.

### Bugfixes

- Aligned OIDC administration with the canonical API contract, rejected unknown provider fields, and preserved disabled providers in administrator listings.
- Updated shared profile state immediately after account changes and made unsupported task secret references fail closed.
- Restricted and rate-limited runner enrollment, validated production endpoints, and bound development database/NATS ports to loopback.
- Preserved dead-letter identity and diagnostics, redacted recovery data, and NACKed messages when dead-letter persistence or publication fails.
- Preserved worker event sequence high-water marks during compaction and embedded the release version in built binaries.

### Other

- Added release-time Go/npm dependency gates and current Go/npm SBOM generation.
- Added nginx compression and immutable caching for hashed frontend assets, and reduced the shipped logo size.
- Bounded audit filter discovery, aligned live API documentation/statuses, and removed the known frontend SSR warning without hiding other test errors.
- Added PostgreSQL/NATS operational recovery-gate coverage and documented broker isolation and singleton deployment boundaries.

### Breaking Changes

- Deprecated legacy task, event, and run action routes now return HTTP 410; clients must use the canonical endpoints.
- Non-empty task secret references are rejected until a configured secret resolver is available.

## [0.2.1] - 2026-08-22

### Features

- Added selectable GUI, TUI, and headless worker artifacts with tray controls, capacity reporting, and lifecycle recovery.
- Added signed NATS start claims, atomic run starts, component readiness, durable repository validation, and shared scheduling/timezone improvements.
- Expanded the administration console with task, schedule, runner, resource, session, audit, and execution-management workflows.

### Bugfixes

- Hardened credentialed CORS, pagination arithmetic, worker boot metadata handling, and control-plane startup cleanup.
- Prevented blocked schedules from starving due runs and kept worker event ordering recoverable.
- Restricted production transport and enrollment defaults, including explicit local origins and private PostgreSQL/NATS ports.

### Other

- Standardized the frontend runtime and Vite toolchain, documented the current architecture and deployment boundaries, and added CI/security status checks.

## [0.2.0] - 2026-08-18

### Features

- Completed the live run lifecycle with dispatched/start-failure handling, cancellation, timeouts, task resources, signed worker start claims, and recovery semantics.
- Added the reference-aligned administration console, dialogs, themes, navigation, task actions, manual execution, version diffs, and bounded live logs.
- Added selectable worker interfaces, cross-platform artifacts, lifecycle controls, capacity visibility, persistent SQLite recovery, and application-version display.

### Bugfixes

- Improved worker execution diagnostics and live log delivery, and prevented blocked schedules from starving due runs.

### Other

- Replaced the Wails worker shell with Gio-based GUI/TUI support and refreshed frontend/build dependencies and documentation.

## [0.1.0] - 2026-08-17

### Features

- Introduced the first alpha platform: a Go control plane with PostgreSQL, NATS JetStream, scheduling, retries, cancellation, recovery, audit events, and streamed logs.
- Added signed worker communication, enrollment, mutual TLS, RBAC, password/OIDC authentication, sessions, CSRF protection, and external-secret support.
- Added the React console for tasks, schedules, runs, logs, runners, pools, resources, audit, users, roles, SSO, accounts, filters, themes, and responsive layouts.
- Added persistent concurrent workers with SQLite recovery, capacity control, cross-platform builds, a tray UI, Docker Compose, SBOM generation, and security checks.
