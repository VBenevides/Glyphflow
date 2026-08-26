# Changelog

All notable changes to Glyphflow are documented here.

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
