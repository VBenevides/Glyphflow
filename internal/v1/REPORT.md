# Glyphflow v1 design review report

Review date: 2026-08-13

## Result

The external review is technically sound. Revision 3 accepts all correctness blockers and protocol-hardening findings.

The new model preserves the v2 architecture. It tightens version activation, scheduling, placement, recovery, ordering, cancellation, leases, and runner durability.

The authentication model remains sound. Revision 3 improves its singleton constraint, OIDC state storage, audit actors, and concurrency rules.

This report describes a target design. The current Go entry points do not implement it.

## Scope

The review compared these sources:

- `internal/v1/v1-er-diagram-2.md`.
- `internal/v1/v1-er-diagram-2-external-review.md`.
- The current Go backend, migrations, queue adapter, worker store, and API.
- The AI Platform password, OIDC, session, role, permission, and seed flows.

TokenSave data was not available. The review used direct source tracing.

## External review disposition

| Finding | Decision | Revision 3 change |
|---|---|---|
| No explicit current versions | Accept | Add same-parent current-version pointers to tasks and schedules. |
| Static runner pinning | Accept | Add runner pools, membership, selectors, and optional direct pinning. |
| Missing schedule policies | Accept | Add misfire, catch-up, deadline, and concurrency policies. |
| Missing retry semantics | Accept | Add retry and ambiguity fields to immutable task versions. |
| Ambiguous execution | Accept | Add `UNKNOWN` states and an explicit ambiguity policy. |
| Exactly-once limitation | Accept | State the reliability contract without an exactly-once claim. |
| Unsafe lease expiry | Accept | Add lease state and resource-owned fencing counters. |
| Fencing limitations | Accept | State that external systems must enforce fencing tokens. |
| Duplicate event behavior | Accept | Treat duplicates as successful idempotent processing. |
| Event ordering | Accept | Separate state-event and log-chunk sequences. |
| Undefined state machines | Accept | Add formal run and attempt state machines. |
| Missing cancellation model | Accept | Add run and attempt cancellation fields and targeted cancel orders. |
| SQLite durability | Accept | Require and verify WAL, FULL synchronization, and foreign keys. |
| Runner restart ambiguity | Accept | Bind claims to a boot ID and prohibit automatic local reruns. |
| Stale orders | Accept | Add session, lease, expiry, fencing, and execution digest checks. |
| Informal signed protocol | Accept | Define canonical, versioned, key-addressed signed messages. |
| One runner state | Accept | Split desired and observed runner states. |
| Runner session arbitration | Accept | Permit one active session per runner and unique boot IDs. |
| Ambiguous certificate artifact | Accept | Store only the issued public certificate chain. |
| Self-blocking resources | Keep | Continue to use one system-managed exclusive resource per task. |
| Resource quantity and lock mode | Defer | Keep exclusive-only resources until a real shared-capacity use case exists. |
| Secret version traceability | Accept | Record resolved secret version identifiers on each attempt. |
| Execution digest | Accept | Add versioned execution-specification digests. |
| Generic run event payload | Accept | Add event kind, state sequence, reported time, and accepted time. |
| Unbounded logs | Accept | Add task output limits and require bounded retention. |
| Authentication singleton | Accept | Require `AUTH_SETTINGS.id = 1`. |
| Last administrator race | Accept | Require locked or serializable administrator changes. |
| Refresh-token families | Defer | Keep one rotating refresh token per session for v1. |
| User-only audit actors | Adjust | Add user, runner, and system actor types. Defer service accounts. |
| Object storage for logs | Defer | Keep bounded PostgreSQL chunks until volume proves the need. |
| Separate protocol document | Accept | Add it as required work. The ERD contains only the minimum contract. |

## Critical design improvements

### Explicit version activation

**Impact / Severity Level:** Critical

**Description:** Version ordering does not define the active task or schedule version. Concurrent edits can select different versions.

**Modification:** Add `current_version_id` to tasks and schedules. Use same-parent composite foreign keys.

Create and activate a version in one transaction. Update a schedule's next-fire time in the same transaction.

### Dynamic runner placement

**Impact / Severity Level:** Critical

**Description:** A required runner ID makes one runner failure a task availability failure.

**Modification:** Target a runner pool and placement selectors. Keep `pinned_runner_id` as an optional exception.

Record the selected runner and active runner session on each attempt.

### Complete schedule policy

**Impact / Severity Level:** Critical

**Description:** A cron expression does not define missed work, queue deadlines, or overlap behavior.

**Modification:** Add misfire, catch-up, start-deadline, concurrency, and maximum-concurrency fields to schedule versions.

Lock a due schedule while the transaction creates a run and advances `next_fire_at`.

### Retry and ambiguity policy

**Impact / Severity Level:** Critical

**Description:** Multiple attempts do not state which failures permit retry. A lost runner can leave an unproven external result.

**Modification:** Store immutable retry fields and an ambiguity policy on each task version.

Use `UNKNOWN` when the platform cannot prove an outcome. Do not treat it as a normal failure.

### Resource takeover

**Impact / Severity Level:** Critical

**Description:** An expired lease with no release time can block a partial unique index forever.

**Modification:** Give leases `ACTIVE`, `RELEASED`, and `EXPIRED` states. Generate fencing tokens while locking the resource row.

Expire the old lease and create the new lease in one transaction.

### Event ordering and duplicate delivery

