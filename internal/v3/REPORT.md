# Glyphflow Project Audit

Audit date: 2026-08-18  
Baseline: `d269f1c` (`version/v0.2.0`)  
Scope: backend, worker build modes, frontend, deployment files, scripts, tests, and security controls.

## Result

Glyphflow has a working control-plane, worker, and React console path. The baseline test suites pass. No Critical finding was confirmed. The main risks are contract drift, incomplete production wiring for OIDC client secrets, release-script version handling, and deployment defaults that are safe only for local development.

## Available features

| Area | Current capability | Evidence |
|---|---|---|
| Control plane | PostgreSQL-backed API, scheduler, dispatcher, NATS JetStream, worker heartbeat monitoring, migrations, readiness checks, and graceful shutdown run from `backend/cmd/controlplane`. | `backend/cmd/controlplane/main.go` |
| Worker | Durable SQLite state, signed order/event recovery, NATS publishing and consuming, heartbeats, resource limits, streamed output, and headless execution. | `backend/cmd/worker/run.go`, `backend/internal/worker` |
| Worker UI modes | Headless default, Gio desktop UI, and Bubble Tea TUI builds. The Windows TUI build can minimize and restore the console from the system tray; other platforms currently use the terminal without a tray integration. | `backend/cmd/worker/main.go`, `main_desktop.go`, `main_tui.go`, `tui_tray_windows.go`, `tui_tray_other.go` |
| Runner management | Enrollment, pool and architecture selection, capacity, binary configuration, worker UI selection, lifecycle actions, endpoint configuration, and runner detail views. | `backend/internal/api/infrastructure.go`, `frontend/src/runner-pages.tsx` |
| Task execution | Immutable task versions, schedules, retries, cancellation, reconciliation, leases, output limits, placement selectors, and audit events. | `backend/internal/api/operations.go`, `backend/internal/store` |
| Authentication | Password and OIDC login, PostgreSQL-backed sessions, refresh-token rotation, CSRF protection, RBAC, SSO administration, account management, and active-session management. | `backend/internal/api`, `backend/internal/platform`, `backend/internal/store` |
| Frontend | React/Vite console with dashboard, tasks, schedules, runs, runners, resources, audit, global variables, administration, account pages, responsive layout, themes, filters, pagination, and permission-aware routes. | `frontend/src/routes.tsx`, `frontend/src` |
| Delivery and security | Signed orders and events, outbound-only workers, queue permissions, TLS configuration, key persistence, request limits, server timeouts, correlation IDs, and command argument arrays. | `backend/internal/protocol`, `backend/internal/config`, `backend/internal/api`, `README.md` |

## Potential new features

### F1. Complete one canonical API contract — High

**Impact:** Some legacy routes intentionally return `501 Not Implemented`, while newer frontend routes and administrative services are implemented elsewhere. Consumers can see an incomplete or misleading API surface.

**Description:** `backend/internal/api/api.go`, `routes.go`, and `docs.go` maintain separate route lists. The OpenAPI document and route registry still contain legacy placeholders such as `/api/v1/roles`, `/api/v1/sso`, `/api/v1/logs`, `/api/v1/events`, `/api/v1/runs/retry`, and `/api/v1/runs/cancel`.

**Suggested modification:** Select the supported route contract. Either remove and deprecate unused placeholders or implement them as adapters to the current services. Add contract tests that compare documented methods and responses with the mounted handlers.

### F2. Add a real release acceptance test — High

**Impact:** Unit and frontend tests do not prove that the built frontend, control plane, PostgreSQL, NATS, enrollment flow, and worker execute one task together.

**Description:** Existing integration tests are mostly configuration-driven and can skip external infrastructure. `scripts/release-check.sh` builds binaries and runs checks but does not exercise a complete deployed workflow.

**Suggested modification:** Add a CI or Compose smoke test that starts PostgreSQL, NATS, control plane, frontend proxy, and one worker; logs in; enrolls the worker; runs a task; verifies events and output; and checks restart recovery.

### F3. Expose operational metrics — Medium

**Impact:** Operators cannot collect the internal runtime counters through a standard monitoring endpoint.

**Description:** The README describes internal runtime metrics, and `platform.Metrics` records counters, but the HTTP surface exposes health/readiness checks rather than a documented metrics endpoint.

**Suggested modification:** Expose a protected or network-restricted `/metrics` endpoint, or document the chosen OpenTelemetry/Prometheus integration. Keep sensitive labels out of metric dimensions.

### F4. Add non-Windows TUI tray support — Low

**Impact:** The TUI tray behavior is platform-specific; Linux and macOS TUI builds currently remain terminal-only.

**Description:** `tui_tray_windows.go` implements tray behavior, while `tui_tray_other.go` is a no-op.

**Suggested modification:** Add platform-specific integrations only if the supported deployment targets require them. Otherwise document Windows-only tray support as the intentional boundary.

## Potential bugs and edge cases

### B1. Production OIDC client-secret resolver is not wired — High

**Impact:** OIDC providers configured with a `secretReference` can be saved, but confidential-client login fails because the production control-plane startup does not configure `OIDCService.SetSecretResolver`.

