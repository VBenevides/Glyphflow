# Glyphflow code audit

Audit date: 2026-08-14

## Result

Glyphflow has a useful alpha control plane and React console. The main vertical slice is not release-ready.

The unit, race, vet, frontend, lint, and build checks pass. The tagged integration suite does not compile.

The highest risks are runner trust, cancellation semantics, worker recovery, stored signing secrets, and unbounded HTTP input.

## Scope

The audit covered these paths:

- All tracked Go, TypeScript, CSS, SQL, shell, configuration, and root documentation files.
- The control-plane, worker, PostgreSQL, NATS, and browser flows.
- `internal/v0`, `v0-review`, `v1`, `v1-review`, and `v1-test` records.
- Current branch history through `26a2c98`.

TokenSave tools and `.tokensave/tokensave.db` were not available. Direct source tracing was used.

## Previous steps

### v0

v0 added the project layout, protocol primitives, database helpers, worker execution helpers, and early API code.

The v0 review found that roadmap results exceeded production behavior. The code had useful primitives but no complete durable task path.

The current `internal/v0/TODO.md` still uses unchecked boxes beside recorded commits and results. Treat its boxes as historical data.

### v1 design and implementation

v1 revision 3 defined the correct target model:

- Immutable task and schedule versions.
- Dynamic runner placement and active sessions.
- Schedule, retry, ambiguity, cancellation, and resource policies.
- Ordered state events and separate log sequences.
- Durable SQLite recovery and signed execution messages.
- Persistent users, roles, sessions, OIDC, audit, and canonical PostgreSQL tables.

`internal/v1/TODO.md` records 63 completed rows. `TODO-FRONTEND.md` records 51 completed rows and two deferred rows.

The v1 review then found an incompatible browser and API, unwired runtimes, in-memory state, and unsafe authentication gaps.

Later commits fixed much of that review. They added canonical persistence, cookie sessions, OIDC token verification, the scheduler, dispatcher, worker, logs, and the UI.

### v1 human test and persistence pass

The nine human frontend findings in `internal/v1-test/TEST-1.md` are recorded as fixed.

The persistence roadmap records 22 completed rows and nine open rows. Several open rows remain current:

- Database error and transaction conventions.
- Retention and maintenance queries.
- Filter and pagination compatibility.
- Restart workflow coverage.
- Full backend, frontend, and database release gates.

Some older checked rows describe helpers, not active production behavior. Metrics, retention, signals, and full schedule policies are examples.

## Available features

### Backend

- One Go control-plane process with HTTP, scheduling, dispatch, event ingestion, and heartbeat loops.
- One Go worker with embedded enrollment data, SQLite inbox/outbox storage, direct process execution, and streamed logs.
- Canonical PostgreSQL tables for identity, SSO, audit, tasks, schedules, runs, runners, resources, inbox, and outbox data.
- Ed25519 order and event envelopes with fixed domains and protocol validation.
- Argon2id password hashing with peppering, session rotation, CSRF checks, RBAC, and system-admin guards.
- OIDC discovery, authorization-code exchange, PKCE state storage, JWKS verification, and group-role mappings.
- Task, schedule, run, runner, pool, resource, audit, user, role, SSO, and account APIs.
- Append-only audit events, run events, and log chunks.

### Frontend

- React 18, React Router, TanStack Query, TypeScript, Vite, Vitest, and Lucide icons.
- Session restore, login, registration, OIDC callback, and account pages.
- Responsive navigation, light and dark themes, permission-aware routes, and accessible dialogs.
- Task, schedule, run, log, runner, pool, resource, audit, user, role, SSO, and settings pages.
- Loading, empty, forbidden, not-found, retry, validation, and correlation-ID states.

## Frontend and backend integration

| Flow | Status | Evidence |
|---|---|---|
| Password login and session restore | Partial | Cookies work, but login and refresh also return tokens in JSON. |
| Task creation | Partial | The API stores task environment JSON, but dispatch does not send it to the worker. |
| Task editing | Incomplete | The API omits several stored fields, and the editor resets them. |
| Schedule creation | Partial | Basic triggers work. Stored policy fields are not enforced by the scheduler. |
| Manual and scheduled execution | Partial | Basic dispatch exists, but task resources and stored execution policies are not applied. |
| Run logs | Partial | PostgreSQL logs reach the UI. Cross-origin API configuration is bypassed. |
| Cancellation | Unsafe | The API marks a run cancelled but sends no cancel order. |
| Retry and reconciliation | Broken | Runs enter `RETRY_WAIT`, but dispatch claims only `WAITING`. |
| OIDC login and linking | Incomplete | Callback paths and nonce handling drift. Linking has no backend route. |
| Resources and leases | Isolated | CRUD and lease methods exist. Dispatch does not acquire task resources. |
| Filters and dashboard | Inaccurate | Several frontend query parameters are ignored by backend collections. |

## Bugs and incomplete features