**Impact / Severity Level:** Critical

**Description:** At-least-once delivery creates duplicates. Out-of-order events can otherwise move a terminal attempt back to a running state.

**Modification:** Deduplicate by event ID. Use independent state and log sequences.

Apply only a newer legal state transition. Treat an already committed duplicate as success and acknowledge it.

### Cancellation

**Impact / Severity Level:** Critical

**Description:** Authorization includes cancellation, but v2 does not model cancellation state or message races.

**Modification:** Record cancellation on the run and attempt. Target each cancel order to one attempt, lease, and fencing token.

A late cancel cannot change a successful attempt or affect a newer attempt.

## Protocol and runner improvements

### Signed message contract

**Impact / Severity Level:** High

**Description:** The v2 phrase "signed messages" does not define stable signed bytes or required freshness fields.

**Modification:** Use canonical, versioned binary encoding. Include the key ID, recipient, expiry, lease, fencing token, and execution digest.

Store public control-plane and runner signing keys. Keep private keys outside PostgreSQL.

### Runner durability and restart recovery

**Impact / Severity Level:** High

**Description:** A SQLite commit is not durable enough without explicit settings. A process start is not atomic with that commit.

**Modification:** Require WAL, FULL synchronization, and foreign-key enforcement. Fail runner startup if verification fails.

Bind each claimed order to a boot ID. Mark an older boot's unfinished order as unknown after restart.

### Desired and observed runner state

**Impact / Severity Level:** High

**Description:** Administrative intent and measured health have different meanings.

**Modification:** Split desired state from observed state. Use `DRAINING` to stop new placement while current work finishes.

Permit one active session for each runner identity.

### Reproducible execution

**Impact / Severity Level:** High

**Description:** A secret reference can resolve to a different secret version for the same task version.

**Modification:** Record resolved secret version identifiers on the attempt. Never store secret values.

Sign the immutable execution digest in each order and verify it before execution.

## Authentication and authorization assessment

The v2 authentication design remains suitable for v1.

- A local user can have a password, SSO identities, or both.
- Password login can be disabled without disabling SSO.
- Both login methods create the same revocable session.
- Roles contain seeded permissions.
- Users can receive multiple manual, system, and SSO roles.
- Administrators can create custom roles from seeded permissions.
- Permission changes apply on the next request.

Revision 3 stores a hashed nonce and encrypted PKCE verifier. It keeps SSO client secrets behind secret references.

The database enforces the authentication singleton. Locked transactions protect the last administrator.

## Current implementation gaps

### Static API bearer token

**Impact / Severity Level:** Critical

**Description:** `backend/internal/api/api.go` compares one configured token and returns a fixed permission map.

**Modification:** Replace it after user sessions, the bootstrap administrator, and permission middleware work.

### Missing v1 schema

**Impact / Severity Level:** Critical

**Description:** The current migrations implement the v0 task, run, runner, event, lease, inbox, and outbox model.

**Modification:** Implement revision 3 through versioned migrations. Do not change applied v0 migration files.

### Unwired production flow

**Impact / Severity Level:** Critical

**Description:** The control-plane command serves the HTTP API. The worker command validates configuration and exits.

**Modification:** Wire scheduler, dispatcher, NATS consumers, executor, and outbox producers in dependency order.

### Simplified worker store

**Impact / Severity Level:** High

**Description:** The current SQLite table stores one JSON value per message. It does not model claims, recovery, or event publication.

**Modification:** Replace it with revision 3 runner tables and verified durability settings.

## Permission catalog

Permission keys belong to application code. Custom roles select these keys but cannot create new keys.

| Endpoint group | Permission |
|---|---|
| Users and sessions | `users.read`, `users.manage` |
| Roles and assignments | `roles.read`, `roles.manage` |
| SSO providers and mappings | `sso.read`, `sso.manage` |
| Authentication settings | `auth.settings.manage` |
| Tasks and schedules | `tasks.read`, `tasks.manage` |
| Runs | `runs.read`, `runs.execute`, `runs.cancel`, `runs.retry` |
| Execution output | `logs.read` |
| Exclusive resources | `resources.read`, `resources.manage` |
| Runners and enrollment | `runners.read`, `runners.manage` |
| Audit events | `audit.read` |

Health and login entry routes remain public. Account linking and session management require authentication.

## Reliability statement

Glyphflow can guarantee one durable logical run for each created scheduled occurrence.

Glyphflow uses at-least-once physical delivery and idempotent database processing.

Glyphflow cannot guarantee exactly-once external side effects. A task must pass `run_id` to a cooperative downstream system.

Fencing tokens help only when the protected system rejects old tokens.

An unknown result follows the task version's ambiguity policy. Automatic retry is not always safe.

## Recommended delivery order

1. Write and approve the execution protocol and state machines.
2. Add revision 3 migrations and database constraints.
3. Implement seeds, bootstrap administration, sessions, and permission middleware.
4. Implement task versions, schedule versions, and due-schedule transactions.
5. Implement dynamic placement, attempts, resource takeover, and dispatch outbox.
6. Implement the signed message contract and runner SQLite recovery.
7. Implement event ordering, retry, ambiguity, cancellation, and run aggregation.
8. Add failure-injection tests for every reviewed crash boundary.

Do not claim critical-workload readiness until these steps pass with PostgreSQL, NATS, and real process execution.