**Description:** `OIDCService` explicitly returns `OIDC client secret resolver is not configured` when a provider has a secret reference. `backend/cmd/controlplane/main.go` sets the provider and state repositories but never sets the resolver.

**Suggested modification:** Wire a deployment-backed secret resolver during startup, or reject secret references with a clear configuration error until a resolver is available. Add an integration test for a confidential OIDC provider.

### B2. Saving a display name leaves the sidebar stale — Medium

**Impact:** The account page shows the new display name, but the sidebar keeps the old value until the application reloads or restores the auth context.

**Description:** `frontend/src/account-pages.tsx` refetches its profile query after `PUT /api/v1/me`, but does not call the `setProfile` function from `frontend/src/auth.tsx`. The sidebar reads the separate auth-context profile.

**Suggested modification:** Apply the returned profile to the auth context after a successful save, then refetch only if needed for related account data. Add a test that asserts the shared profile state changes.

### B3. Release checks can build binaries with the `dev` version — Medium

**Impact:** A release-check binary built from an arbitrary working directory can report `dev` instead of the root `VERSION` value.

**Description:** `scripts/release-check.sh` invokes `go build` without the version linker flags used by `build/Dockerfile`, `backend/Taskfile.yml`, and `backend/build_runner_binaries.sh`. The runtime fallback in `backend/version.go` reads a relative file path, which is not reliable after packaging.

**Suggested modification:** Read the root `VERSION` in `scripts/release-check.sh` and pass `-X github.com/VBenevides/Glyphflow/backend.Version=...` to both builds. Verify the resulting binaries from a different working directory.

### B4. API documentation and runtime behavior can drift — Medium

**Impact:** Generated clients and operators may rely on response codes or paths that do not match the handlers actually mounted by `api.go`.

**Description:** The route registry, OpenAPI JSON, and mux registration are maintained independently. Existing tests check selected registration and JSON validity, but not every documented method against the live handler.

**Suggested modification:** Generate the OpenAPI route data from the canonical route definitions or add a table-driven live contract test for every documented operation.

## Security risks and patches

### S1. Wildcard CORS and insecure defaults are easy to carry into deployment — Medium

**Impact:** `compose.yaml` and `dev_run.sh` default to wildcard CORS, local HTTP, development mode, and known bootstrap credentials. The current CORS middleware reflects arbitrary request origins while allowing credentials when `CORS_ORIGIN=*`.

**Description:** These values are useful for local development, but copying them into a reachable deployment weakens origin and transport boundaries. Configuration validation helps when production mode is selected, but an operator can override that protection with development or insecure-transport settings.

**Suggested modification:** Make deployment configuration fail closed: require explicit origins, HTTPS, NATS TLS, and bootstrap credentials outside development. Reject wildcard CORS when credentials are enabled, and use a separate clearly named development profile.

### S2. Docker-served frontend does not apply the repository security headers — Medium

**Impact:** The static frontend served by the Nginx image may omit the CSP, referrer, frame, MIME, and permissions headers defined in `frontend/_headers`.

**Description:** `frontend/_headers` contains the headers, but `build/nginx.conf` has no equivalent `add_header` directives and the Docker image does not use a hosting platform that consumes `_headers`.

**Suggested modification:** Add equivalent Nginx headers, with a separate policy for proxied API/docs paths where necessary, and verify them in the release smoke test.

### S3. OIDC URL validation can be vulnerable to DNS rebinding — Medium

**Impact:** An OIDC issuer or endpoint can resolve to a public address during validation and to a private or loopback address when the HTTP request is made.

**Description:** `secureURL` performs DNS/IP checks, but the default HTTP transport resolves the host again. The validation result is not pinned to the connection that performs the request.

**Suggested modification:** Use an allowlist where possible. Otherwise, use a custom dialer that validates the resolved address at connection time and apply the same policy after redirects.

### S4. Security verification script is not self-contained — Low

**Impact:** `scripts/security-check.sh` exits with status 127 when `govulncheck` is absent, before it reaches the remaining checks. This can be mistaken for a clean security result or ignored in ad hoc release work.

**Description:** The script assumes external tools are installed and also references older review/SBOM locations. The current baseline run stopped at `govulncheck: command not found`.

**Suggested modification:** Check tool availability up front with actionable errors, pin the tool versions or run the check in a controlled container, and generate the current SBOM as part of the release path.

## Baseline verification

| Check | Result |
|---|---|
| `GOCACHE=/tmp/glyphflow-audit-go-cache go test ./...` from `backend` | Pass |
| `GOCACHE=/tmp/glyphflow-audit-vet-cache go vet ./...` from `backend` | Pass |
| `GOCACHE=/tmp/glyphflow-audit-tui-cache go test -tags workerui_tui ./cmd/worker` | Pass |
| `go mod verify` from `backend` | Pass |
| `npm test` from `frontend` | Pass — 35 files, 94 tests |
| `npm run build` from `frontend` | Pass |
| `bash -n dev_run.sh backend/build_runner_binaries.sh scripts/security-check.sh scripts/release-check.sh` | Pass |
| `./scripts/security-check.sh` | Blocked — `govulncheck` is not installed; later checks were not reached |

The detailed findings are tracked in [`TODO.md`](TODO.md). This report is a baseline audit, not a claim that the listed patches have already been implemented.
