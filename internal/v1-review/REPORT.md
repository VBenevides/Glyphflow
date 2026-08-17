# Glyphflow v1 implementation and compatibility audit

Review date: 2026-08-14

## Result

The v1 plans define a coherent scheduler, worker, authentication model, API, and web console.

The repository contains much of the planned frontend surface. It also contains backend migrations and tested domain helpers.

The production frontend and backend are not compatible. The browser cannot complete startup, login, session restoration, or a management workflow.

The production control-plane command does not use PostgreSQL or NATS. The worker command validates configuration and then exits.

The checked v1 roadmaps therefore overstate completion. Passing unit tests do not prove the planned v1 outcome.

## Scope

This audit compared these sources:

- `internal/v1/REPORT.md` and `internal/v1/TODO.md`.
- `internal/v1/REPORT-FRONTEND.md` and `internal/v1/TODO-FRONTEND.md`.
- `internal/v1/v1-er-diagram-3.md`.
- The execution, reliability, and authentication deployment documents.
- The production frontend and backend entry points.
- Frontend API calls and backend route registrations.
- Authentication transport, JSON shapes, persistence wiring, and tests.

Revision 3 is the current design. Earlier ERD revisions are historical inputs.

TokenSave tools and `.tokensave/tokensave.db` were unavailable. This audit used direct source tracing.

## Planned v1 outcome

| Area | Planned behavior | Source |
|---|---|---|
| Control plane | PostgreSQL-backed scheduler, dispatcher, API, event consumer, retention, and metrics | `internal/v1/TODO.md` |
| Worker | NATS consumer, durable SQLite inbox and outbox, executor, recovery, and event producer | `internal/v1/v1-er-diagram-3.md` |
| Delivery | Signed messages, at-least-once delivery, idempotent processing, and explicit ambiguity | `internal/v1/EXECUTION-PROTOCOL.md` |
| Authentication | Password and OIDC login through one revocable cookie session | `internal/v1/REPORT-FRONTEND.md` |
| Authorization | Database roles and permissions checked on every backend request | `internal/v1/REPORT.md` |
| Frontend | Startup restore, permission-aware shell, scheduler workflows, administration, and account pages | `internal/v1/TODO-FRONTEND.md` |
| Compatibility | Stable request, response, error, pagination, and log-stream contracts | `internal/v1/REPORT-FRONTEND.md` |

## Available features

### Frontend

| Impact / Severity Level | Capability | Evidence | Current limit |
|---|---|---|---|
| High | Responsive shell and permission-aware routes | `frontend/src/routes.tsx`, `shell.tsx`, and `permissions.ts` | These routes depend on an incompatible backend session and API. |
| High | Typed request client and query states | `frontend/src/api.ts` and `query.tsx` | The client assumes secure cookies that the backend does not issue. |
| High | Task, schedule, run, runner, resource, audit, and administration pages | Files under `frontend/src/*-pages.tsx` | Many controls call missing routes or show placeholder sections. |
| High | Password and OIDC entry pages | `frontend/src/auth-pages.tsx` | Startup fails before these flows can work against the backend. |
| Medium | Theme, CSP template, safe output, and dangerous-action controls | `frontend/_headers`, `safe.tsx`, and `actions.tsx` | Deployment origins remain placeholders. |
| Medium | Unit and mocked workflow tests | `frontend/src/*.test.ts*` | Tests replace `fetch` and do not validate the Go API. |

### Backend libraries

| Impact / Severity Level | Capability | Evidence | Current limit |
|---|---|---|---|
| High | Revision 3 PostgreSQL schema | `backend/migrations/017_*` and `018_*` | The production command never connects or applies migrations. |
| High | Signed protocol primitives | `backend/internal/protocol` | The production commands do not run the protocol flow. |
| High | Durable worker SQLite tables and executor helpers | `backend/internal/worker` | The worker command never opens the store or consumes an order. |
| High | NATS JetStream adapter | `backend/internal/queue/nats.go` | No production caller connects to NATS. |
| High | Scheduling, placement, retry, lease, and recovery helpers | `backend/internal/controlplane`, `platform`, and `store` | Most production callers are absent. |
| Medium | In-memory password, session, role, and OIDC helpers | `backend/internal/api` | State is not persistent and several services are not wired. |
| Medium | Health checks and OpenAPI UI | `backend/internal/api/api.go` and `docs.go` | Readiness does not check PostgreSQL, NATS, or workers. |

## Frontend and backend compatibility

Overall status: **Incompatible**.