| Severity | Description and evidence | Suggested modification |
|---|---|---|
| Critical | Cancelling a running run only updates PostgreSQL. The worker continues and can create external effects. Evidence: `RunService.action`, `RunStore.Transition`, and no cancel handling in `OrderRuntime.Handle`. | Record a cancellation request, publish a signed cancel order, terminate the matching process, and let completion win races. |
| High | Retry and reconciliation never dispatch. Both actions set `RETRY_WAIT`, while `ClaimWaiting` selects only `WAITING`. | Apply retry policy and backoff, then make the run claimable for a new attempt in one transaction. |
| High | Worker recovery emits unsigned JSON through the signed event subject. The control plane rejects it. Evidence: `LocalStore.RecoverOrders` and `applyRunnerEvent`. | Persist enough order data to create a signed `UNKNOWN` event after the signing key is loaded. |
| High | Worker signing keys are regenerated on each boot with the same key ID. A heartbeat overwrites the old public key. Pending old events then fail verification. | Persist the worker private key with `0600` permissions. Use versioned key IDs and retain old public keys until pending events expire. |
| High | State-event order is not enforced when applying events. `last_applied_state_sequence` is unused, and attempt updates have no legal-state guard. | Lock the attempt. Accept only the next legal state sequence. Treat duplicates as success. |
| High | Stored task behavior is not executed. Dispatch ignores environment, secret references, selectors, retry fields, ambiguity policy, and resources. | Extend the dispatch transaction and signed order. Resolve secrets at dispatch and record only resolved versions. |
| High | Schedule policy fields are stored but ignored. `CreateDueRun` handles one occurrence without misfire, deadline, catch-up, or concurrency decisions. | Reuse the existing policy helpers inside the due-schedule transaction. |
| High | The default OIDC callback points to `/api/v1/auth/oidc/callback`, while the SPA sends `/auth/oidc/callback`. Providers do not return nonce as a query value. | Choose one callback flow. Verify the ID-token nonce against stored state, then redirect to the SPA. |
| High | OIDC secret references are never resolved. Token exchange does not send `client_secret`. | Inject one secret resolver. Resolve the reference only during token exchange. |
| High | Account identity linking points to `/api/v1/auth/oidc/link`, which does not exist. | Add an authenticated link purpose to the existing OIDC state flow, or remove the button until supported. |
| High | The tagged integration suite does not compile. It calls deleted store APIs such as `ResourceLeaseInput` and `CreateTaskRun`. | Rewrite the suite against canonical repositories and the current dispatcher. Run it with PostgreSQL and TLS NATS. |
| Medium | Task edit responses omit working directory, selectors, environment, secrets, output limit, attempts, and ambiguity policy. | Return the active version fields and hydrate the full editor baseline. |
| Medium | Run filters ignore task, runner, trigger, and time. Task search/state, schedule task, and runner search are also ignored. | Move filtering and pagination into SQL repositories. Keep deterministic ordering and totals. |
| Medium | Dashboard labels do not match queries. Due schedules are unfiltered. Counts use page length instead of `total`. | Add small dashboard aggregate queries or use correct collection filters and totals. |
| Medium | Log fetch and download paths bypass `ApiClient` and `VITE_API_URL`. | Prefer one same-origin deployment contract. Remove the unused cross-origin option or route every request through one base URL. |
| Medium | Local registration creates the user and default role assignment in separate transactions. A role failure leaves a partial user. | Add one repository transaction for local user, password, and initial role assignment. |
| Medium | Retention, metrics, signals, and secret helpers exist only as isolated library code. The control-plane main does not start them. | Wire only retention and health metrics now. Defer notifications and approvals until product requirements exist. |
| Medium | Route access exists in three forms: mux code, route definitions, and OpenAPI JSON. Tests compare paths, not methods or permissions. | Delete unused placeholder routes. Add one contract test for method, path, access, and permission. |
| Low | Schedule preview returns one occurrence although the UI presents a list. | Return a small fixed count, such as five, using the existing scheduler function. |
| Low | Run detail claims attempt, event, session, and lease data are loaded, but it renders explanatory text only. | Return and render the actual attempt timeline before adding more run features. |

## Security risks and patches

| Severity | Description and evidence | Suggested modification |
|---|---|---|
| High | Login, refresh, and OIDC callback responses expose access and refresh tokens to JavaScript. This defeats `HttpOnly` cookies. | Set cookies, then return `204` or a token-free profile response. Reject token fields in browser requests. |
| High | The control-plane signing private key is stored in PostgreSQL configuration. Database and NATS URLs can also contain credentials. | Load private keys and connection URLs from external secrets. Keep only non-secret application settings in `config`. |
| High | Unsigned heartbeat JSON registers or replaces runner public keys. Queue ACL helpers are not enforced by the application. | Bind a runner key during one-use HTTPS enrollment. Sign later heartbeats and verify boot, key, timestamp, and subject. |
| High | HTTP request bodies are unbounded. Audit middleware copies full authenticated bodies before handlers decode them. | Apply `http.MaxBytesReader` once before audit capture. Use a small API limit and a separate artifact limit if required. |
| High | Security configuration does not fail closed. Invalid booleans silently use defaults. Plain transport is allowed when `ENVIRONMENT` is omitted. | Reject malformed booleans. Require secure transport by default and use one explicit local-development override. |
| Medium | OIDC discovery, token, and JWKS requests accept any HTTPS host and follow redirects. An administrator can configure internal targets. | Reject loopback, link-local, and private destinations. Recheck every redirect target. |
| Medium | `writeError` returns raw database and internal errors to clients. | Map known conflicts to stable codes. Log private details with the correlation ID only. |
| Medium | The authentication rate limiter retains attacker-selected keys without a bound or cleanup. | Delete expired keys and cap the map. Add a coarse source-address limit. |
| Low | The public OIDC provider endpoint returns administrative provider fields. | Return only the provider ID, display name, and optional public icon. |

