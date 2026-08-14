# Canonical Database Persistence Roadmap

## Objective

Replace the current mixed legacy/`_v1` schema and CONFIG-backed state snapshots with one canonical PostgreSQL schema. All durable application state must use its domain table. The `config` table must contain configuration only. Role references, including the default role and SSO group mappings, must use role IDs. Role names must be unique case-insensitively.

## Scope decisions

- Glyphflow is alpha software. Do not preserve or migrate existing application data.
- Do not add compatibility views, legacy tables, aliases, dual writes, or version-suffixed tables.
- Replace the migration history with one canonical baseline migration. Reset the local PostgreSQL volume before testing it.
- Keep the worker's local SQLite inbox/outbox database. This roadmap concerns the control-plane PostgreSQL schema.
- Keep JSONB only for genuinely structured fields such as commands, selectors, policies, capabilities, audit payloads, and CONFIG values. Do not store whole service snapshots in JSONB.
- Reuse the existing PostgreSQL driver and Argon2id dependency. Do not add an ORM or another password library.
- Implement one unchecked item at a time. Run its listed tests before checking it. If a test fails, fix the implementation before continuing.
- Do not change the **How to test** column while implementing.
- After each completed item, use a commit such as `feat(database): add canonical identity schema` and record the hash and result in **Commit / Test result**.
- The project is still in development and there is no need to keep backward compatibility at this stage

## Target canonical tables

The final PostgreSQL schema should contain one table for each concept:

```text
config
users
user_passwords
auth_sessions
roles
permissions
role_permissions
role_assignments
sso_providers
sso_group_role_mappings
user_sso_identities
sso_authorization_states
audit_events
runner_pools
runners
runner_sessions
runner_keys
runner_enrollments
tasks
task_versions
schedules
schedule_versions
runs
execution_attempts
run_events
execution_log_chunks
resources
task_resource_requirements
resource_leases
dispatch_outbox
event_inbox
```

`schema_migrations` is migration-runner metadata and is not an application table.

## Phase 1: Analysis guardrails and canonical schema

### Current frontend/backend contract baseline

- `backend/internal/api/routes.go` registers 45 route patterns: 9 public, 1 authenticated, and 35 permission-protected.
- `frontend/src/api.ts` defines 17 API models: `ApiErrorBody`, `Page`, `Identity`, `Profile`, `PermissionSnapshot`, `RuntimeConfig`, `OidcProvider`, `Task`, `Schedule`, `Run`, `Runner`, `Resource`, `AuditEvent`, `AuthSession`, `UserRecord`, `RoleDefinition`, and `QueryValue`.
- Mutable control-plane maps currently exist in authentication/users/passwords/roles/assignments, sessions/refresh families, OIDC providers/state, audit, tasks/schedules, runs/logs, runners/resources/enrollments, role administration, and store dispatch/versioning. Platform-only maps include rate limiting, signals, event tracking, secrets, service accounts, leases, runner sessions, failure injection, and administrator state.
- Known contract drift to resolve in later items: `RoleDefinition` still exposes optional `id` plus legacy `key`, `RuntimeConfig` still exposes `defaultRole`, and authentication settings still use role names at runtime.