| Impact / Severity Level | Contract | Evidence | Description | Suggested modification |
|---|---|---|---|---|
| Critical | Startup configuration | `bootstrapSession` calls `/api/v1/config`. The backend has no matching route. | Every frontend startup ends at the fatal error page. | Add the public runtime configuration endpoint with the frontend JSON shape. |
| Critical | Session transport | `ApiClient` sends cookies. `SessionManager.Authenticator` accepts only `Authorization: Bearer`. | Login returns tokens in JSON, but the frontend ignores them. Session restoration stays unauthorized. | Implement the planned `HttpOnly` cookie session contract on both sides. |
| Critical | CSRF | The frontend reads `glyphflow_csrf`. `ValidateCSRFRequest` requires it. No backend route sets it. | Every unsafe browser request fails with `403`, including login and registration. | Issue the CSRF cookie and validate its header after the session transport is fixed. |
| Critical | Browser origin | `dev_run.sh` uses frontend `127.0.0.1:5173`, `WEB_ORIGIN=http://localhost:5173`, and API `localhost:8080`. | Origins differ, and the backend has no CORS or preflight support. Browser requests are blocked. | Use one same-origin development proxy, or add an exact tested CORS policy. |
| Critical | Current profile | `AuthService.Profile` returns permissions as `map[string]bool`. The frontend expects `string[]`. | Permission checks receive a non-iterable object and cannot build the shell safely. | Publish one JSON schema and return permission keys as an array. |
| High | Authentication JSON | `AuthTokens`, `SessionInfo`, and `OIDCProvider` lack matching JSON tags. | Field names and provider data do not match frontend models. | Add explicit JSON DTOs and contract tests for every authentication response. |
| High | OIDC availability | `cmd/controlplane/main.go` leaves `Server.OIDC` unset. | Provider discovery and OIDC login routes are not registered in production. | Build the OIDC service from persistent provider configuration before registering routes. |
| High | Task and schedule API | Only task list returns data, and it always returns an empty page. Other handlers return `501`. | Create, detail, version, preview, and edit calls cannot work. | Implement the planned storage-backed task and schedule routes. |
| High | Run and log API | Collection and action handlers return `501`. Detail and stream routes are absent. | Run inventory, detail, actions, and live logs cannot work. | Implement run detail, actions, events, stream resume, and download routes. |
| High | Runner and resource API | Collection routes return `501`. Detail, enrollment, action, and delete routes are absent. | Infrastructure pages cannot load or mutate data. | Add the planned runner, enrollment, resource, and lease handlers. |
| High | Administration API | `Server.AuthAdmin` and `Server.Roles` are unset. User, role, SSO, and audit collections are stubs. | Administration pages receive `404` or `501`. | Wire one persistent authorization service and implement the matching routes. |
| High | Account API | The backend supports only `GET /me` and current-session revocation. | Profile, password, identity, and other owned-session actions fail. | Add the planned account routes and owned-session checks. |
| High | Route naming | The backend registers run actions under `/api/v1/tasks/`. The frontend uses `/api/v1/runs/{id}`. | Detail requests can receive the wrong permission check before `501`. | Move run actions to the run resource paths used by the frontend contract. |

## Potential bugs and edge cases

### Production components are not wired

**Impact / Severity Level:** Critical

**Description:** The control plane constructs only in-memory authentication and an HTTP server.

`DATABASE_URL` and `NATS_URL` are validated but never used. Readiness always succeeds when `Ready` is nil.

The worker validates configuration, prints one line, and exits.

**Evidence:** `backend/cmd/controlplane/main.go` and `backend/cmd/worker/main.go`.

**Suggested Modification:** Connect PostgreSQL and NATS, apply migrations, construct components, and pass real readiness checks. Run the worker loop until cancellation.

### The route registry does not describe the actual router

**Impact / Severity Level:** High

**Description:** Registry validation only checks a manually maintained slice. It does not compare the `http.ServeMux` registrations.

The registry omits the OIDC callback, documentation login, and dynamic role route. It also lists services that production does not register.

**Evidence:** `backend/internal/api/routes.go`, `api.go`, `oidc_auth.go`, `roles.go`, and `routes_test.go`.

**Suggested Modification:** Register each route from one route definition that contains its method, access rule, and handler.

### Logout does not match the frontend flow

**Impact / Severity Level:** High

**Description:** The frontend sends an empty logout request. The backend revokes only a session ID from the JSON body.

The UI clears local identity even when no backend session was revoked.

**Evidence:** `frontend/src/shell.tsx` and `backend/internal/api/password_auth.go`.

**Suggested Modification:** Revoke the authenticated cookie session and clear all related cookies in the logout handler.

### Worker recovery does not publish the required outcome

**Impact / Severity Level:** High

**Description:** `RecoverOrders` selects IDs and updates state in separate statements. It does not insert an event-outbox record.

The execution protocol requires one atomic state change and durable recovery event.

**Evidence:** `backend/internal/worker/store.go`, `recovery.go`, and `internal/v1/EXECUTION-PROTOCOL.md`.

**Suggested Modification:** Use one SQLite transaction to mark each order unknown and insert its recovery event.

### Edit pages do not load existing versions

**Impact / Severity Level:** Medium

**Description:** Task and schedule edit routes initialize empty drafts. They never load the selected current version.

**Evidence:** `frontend/src/task-editor.tsx` and `schedule-pages.tsx`.

**Suggested Modification:** Load the active version before editing and keep the original draft as the dirty-state baseline.

### Account pages start dirty for existing display names

**Impact / Severity Level:** Medium

**Description:** `useUnsavedChanges` treats any non-empty display name as a change.

An unchanged profile can trigger a leave warning after data loads.

**Evidence:** `frontend/src/account-pages.tsx`.