No dependency vulnerability result is claimed. The external npm advisory request was unavailable under the workspace privacy policy.

## Potential new features

These features fit the current product after the critical flow works:

| Severity | Feature | Suggested modification |
|---|---|---|
| High | Restart acceptance workflow | Create auth, task, schedule, runner, run, log, and audit data. Restart only the control plane and worker. Verify recovery. |
| High | Attempt timeline | Return attempts, state events, log gaps, runner sessions, leases, and cancellation details on run detail. |
| Medium | Dead-letter operations | List rejected messages with safe metadata. Support an audited retry after the root cause is fixed. |
| Medium | Schedule enable and disable | Add one explicit state action instead of creating a new version for an operational pause. |
| Medium | Runtime metrics and maintenance | Expose low-cardinality health metrics. Run bounded cleanup for sessions, OIDC state, inbox, outbox, and logs. |
| Low | Structured selector and secret editors | Replace raw selector and secret-reference JSON with simple key/value rows. |
| Low | Operator display polish | Format dates and durations with `Intl`. Store filters in the URL. Preserve table position after mutations. |

Notifications, approvals, localization, another theme, and service splitting remain speculative. Add them only after a product requirement.

## Requested product changes

| Severity | Description and evidence | Suggested modification |
|---|---|---|
| High | Task environment variables are stored in `task_versions.environment`. The editor exposes raw JSON, and dispatch omits the values. Evidence: `frontend/src/task-editor.tsx`, `backend/migrations/001_canonical.sql:216`, and `RunStore.ClaimWaiting`. | Show `Variable Name | Variable Value` rows with add and remove actions. Validate names, sign the map, and merge it into the task process environment. |
| Medium | The application has no global variable model. Operators need reusable non-secret values such as `PYTHON_PATH` and `CACHE_PATH`. | Add `global_variables` with audited CRUD. Resolve `$(VAR_NAME)` in supported task and schedule fields. Suggest names with a native `datalist`. |
| Medium | A global variable deletion needs a strict reference guard. Text searches alone can miss references or race with new versions. | Store normalized references when task or schedule versions are published. Use PostgreSQL foreign keys to block deletion. Block renames while referenced. |
| Medium | Interval scheduling adds schema, API, store, scheduler, UI, and test branches. Evidence: `schedule_type`, `NextFire`, and `ScheduleEditorPage`. | Convert or remove existing interval rows. Remove the type column and selector, then keep one cron-only path. |
| Medium | Task and schedule forms render fields outside their applicable context. The runner selector renders without a pool. Catch-up and concurrency limits always render. | Use plain JSX conditions. Show catch-up only for `RUN_UP_TO_N` and maximum concurrency only for `ALLOW`. Omit hidden values from requests. |

Global variables must not store credentials. Keep credentials in the secret-reference system.

Resolve global values when a run is created. Record the resolved values or digest so later edits do not alter run history.

The manual-run form has no dependent fields today. Apply the same conditional rule when dependent run options are added.

## Frontend direction

Keep the current React stack. It already provides the framework, routing, query cache, icons, tests, and reusable controls.

A framework migration would not fix the current contract and execution defects. It would add churn to 63 frontend source files.

Improve the professional look through the existing components and CSS tokens. Add a UI library only when repeated missing primitives exceed the current set.

## Quality assessment

The code uses the standard library and small dependencies well. Protocol and schema tests contain many useful invariants.

Production services also retain in-memory fallback implementations. These duplicate behavior and let tests avoid PostgreSQL defects.

The lean fix is to require repositories in production services. Tests should use small repository fakes or real PostgreSQL.

## Verification

| Command | Result |
|---|---|
| `cd backend && go test ./...` | PASS, 9 tested packages plus 2 commands. PostgreSQL tests skipped without `DATABASE_URL`. |
| `cd backend && go test -race ./...` | PASS. |
| `cd backend && go vet ./...` | PASS. |
| `cd backend && go test -tags=integration ./internal/integration` | FAIL to compile due to removed store symbols. |
| `cd frontend && npm test` | PASS, 29 files and 48 tests. |
| `cd frontend && npm run lint` | PASS. |
| `cd frontend && npm run build` | PASS. Main JavaScript is 341.39 kB before gzip and 102.95 kB after gzip. |
| `npm audit --omit=dev` | NOT RUN. External advisory disclosure was not approved by the environment policy. |

The frontend tests mainly verify helpers and static contracts. They do not render the full login, editor, cancellation, or enrollment workflows.