| Task | Description | Impact | How to implement | How to test | Commit / Test result |
|---|---|---|---|---|---|
| [x] Critical: Record the current frontend/backend contracts | Prevent accidental API breakage while persistence is replaced. | Establishes a verifiable baseline before schema changes. | Inventory every route from `backend/internal/api/routes.go`, every response type from `frontend/src/api.ts`, and every mutable in-memory map in `backend/internal/api` and `backend/internal/platform`. Add a short contract section below this roadmap if an undocumented field is discovered. Do not change behavior in this item. | Run `cd backend && go test ./...`; run `cd frontend && npm run typecheck && npm test -- --run && npm run build`. Record baseline counts. | 27c0cb8 — PASS: backend `go test ./...` (9 packages); frontend typecheck/tests/build (45 tests, 29 files). |
| [x] Critical: Replace migration history with one canonical baseline | Remove all legacy and `_vN` tables instead of maintaining multiple generations. | Gives new and reset databases exactly one schema. | Delete the current SQL migration files and add one baseline migration containing the target table list above. Use unsuffixed names only. Include foreign keys, checks, timestamps, case-insensitive unique indexes, active-row partial indexes, and append-only protection for `audit_events` and `run_events`. Do not add rename/copy/compatibility SQL because the database is disposable. Update migration tests to inspect the canonical file. | Reset with `docker compose down -v`, start PostgreSQL, run the control plane once, and query `pg_tables`. `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename ~ '_v([0-9]+|x)$';` must return zero rows. Run `go test ./internal/store`. | afcf043 — PASS: `GOCACHE=/tmp/glyphflow-go-cache go test ./internal/store`; reset PostgreSQL/control-plane migration; canonical `pg_tables` query (0 suffixed tables, 1 canonical migration). |
| [x] Critical: Define canonical identity constraints | Make identity data valid independently of application code. | Prevents duplicate users, duplicate role names, orphaned assignments, and unsupported password formats. | Require non-null normalized email and username fields. Add unique indexes on `lower(email)`, `lower(username)`, and `lower(roles.name)`. Add `roles.id` as the role primary key. Require `user_passwords.password_hash` to be an Argon2id PHC string. Use foreign keys with deliberate delete behavior. System roles must be marked immutable. | Add migration tests for duplicate email, duplicate role name with different case, orphaned role assignment, malformed password hash, and system-role fields. Each invalid insert must fail. | pending — `GOCACHE=/tmp/glyphflow-go-cache go test ./internal/store`: PASS; PostgreSQL constraint checks: duplicate role, orphan assignment, malformed hash, and normalized email rejected; system-role update/delete rejected. |
| [ ] High: Define canonical execution constraints | Preserve scheduler, runner, leasing, outbox, and append-only guarantees in the new schema. | Prevents duplicate dispatch, split-brain leases, and corrupt run history. | Port only the strongest constraints from the current execution schemas: immutable task/schedule versions, unique logical schedule occurrences, optimistic run state version, one active runner session, one unused enrollment, one active resource lease, unique event sequence, durable dispatch outbox, idempotent event inbox, append-only run events, ordered log chunks. | Add schema tests for duplicate schedule occurrence, duplicate event ID/sequence, active lease conflict, second active runner session, second unused enrollment, and UPDATE/DELETE rejection on run events. | Pending |
| [ ] High: Make CONFIG configuration-only by schema and repository policy | Prevent service state from returning to CONFIG. | Enforces the requested persistence boundary. | Keep `config(name, value, updated_at)`. Add a check rejecting names beginning with `state.`. Add an allowlist in `ConfigStore` for `ENABLE_PASSWORD_LOGIN`, `ENABLE_PASSWORD_REGISTRATION`, `DEFAULT_ROLE_ID`, `DATABASE_URL`, `NATS_URL`, `WEB_ORIGIN`, `MAX_MESSAGE_BYTES`, `GLYPHFLOW_BOOTSTRAP_EMAIL`, and `GLYPHFLOW_SYSTEM_ADMINS`. Remove old `DEFAULT_ROLE`. Never store password, pepper, access-token secret, OIDC client secret, or refresh token there. | Test that every allowed key round-trips, an unknown key fails, `state.auth` fails, and no secret key can be inserted through `ConfigStore`. Query `SELECT name FROM config WHERE name LIKE 'state.%';`; it must return zero rows. | Pending |

## Phase 2: Identity, roles, and authentication persistence