**Suggested Modification:** Compare the edited display name with the loaded profile value.

### OpenAPI and runtime routes can drift

**Impact / Severity Level:** Medium

**Description:** The OpenAPI document, route registry, and mux registrations are three separate route lists.

The documentation test checks JSON syntax but not runtime agreement.

**Evidence:** `backend/internal/api/docs.go`, `routes.go`, and `docs_test.go`.

**Suggested Modification:** Add one synchronization test, or generate the document from the route definitions.

## Security risks and patches

### OIDC callback does not verify a provider token

**Impact / Severity Level:** High

**Description:** The callback builds identity claims from request query values. It performs no authorization-code exchange or ID-token signature check.

Production does not wire this service today. It must remain disabled until the flow is complete.

**Evidence:** `backend/internal/api/oidc_auth.go` and `backend/internal/platform/oidc.go`.

**Suggested Modification:** Exchange the provider code server-side. Verify the signed ID token through issuer metadata and JWKS.

### Authentication abuse controls are test-only

**Impact / Severity Level:** High

**Description:** A bounded rate limiter exists, but no login, registration, OIDC, or callback handler calls it.

**Evidence:** `backend/internal/platform/rate_limit.go` and `rate_limit_test.go`.

**Suggested Modification:** Apply limits by normalized username and source address at each authentication entry route.

### Password policy is not enforced at the trust boundary

**Impact / Severity Level:** High

**Description:** Backend registration accepts any non-empty password. The production command also passes a nil Argon2id pepper.

Frontend length checks can be bypassed.

**Evidence:** `backend/internal/api/auth_service.go`, `platform/password.go`, and `cmd/controlplane/main.go`.

**Suggested Modification:** Enforce the approved password policy in `AuthService.Register`. Load a deployment pepper from a secret source.

### Administrator and login-method guards are not used by the API

**Impact / Severity Level:** High

**Description:** Guard helpers exist, but production mutation handlers do not call them in database transactions.

Future wiring could disable the final administrator or login method.

**Evidence:** `backend/internal/platform/admin.go`, `auth_policy.go`, and `backend/internal/api/auth_admin.go`.

**Suggested Modification:** Apply both guards inside the persistent mutation transaction before exposing administration routes.

### Authentication state is not persistent

**Impact / Severity Level:** Medium

**Description:** Users, password hashes, roles, access sessions, and refresh sessions live in process memory.

Restarting the API removes all sessions and non-bootstrap users. Database security constraints do not protect this state.

**Evidence:** `backend/internal/api/auth_service.go` and `session.go`.

**Suggested Modification:** Replace in-memory maps with the existing PostgreSQL identity schema and transaction-safe stores.

## Potential new features

These are planned v1 capabilities, not optional expansion.

| Impact / Severity Level | Description | Evidence | Suggested modification |
|---|---|---|---|
| Critical | Production control-plane runtime | Domain helpers exist, but `cmd/controlplane` starts only HTTP. | Wire migrations, scheduler, dispatch, consumers, producers, retention, metrics, and graceful shutdown. |
| Critical | Production worker runtime | Worker store, verifier, executor, and NATS adapter exist separately. | Wire enrollment, SQLite recovery, order consumption, execution, event publication, and heartbeats. |
| High | Storage-backed management API | Frontend pages already define the required user workflows. | Implement the v1 task, schedule, run, runner, resource, audit, and administration contracts. |
| High | Resumable execution log stream | Frontend resume and deduplication helpers already exist. | Add a same-origin stream and bounded download endpoint backed by accepted log chunks. |
| High | Stable browser session API | Frontend already uses credentialed requests and refresh serialization. | Add runtime config, secure cookies, CSRF issuance, refresh rotation, and authenticated logout. |

Optional themes, localization, notifications, approvals, and telemetry should remain deferred until the v1 acceptance path works.

## Verification results

| Command | Result | Meaning |
|---|---|---|
| `GOCACHE=/tmp/glyphflow-audit-go-cache go test ./...` | PASS | Backend unit and in-memory tests pass. |
| `npm test` | PASS, 27 files and 40 tests | Frontend unit and mocked-fetch tests pass. |
| `npm run build` | PASS | TypeScript and the production bundle compile. |
| `go test -tags=integration -v ./internal/integration` | PASS with PostgreSQL/NATS test skipped | Required service variables and mutual-TLS files were absent. |

No test starts the production frontend and backend together.

No test proves startup configuration, cookie login, `/me`, logout, or one scheduler workflow across the real boundary.

The existing failure-injection test uses an in-memory counter. It does not kill real schedulers, consumers, producers, or workers.

## Recommended delivery order

1. Approve one cookie-session and JSON contract.
2. Make frontend startup, login, `/me`, refresh, and logout pass together.
3. Wire PostgreSQL authentication, roles, migrations, and readiness.
4. Implement one task, schedule, run, and log vertical slice.
5. Wire the control-plane and worker production loops.
6. Add real PostgreSQL, NATS, process, restart, and browser acceptance tests.
7. Reconcile the checked v1 roadmaps with verified end-to-end results.

Do not add optional product features before this acceptance path passes.