| Task | Description | Impact | How to implement | How to test | Commit / Test result |
|---|---|---|---|---|---|
| [ ] Critical: Replace AuthService maps with identity repositories | Store users and password hashes in `users` and `user_passwords`. | Makes registration, login, bootstrap, profile updates, and user disablement durable. | Add small pgx repositories in `backend/internal/store` for users and passwords. Inject them into `AuthService`. Replace `users`, `byEmail`, and `passwords` maps. Use transactions for registration/bootstrap. Normalize and validate email before insert. Return generic login/registration errors while preserving specific internal errors. Remove the corresponding sections from `backend/internal/api/persistence.go`. | Register a user, verify rows in both tables, restart only the control plane, and log in. Verify duplicate-case email fails. Verify an invalid email never inserts. Run auth unit and integration tests. | Pending |
| [ ] Critical: Consolidate the duplicate role services | Make authorization and role administration use the same role records. | Fixes the current split where `AuthService` and `RoleAdminService` can disagree. | Remove independent role/permission/assignment maps from `AuthService` and `RoleAdminService`. Add one role repository/service backed by `roles`, `permissions`, `role_permissions`, and `role_assignments`. Seed `admin` and `user` once using stable opaque IDs. Keep permission evaluation in one shared method. Custom role create/update/delete must immediately affect authorization. | Create a custom role, assign it, verify its permission grants access, update permissions, verify access changes, restart, and repeat. Verify system roles cannot be renamed/deleted. Verify duplicate names differing only by case fail. | Pending |
| [ ] Critical: Use role IDs in every backend contract | Stop treating mutable role names as identifiers. | Makes renaming safe and satisfies the default-role requirement. | Make `RoleDefinition.id` required and `name` the unique display/lookup name. Change role routes to `/api/v1/admin/roles/{role_id}`. Change assignments, SSO group mappings, and auth settings requests to `role_id` fields. `DEFAULT_ROLE_ID` must contain an existing role ID. Validate it on startup and update. Audit role changes with both ID and name. | Add tests proving a role can be renamed without breaking assignments/default-role/SSO mappings. Invalid or deleted role IDs must be rejected. API contract tests must reject a role name where an ID is required. | Pending |
| [ ] Critical: Persist authentication settings transactionally | Make password-login, registration, and default-role changes survive restart without CONFIG snapshots. | Keeps authentication behavior consistent across frontend, backend, and restarts. | Store the three settings as individual CONFIG rows. Update them through one transaction after validating `DEFAULT_ROLE_ID`. The backend must continue to reject password login, registration, and password changes when disabled. Return `defaultRoleId` from `/api/v1/config`. Remove `defaultRole`. | Disable each password feature through the admin endpoint and verify the backend endpoint is blocked before and after restart. Change default role by ID, register a user, and verify the assignment row uses that ID. | Pending |
| [ ] Critical: Persist sessions with atomic refresh rotation | Remove in-memory access/refresh session state. | Prevents logout/session loss on restart and supports safe token replay handling. | Replace `SessionManager.sessions` and `RefreshSessionManager.sessions` with `auth_sessions`. Store only refresh-token hashes. Include access expiry, refresh expiry, revoked time, user agent, IP, last-seen time, and a session-family ID. Rotate refresh tokens in a transaction using row locking; revoke the family on replay. Authentication must verify the signed access token and an active DB session. Logout, logout-all, disable-user, and admin revoke must update rows. | Login, restart, call `/me`, refresh, and logout. Verify refresh replay revokes the family. Verify disabling a user invalidates all sessions. Run concurrent refresh requests and assert only one succeeds. Ensure plaintext refresh/access tokens never appear in PostgreSQL. | Pending |
| [ ] High: Harden the existing Argon2id implementation | Keep advanced hashing and make upgrades safe. | Improves resistance to offline cracking and timing/user-enumeration attacks. | Keep Argon2id and PHC strings. Combine the deployment pepper with the password using HMAC-SHA-256 before Argon2id instead of appending the pepper to the salt. Keep a random salt per password. Use a fixed dummy Argon2id verification for unknown users. After successful login, call `NeedsRehash` and replace an outdated hash transactionally. Document memory/time/parallelism defaults and make them configurable only through non-secret environment variables if needed. | Verify equal passwords produce different hashes, malformed hashes fail safely, unknown and known-user failures perform one Argon2id operation, old parameters rehash on login, wrong pepper fails, and plaintext/password/pepper never reaches logs, audit, CONFIG, or PostgreSQL. | Pending |
| [ ] High: Persist immutable system-admin enforcement | Keep deployment-selected administrators durable and non-demotable. | Prevents accidental loss of administrative control. | Read `GLYPHFLOW_SYSTEM_ADMINS` from configuration, normalize emails, and derive system-admin assignments in a transaction. Store assignment source as `system-admin`. Reject disable, demotion, or assignment deletion for configured emails. Reconcile additions/removals at startup without writing an auth snapshot. Protect the last active administrator independently. | Start with two configured admins, attempt disable/demotion, and verify rejection. Remove one email from configuration, restart, and verify only the derived source is removed while explicit assignments remain. | Pending |

## Phase 3: SSO persistence and security

| Task | Description | Impact | How to implement | How to test | Commit / Test result |
|---|---|---|---|---|---|
| [ ] Critical: Persist OIDC providers without secrets | Store provider configuration in `sso_providers`. | Makes SSO configuration restart-safe while keeping credentials outside the database/UI. | Replace `OIDCService.providers` with repository reads/writes. Store provider ID, unique name, issuer, client ID, callback allowlist, auth endpoint override, audience, enabled flag, auto-provision flag, and secret reference. Resolve the client secret at runtime from the secret reference; never return or audit the secret value. | Create/toggle a provider, restart, and verify it remains. Verify duplicate-case provider names fail, unapproved callback URLs fail, and API/audit responses contain only secret references. | Pending |
| [ ] Critical: Persist OIDC authorization state securely | Make in-flight SSO safe across process restart. | Prevents failed callbacks and protects PKCE verifier/state values. | Store only hashed state and nonce in `sso_authorization_states`. Encrypt the PKCE verifier using a key derived from an external application secret, not a database value. Consume state atomically once, enforce expiry/provider/purpose/callback, and delete expired rows. Do not store the state/nonce plaintext. | Start OIDC login, restart the control plane, complete callback successfully once, then verify replay/expired/wrong callback/wrong provider attempts fail. Inspect PostgreSQL for plaintext state, nonce, or verifier. | Pending |
| [ ] High: Persist SSO identities and group-role mappings by ID | Keep federated identity links and authorization stable across role renames. | Prevents duplicate identities and stale name-based mappings. | Store identities in `user_sso_identities` with unique `(provider_id, provider_subject)`. Store group mappings in `sso_group_role_mappings(provider_id, group_name, role_id)`. Auto-provision users and assignments in one transaction. Mark mapping-derived assignments with a source so manual controls cannot delete them directly. | Link/unlink identity, restart, and verify state. Attempt duplicate subject linking and expect conflict. Rename a role and verify group mapping still grants it. Verify removing a group mapping removes only its derived assignment. | Pending |

## Phase 4: Operations and infrastructure persistence

| Task | Description | Impact | How to implement | How to test | Commit / Test result |
|---|---|---|---|---|---|
| [ ] Critical: Persist tasks and immutable versions | Replace `OperationsService.tasks` with `tasks` and `task_versions`. | Makes task creation and editing durable and preserves execution inputs. | Add repository methods for paginated list/get/create/create-version. Create the task and first version in one transaction. Store command arguments as a JSON array, runner pool ID, placement, environment, secret references, timeout/output/retry/ambiguity/resource policies. Advance `current_version_id` only after the new immutable version commits. Never UPDATE a version row. | Create a task, add a version, restart, and verify both versions and active pointer. Attempt version UPDATE/duplicate number and expect failure. Verify a failed version insert does not move the pointer. | Pending |
| [ ] Critical: Persist schedules and immutable versions | Replace `OperationsService.schedules` with `schedules` and `schedule_versions`. | Makes scheduling durable and audit-friendly. | Add repository methods for list/get/create/create-version/enable/disable. Store task-version ID, cron/interval expression, timezone, boundaries, misfire/catch-up/deadline/concurrency policy. Update active version and next fire time transactionally. Never UPDATE a version row. | Create/update a schedule, restart, preview occurrences, and verify history. Test invalid timezone/expression, DST gap/repetition, duplicate version, and failed-pointer rollback. | Pending |
| [ ] Critical: Persist runs, attempts, events, and logs | Replace `RunService.runs` and `RunService.logs`. | Makes run history and live output durable. | Use `runs`, `execution_attempts`, `run_events`, and `execution_log_chunks`. Create manual/scheduled/retry runs with idempotency keys. Use optimistic state versions. Append events/log chunks with unique sequences. Paginate/filter in SQL. Keep stdout/stderr separate and resume by sequence. | Start a run, append attempts/events/logs, restart, and verify detail/log resume. Test duplicate idempotency key, invalid transition, stale version, duplicate event/chunk, and append-only enforcement. | Pending |
| [ ] Critical: Persist runners, sessions, keys, and enrollment | Replace `InfrastructureService.runners` and enrollment maps. | Makes runner lifecycle secure and restart-safe. | Use `runner_pools`, `runners`, `runner_sessions`, `runner_keys`, and `runner_enrollments`. Store enrollment token hashes only. Atomically consume one-use enrollment. Keep one active runner session. Persist heartbeat/desired/observed state and key revocation. Use database time for expiry decisions. | Enroll a runner, restart before consumption, consume once, and verify replay fails. Test second active session, expired token, revoked key, stale heartbeat, and state transitions. Inspect DB for plaintext enrollment tokens. | Pending |
| [ ] Critical: Persist resources and leases transactionally | Replace `InfrastructureService.resources` and in-record lease fields. | Prevents double allocation and preserves fencing tokens. | Use `resources`, `task_resource_requirements`, and `resource_leases`. Acquire with a transaction and row lock, increment the resource fencing token, and enforce one active lease with a partial unique index. Release only for matching holder/token. Never reset fencing tokens. | Acquire/release across restart. Race two acquisitions and assert one succeeds. Verify expired lease replacement receives a larger fencing token and stale release fails. | Pending |
| [ ] High: Unify dispatch outbox and event inbox with canonical runs | Remove the old `task_definitions`/`task_runs` persistence path. | Keeps queue delivery and API run state in one model. | Update `backend/internal/store/runs.go`, `state.go`, and integration tests to use canonical `tasks`, `task_versions`, `runs`, `execution_attempts`, `resource_leases`, `dispatch_outbox`, and `event_inbox`. Create run, attempts, leases, and outbox atomically. Apply incoming event deduplication and state transition in one transaction. | Run integration tests against PostgreSQL/NATS. Inject duplicate dispatch/events and transaction failures. Verify no orphan run/lease/outbox rows and exactly-once state application. | Pending |

## Phase 5: Audit, request durability, and cleanup

| Task | Description | Impact | How to implement | How to test | Commit / Test result |
|---|---|---|---|---|---|
| [ ] Critical: Persist audit events directly and append-only | Replace `AuditQueryService.events`. | Makes security history durable and tamper-resistant. | Insert REST audit events directly into `audit_events` after the domain transaction result is known. Store timestamp, actor ID/name/email, method, description, target, result, request/input/output/before/after, traceback, and correlation ID. Redact before insert. Reject UPDATE/DELETE with a DB trigger. Query/filter/page in SQL. | Perform successful and failed mutations, restart, and query audit. Verify sensitive fields are redacted, correlation/traceback persist, pagination/filtering work, and UPDATE/DELETE fail. | Pending |
| [ ] Critical: Remove CONFIG snapshot persistence completely | Delete the temporary whole-application snapshot mechanism. | Prevents accidental dual persistence and state loss. | Delete `backend/internal/api/persistence.go` state structs/constants and the request wrapper that serializes services. Remove `state.*` initialization/restore/save/shutdown calls from `main.go`. Remove snapshot-only methods from session/OIDC helpers unless still required by unit tests. Every mutation must return success only after its domain transaction commits. | `rg 'state\.(auth|sessions|refresh|roles|oidc|audit|operations|infrastructure|runs)' backend` must return no runtime references. Query CONFIG and confirm only allowlisted configuration rows exist. Force a DB write failure and verify the API returns failure without reporting success. | Pending |
| [ ] High: Add database transaction/error conventions | Make persistence failures observable and consistent. | Avoids partial writes and false-success responses. | Add one small error mapping layer for unique/FK/serialization/deadlock/timeouts. Use request contexts and bounded query timeouts. Retry only safe transaction serialization/deadlock failures with a small bound. Return correlation IDs and structured `409`/`422`/`503` responses. Never log SQL parameters containing credentials/tokens. | Unit-test pg error mapping. Terminate DB connections during mutations and verify no success response/partial state. Run concurrent role/session/lease/run transition tests. | Pending |
| [ ] Medium: Add retention and maintenance queries | Bound high-volume durable tables. | Prevents unbounded session/state/log/audit growth. | Add indexes and maintenance functions for expired sessions, consumed/expired OIDC states, used enrollments, old inbox/outbox rows, and log retention. Audit retention must be explicit and disabled by default. Use batches and `SKIP LOCKED` where workers can overlap. | Seed large expired/active sets, run cleanup, and verify only eligible rows are removed without long locks. Check query plans use intended indexes. | Pending |

## Phase 6: Frontend/backend compatibility

| Task | Description | Impact | How to implement | How to test | Commit / Test result |
|---|---|---|---|---|---|
| [ ] Critical: Change frontend role models and routes to IDs | Keep UI compatible with the new backend role contract. | Prevents default-role, role editing, and SSO mapping regressions. | In `frontend/src/api.ts`, require `RoleDefinition.id` and replace `key` with `name` where appropriate. In `admin-pages.tsx`, set option values and route parameters to `role.id`; display `role.name`. Send `default_role_id` and role IDs in assignments/group mappings. Rename runtime config field to `defaultRoleId`. Update unsaved-state logic after successful saves. | Run component/workflow tests for role create/edit/delete, auth settings, SSO mappings, tab changes, and refresh. Verify names can change while selected/default mappings remain stable. Run typecheck and build. | Pending |
| [ ] High: Verify every collection remains paginated/filter-compatible | Prevent table-backed APIs from breaking existing pages. | Keeps frontend behavior stable with SQL queries. | Compare backend response fields/query parameters with frontend `Page<T>` consumers for users, roles, tasks, schedules, runs, runners, resources, and audit. Implement server pagination and deterministic ordering. Keep camelCase responses and accepted snake_case request fields unless deliberately changed and updated together. | Run all frontend tests plus API contract tests. Seed more than one page for every collection and verify navigation/filtering does not duplicate or omit rows. | Pending |
| [ ] High: Add restart workflow coverage | Prove frontend-visible state survives control-plane restart. | Validates the main purpose of the change end-to-end. | Add a script/integration suite that creates a user/session, custom role, SSO provider, task/version, schedule/version, runner/enrollment, resource/lease, run/log, and audit event through APIs; restart only the control plane; verify all through APIs. Keep PostgreSQL and NATS running during restart. | Run the restart suite twice against a reset database. Verify no CONFIG `state.*` rows and no `_vN` tables after both runs. | Pending |

## Phase 7: Final verification

| Task | Description | Impact | How to implement | How to test | Commit / Test result |
|---|---|---|---|---|---|
| [ ] Critical: Run the complete backend release gate | Confirm schema, security, persistence, and concurrency behavior. | Blocks handoff if backend is incomplete. | Fix all failures; do not weaken assertions. Include race detection and integration-tagged tests with PostgreSQL/NATS. | Run `cd backend && go test ./...`; `go test -race ./...`; and the integration suite with required environment variables. Run `go vet ./...` if available. All must pass. | Pending |
| [ ] Critical: Run the complete frontend release gate | Confirm frontend/backend compatibility. | Blocks handoff if UI contracts are stale. | Fix all failures; do not add fallback data that masks API errors. | Run `cd frontend && npm run typecheck && npm test -- --run && npm run build`. All must pass. | Pending |
| [ ] Critical: Inspect the final database | Confirm the requested architecture exists in PostgreSQL. | Catches accidental snapshots, suffixes, secrets, or empty domain tables. | Use SQL to list tables, constraints, indexes, CONFIG names, and representative rows after the restart suite. Verify users/passwords/sessions/domain data are in their own tables. Verify password/token fields contain hashes only. | `_vN` table query returns zero; CONFIG `state.%` query returns zero; every created object exists in its canonical table; plaintext test password/tokens do not appear in `pg_dump --data-only`. | Pending |

## Final acceptance checklist

- [ ] PostgreSQL contains no `_v1`, `_v2`, `_vX`, compatibility, or legacy application tables.
- [ ] `config` contains configuration only and rejects `state.*` keys.
- [ ] Users, Argon2id password hashes, sessions, roles, permissions, and assignments use identity tables.
- [ ] `DEFAULT_ROLE_ID` stores and returns a role ID; role names are case-insensitively unique.
- [ ] Custom role changes affect authorization immediately and survive restart.
- [ ] OIDC providers, identities, mappings, and one-use authorization states survive restart securely.
- [ ] Tasks, schedules, runs, logs, runners, resources, leases, outbox/inbox, and audit events survive restart.
- [ ] No plaintext password, pepper, access token, refresh token, enrollment token, OIDC state, nonce, verifier, or client secret is stored or logged.
- [ ] Backend and frontend release gates pass.
