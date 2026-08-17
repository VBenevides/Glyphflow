# Glyphflow Scheduler ERD Review

## Executive summary

The current Glyphflow beta schema is a major improvement over the original ER diagram. It now models a real distributed execution system rather than only task configuration and logs.

The strongest additions are:

- immutable task and schedule versions;
- logical `RUNS` separated from physical `EXECUTION_ATTEMPTS`;
- runner sessions and runner keys;
- resource leases and fencing tokens;
- PostgreSQL transactional outbox/inbox patterns;
- runner-local SQLite inbox/outbox durability;
- explicit `scheduled_for` timestamps;
- RBAC, password auth, OIDC SSO, sessions, and audit records;
- explicit scheduler and authentication invariants.

The architecture is good enough to build a beta around, but it is not yet ready to claim strong guarantees for critical workloads. The main remaining work is not another redesign of the data model. It is tightening the correctness semantics around version selection, runner assignment, scheduling policies, retries, ambiguous execution, resource takeover, cancellation, event ordering, and crash recovery.

The most important design principle is:

> The system can guarantee one durable logical run per scheduled occurrence, but it cannot guarantee exactly-once external side effects without cooperation from the task or downstream system.

---

# 1. What is already strong

## 1.1 Logical run vs. physical attempts

The separation:

```text
TASK
  -> TASK_VERSION
  -> SCHEDULE_VERSION
  -> RUN
  -> EXECUTION_ATTEMPT
```

is the correct core execution model.

A logical scheduled occurrence should be represented once by `RUNS`, while every retry, failover, or reassignment becomes a separate `EXECUTION_ATTEMPT`.

This allows the platform to represent:

```text
Run abc
scheduled_for = 2026-08-14 02:00

Attempt 1 -> runner 42 -> RUNNER_LOST
Attempt 2 -> runner 18 -> SUCCEEDED
```

That is substantially better than representing execution only as a log row.

## 1.2 Immutable task and schedule versions

Keeping task and schedule versions immutable is the right approach.

A historical run must continue to reference the exact task definition and schedule definition used when it was created.

This is essential for:

- forensic debugging;
- auditing;
- incident analysis;
- reproducing old behavior;
- safe concurrent edits.

## 1.3 Durable message flow

The PostgreSQL -> NATS JetStream -> runner SQLite -> NATS -> PostgreSQL flow is a strong design.

The transaction boundaries are especially good:

```text
PostgreSQL transaction:
    attempt
    leases
    dispatch outbox

then publish

Runner:
    SQLite transaction
    order inbox
    ACK

Runner:
    event outbox

Control plane:
    PostgreSQL event inbox + domain state update
    then ACK
```

The important correctness property is that broker acknowledgement only happens after the corresponding durable database transaction commits.

## 1.4 Runner-local SQLite durability

`ORDER_INBOX` and `EVENT_OUTBOX` are excellent additions.

They let a runner survive process restarts without losing accepted orders or events waiting to be published.

## 1.5 Resource leases

Representing self-blocking and other concurrency restrictions as resources is a clean design.

It is better than adding special scheduler booleans such as:

```text
is_self_blocking
exclusive_resource
```

A single locking mechanism is easier to reason about.

## 1.6 Authentication and authorization

The beta schema also makes strong security improvements:

- password credentials are separated from users;
- refresh tokens are stored hashed;
- OIDC identities are normalized;
- OIDC state/nonce/PKCE data is modeled explicitly;
- SSO client secrets are references rather than raw secrets;
- role assignments record their source;
- permission keys are seed-owned;
- audit events have useful request and object context.

This part is significantly more mature than the original model.

---

# 2. Critical finding: no explicit current version

The schema has:

```text
TASKS
TASK_VERSIONS

SCHEDULES
SCHEDULE_VERSIONS
```

but no explicit pointer identifying the currently active version.

That creates ambiguity.

Avoid application logic such as:

```sql
SELECT *
FROM task_versions
WHERE task_id = ?
ORDER BY version DESC
LIMIT 1;
```

The database should explicitly record the active version.

Recommended change:

```text
TASKS
-----
id
name
current_version_id FK
...

SCHEDULES
---------
id
task_id
current_version_id FK
next_fire_at
...
```

Changing a task becomes:

```text
BEGIN

INSERT task_version version 18

UPDATE tasks
SET current_version_id = <version 18 id>

COMMIT
```

Changing a schedule should atomically update:

```text
current_version_id
next_fire_at
updated_at
```

This prevents `next_fire_at` from being calculated according to one schedule version while application code considers another version current.

---

# 3. Critical finding: task versions are still statically pinned to runners

The current task version contains:

```text
runner_id
```

That recreates one of the limitations of the original design.

If a task is permanently pinned to one runner, a runner failure becomes a task availability failure.

The runner schema already contains:

```text
capacity
capabilities
operating_system
architecture
```

which suggests the system is moving toward dynamic placement.

Recommended model:

```text
RUNNER_POOLS
------------
id
name

RUNNER_POOL_MEMBERS
-------------------
runner_pool_id
runner_id

TASK_VERSIONS
-------------
...
runner_pool_id
```

or selector-based matching:

```json
{
  "os": "linux",
  "environment": "production",
  "region": "eu-west"
}
```

The actual physical runner should be recorded on:

```text
EXECUTION_ATTEMPTS.runner_id
```

Direct pinning can still exist as a special case:

```text
pinned_runner_id nullable
```

but it should not be the default execution model.

---

# 4. Critical finding: scheduling policies are incomplete

The schedule version currently defines:

```text
schedule_type
expression
timezone
starts_at
ends_at
```

This is not enough for critical scheduling.

At minimum add:

```text
misfire_policy
start_deadline_seconds
concurrency_policy
max_concurrent_runs
catchup_limit
retry_policy_id
```

## 4.1 Misfire policy

If the scheduler is unavailable from 01:00 until 05:00 and occurrences were expected at:

```text
01:30
02:30
03:30
04:30
```

the schema must define whether to:

```text
SKIP_ALL
RUN_LATEST
RUN_ALL
RUN_UP_TO_N
FAIL_AND_ALERT
```

## 4.2 Start deadline

A run scheduled for 01:30 but only getting a runner at 11:00 may no longer be safe or useful to execute.

That policy is different from an execution timeout.

Use:

```text
start_deadline_seconds
```

## 4.3 Concurrency semantics

Serialization alone is insufficient.

When a previous run is still active, the next occurrence may need to:

```text
QUEUE
SKIP
REPLACE
ALLOW
```

That policy should be explicit.

---

# 5. Critical finding: retry behavior is not yet modeled

The schema supports multiple attempts per run, but does not define why a retry should happen.

A retry policy should contain concepts such as:

```text
max_attempts
initial_backoff_seconds
max_backoff_seconds
backoff_multiplier
retryable_exit_codes
retryable_termination_reasons
```

A separate ambiguity policy is also required.

For example:

```text
ambiguity_policy =
    RETRY
    REQUIRE_MANUAL_RECONCILIATION
    MARK_FAILED
```

---

# 6. Critical finding: ambiguous execution must be first-class

This is one of the most important distributed-system failure modes.

Example:

```text
runner starts command
command performs external side effect
command succeeds
runner loses network
lease expires
```

The control plane now cannot know whether the task completed.

This is not equivalent to normal failure.

Recommended run/attempt semantics should allow:

```text
UNKNOWN
```

or:

```text
REQUIRES_RECONCILIATION
```

For non-idempotent workloads such as:

- payments;
- database migrations;
- deployment switches;
- destructive maintenance;
- industrial commands;

blind automatic retry may be unsafe.

---

# 7. Important limitation: exactly-once execution cannot be promised

Even with:

- PostgreSQL uniqueness;
- leases;
- fencing tokens;
- NATS deduplication;
- durable inboxes;
- SQLite;
- outbox patterns;

this situation remains possible:

```text
Control plane         Runner A

attempt 7 ----------> starts
                     performs side effect
                     network partition

lease expires

Control plane         Runner B

attempt 8 ----------> starts
```

The platform can potentially guarantee:

## Exactly one logical scheduled run

For example, through:

```text
UNIQUE(schedule_version_id, scheduled_for)
```

## At-least-once physical execution

Possible if ambiguous attempts are automatically retried.

## At-most-once physical execution

Possible only by refusing to retry some ambiguous attempts.

## Exactly-once external effect

This requires cooperation from the task or downstream system.

Typical approaches:

```text
idempotency_key = run.id
```

or a downstream fencing/transaction protocol.

This distinction should be part of Glyphflow's documented execution semantics.

---

# 8. Resource lease expiration needs a safer model

The current invariant:

```text
at most one unreleased lease per resource
```

has a subtle problem.

Suppose:

```text
lease A
expires_at = 12:00
released_at = NULL
```

At 12:05 the lease has expired but still appears unreleased.

A partial uniqueness constraint such as:

```sql
UNIQUE(resource_id)
WHERE released_at IS NULL
```

would still block acquisition.

Recommended model:

```text
RESOURCE_LEASES
---------------
id
resource_id
execution_attempt_id
state
lease_token
fencing_token
acquired_at
expires_at
released_at
```

States:

```text
ACTIVE
RELEASED
EXPIRED
```

Acquisition should be transactional:

```text
BEGIN

lock resource row

inspect current lease

if expired:
    mark EXPIRED

increment fencing counter

create new ACTIVE lease

COMMIT
```

Recommended addition:

```text
RESOURCES
---------
...
next_fencing_token bigint
```

This centralizes fencing-token generation.

---

# 9. Fencing tokens only work when enforced

A fencing token does not automatically stop an old process.

Example:

```text
Runner A -> token 100
network partition
lease expires

Runner B -> token 101
```

If the protected external system does not understand fencing tokens, Runner A can continue performing side effects.

Fencing is effective only when the target can enforce:

```text
accept token 101
reject token <= 100
```

Therefore distinguish between:

- internal resource coordination, where the database can enforce ownership;
- arbitrary external task side effects, where fencing may not be enforceable.

This limitation should be documented.

---

# 10. Event deduplication needs explicit behavior

The event inbox uniqueness rule is good:

```text
UNIQUE(execution_attempt_id, sequence_number)
```

But duplicate broker delivery is normal.

Example:

```text
1. event arrives
2. insert succeeds
3. state update succeeds
4. transaction commits
5. broker ACK is lost
6. event is delivered again
```

The second delivery should be treated as:

```text
already processed -> success -> ACK
```

not as an application error.

Make this an explicit invariant.

---

# 11. Event ordering needs state-aware protection

Sequence numbers alone do not prevent state rollback.

Example:

```text
sequence 8: COMPLETED
sequence 7: STARTED
```

If event 7 is applied after event 8, the attempt must not transition:

```text
SUCCEEDED -> RUNNING
```

Recommended fields:

```text
EXECUTION_ATTEMPTS
------------------
...
last_applied_state_sequence
state_version
```

Then apply state transitions only when valid.

One possible rule:

```text
apply only if sequence_number > last_applied_state_sequence
```

However, log chunks should not block state progress.

It is better to separate:

```text
state event sequence
log chunk sequence
```

so a missing stdout chunk cannot prevent a completion event from being applied.

---

# 12. State machines must be specified explicitly

Using `text state` in PostgreSQL is acceptable, but legal transitions need to be formally defined.

Suggested run states:

```text
WAITING
RUNNING
SUCCEEDED
FAILED
CANCELLING
CANCELLED
UNKNOWN
```

Suggested attempt states:

```text
CREATED
DISPATCH_PENDING
DISPATCHED
ACCEPTED
STARTED
SUCCEEDED
FAILED
TIMED_OUT
CANCELLED
RUNNER_LOST
UNKNOWN
```

Typical transition structure:

```text
CREATED
  |
  v
DISPATCHED
  |
  v
ACCEPTED
  |
  v
STARTED
  |-- SUCCEEDED
  |-- FAILED
  |-- TIMED_OUT
  |-- CANCELLED
  `-- UNKNOWN
```

State updates should be compare-and-swap style:

```sql
UPDATE execution_attempts
SET state = :new_state
WHERE id = :id
  AND state = :expected_old_state;
```

Do not let arbitrary incoming events overwrite the current state.

---

# 13. Cancellation is missing from the execution protocol

The authorization model already includes:

```text
runs.cancel
```

but the execution model does not define cancellation.

Recommended fields:

```text
RUNS
----
cancellation_requested_at
cancellation_requested_by
cancellation_reason
```

and:

```text
EXECUTION_ATTEMPTS
------------------
cancel_requested_at
cancel_acknowledged_at
```

The protocol must define races such as:

```text
controller sends cancel
attempt finishes successfully
cancel arrives late
```

The correct result should normally remain:

```text
SUCCEEDED
```

A stale cancellation for attempt 1 must never terminate attempt 2.

Therefore cancellation messages should include:

```text
execution_attempt_id
lease_token
generation/fencing token
```

---

# 14. Runner SQLite durability is part of correctness

Runner-local durability is only as strong as SQLite's configuration.

For critical work, make these settings part of the runner protocol:

```sql
PRAGMA journal_mode=WAL;
PRAGMA synchronous=FULL;
PRAGMA foreign_keys=ON;
```

The agent should verify them at startup.

Otherwise this failure can occur:

```text
runner commits ORDER_INBOX
runner ACKs message
power failure
latest SQLite commit is lost
```

That would violate the assumed order durability.

---

# 15. Runner restart creates ambiguous local execution states

SQLite does not make the external process execution atomic with the database.

Example:

```text
ORDER_INBOX.state = CLAIMED
COMMIT

start process
```

Crash windows include:

## Crash before process start

```text
CLAIMED persisted
process never started
```

## Crash after process start but before durable state update

```text
process started
runner crashes
SQLite still says CLAIMED
```

After restart, the runner cannot know whether the process ran.

Recommended rule:

```text
do not automatically rerun an order left in an ambiguous state
unless the task explicitly allows retry
```

`RUNNER_SESSIONS.boot_id` helps here.

For example:

```text
order owned by previous boot_id
    ->
UNKNOWN / RUNNER_LOST
    ->
controller applies ambiguity policy
```

For stricter workloads, use a controlled supervisor, process group, container, or another persistent execution identity.

---

# 16. Stale orders must be rejected

A durable broker may deliver an old order late.

Example:

```text
attempt A dispatched
lease expires
attempt B created
old order A arrives
```

Every signed order should include enough information to determine freshness.

Recommended envelope fields:

```text
protocol_version
message_id
message_type

execution_attempt_id
run_id
task_version_id

target_runner_id
target_runner_session_id

lease_token
fencing_token
lease_not_after

created_at
expires_at

execution_spec_digest
signing_key_id
```

A runner must reject an order when:

- it is expired;
- it targets another runner/session;
- its lease is stale;
- its fencing generation is obsolete;
- its signature is invalid;
- its execution spec digest does not match.

Broker delivery alone must never imply that an order is still valid.

---

# 17. Signed NATS messages need a formal protocol

"Signed orders and events" is a good requirement, but the signed data format must be precisely defined.

At minimum sign:

```text
protocol_version
message_id
message_type

issuer
recipient

created_at
expires_at

attempt_id
run_id

sequence_number

lease and fencing data

payload_digest
signing_key_id
```

Use deterministic canonical serialization.

Do not sign arbitrary JSON serialization whose byte representation can change due to field order or encoding differences.

Key rotation should be supported through:

```text
signing_key_id
```

---

# 18. Runner state should separate desired and observed state

A single:

```text
RUNNERS.state
```

will become overloaded.

Use:

```text
desired_state =
    ACTIVE
    DRAINING
    DISABLED
```

and:

```text
observed_state =
    ONLINE
    OFFLINE
    DEGRADED
```

These mean different things.

`DRAINING` should typically mean:

> finish current work, accept no new work.

This is very useful for maintenance and deployments.

---

# 19. Runner sessions need arbitration rules

Recommended constraints:

```text
UNIQUE(runner_id, boot_id)
```

Also explicitly decide whether multiple active sessions for one runner identity are allowed.

If they are not, enforce at most one active session.

Otherwise two processes with the same runner credentials could both act as the same logical runner.

That may be intentional in some systems, but it must never happen accidentally.

---

# 20. Runner enrollment certificate data must be clearly defined

The field:

```text
certificate_artifact
```

is ambiguous.

If it contains:

```text
public certificate
certificate chain
metadata
```

that is fine.

If it contains a private key, that is unsafe.

Private runner keys should ideally be generated and retained by the runner and never stored in the control plane.

Prefer a name such as:

```text
issued_certificate_chain
```

if that is what the field represents.

---

# 21. Self-blocking as a synthetic resource is a strong design

The current invariant:

```text
one system-managed exclusive resource per task
```

is clean.

Example:

```text
resource:
    system/task/<task-id>/self-lock
```

Every task version can require that resource.

This keeps self-serialization inside the same resource-leasing mechanism instead of introducing a special scheduler code path.

Keep this design.

However, resource serialization does not answer whether overlapping scheduled occurrences should:

```text
QUEUE
SKIP
REPLACE
```

so schedule concurrency policy is still required.

---

# 22. Resource requirements may eventually need quantity or lock mode

The current requirement table is binary:

```text
task_version_id
resource_id
```

That is sufficient if every resource means:

> exactly one attempt may hold it.

If future workloads need capacity semantics, consider:

```text
RESOURCES.capacity

TASK_RESOURCE_REQUIREMENTS.quantity
```

or:

```text
lock_mode = SHARED | EXCLUSIVE
```

This does not have to be added to the beta unless needed.

---

# 23. Immutable task versions are not fully reproducible when secrets rotate

The task version stores:

```text
secret_references
```

which is correct.

However:

```text
secret_reference = "prod/database-password"
```

may resolve to different secret versions over time.

Two runs using the same immutable task version may therefore execute with different credentials.

For forensic traceability, record the resolved secret version identifiers on the attempt:

```text
EXECUTION_ATTEMPTS
------------------
resolved_secret_versions jsonb
```

Example:

```json
{
  "prod/database-password": "version-42"
}
```

Never store the actual secret value.

---

# 24. Add an execution specification digest

Recommended task-version fields:

```text
execution_spec_version
execution_spec_digest
```

The signed order should include:

```text
task_version_id
execution_spec_digest
```

This allows the runner to verify exactly which immutable execution specification it is about to execute.

It also prepares the model for later use of:

```text
container image digest
artifact digest
script digest
```

---

# 25. Run events should expose critical fields outside JSON

A generic:

```text
RUN_EVENTS.payload jsonb
```

is useful for flexibility, but critical state information should not be hidden only inside arbitrary JSON.

Recommended:

```text
RUN_EVENTS
----------
event_id
event_kind
reported_at
accepted_at
payload
```

Possible kinds:

```text
ORDER_ACCEPTED
PROCESS_STARTED
PROCESS_EXITED
HEARTBEAT
CANCEL_ACK
EXECUTION_FAILED
```

`reported_at` is runner-provided and useful diagnostically.

`accepted_at` is the trusted control-plane timestamp.

---

# 26. Logging needs hard limits even during beta

Keeping log chunks in PostgreSQL/NATS may be reasonable for beta, but the platform needs limits immediately.

Recommended configuration:

```text
max_log_chunk_bytes
max_logs_per_attempt
max_total_log_bytes
log_retention_days
```

Without limits, a task that produces excessive stdout can overload:

```text
runner SQLite
NATS
PostgreSQL
replication
backups
```

Long term, move bulk log bodies to blob/object storage and keep metadata in PostgreSQL.

---

# 27. Authentication settings singleton should be database-enforced

If `AUTH_SETTINGS` is intended to contain exactly one row, do not rely only on a comment.

Use a database constraint such as:

```sql
CHECK (id = 1)
```

---

# 28. Last-administrator protection must be concurrency-safe

The invariant:

```text
prevent deletion of the last active administrator
```

is correct.

But avoid logic like:

```text
SELECT count(admins)

if count > 1:
    DELETE
```

Two concurrent requests can both observe two administrators and delete both.

The operation needs locking, serializable behavior, or another database-enforced coordination mechanism.

---

# 29. Refresh-token rotation can eventually use token families

The current session model with one stored refresh-token hash is workable.

If stronger token-reuse detection is desired later, consider explicit token generations or token families.

This is not a scheduler blocker, but it improves authentication incident handling.

---

# 30. Audit actor modeling may eventually need more than users

The current audit design is already strong.

However, meaningful actors will eventually include:

```text
USER
SYSTEM
RUNNER
SERVICE_ACCOUNT
SCHEDULER
```

A future-proof model may use:

```text
actor_type
actor_id
actor_user_id nullable
actor_session_id nullable
```

instead of assuming every explicit actor is a user.

---

# 31. Database constraints worth enforcing

The invariant list is good. Recommended database-level constraints include:

```text
UNIQUE(task_id, version)
UNIQUE(schedule_id, version)
UNIQUE(schedule_version_id, scheduled_for)
UNIQUE(run_id, attempt_number)
UNIQUE(execution_attempt_id, sequence_number)
UNIQUE(order_id, sequence_number)
UNIQUE(runner_id, boot_id)
```

Potential checks:

```text
expires_at > acquired_at
finished_at >= started_at
completed_at >= queued_at
capacity >= 0
attempt_number > 0
sequence_number >= 0
```

Important foreign-key invariants:

```text
schedule_version.task_version must belong to schedule.task
run.task_version must belong to the same task as schedule_version
runner_session.runner_id must equal execution_attempt.runner_id
```

These should be database-enforced where practical rather than only application-enforced.

---

# 32. Recommended control-plane scheduling flow

A robust flow would be:

## Step 1: find due schedules

Multiple scheduler replicas select due schedule rows using short database transactions and row locking.

## Step 2: create the logical occurrence

Within the same transaction:

```text
read current schedule version
calculate scheduled_for
apply misfire policy
apply deadline policy
insert RUN
advance next_fire_at
```

The scheduled-occurrence uniqueness constraint prevents duplicate logical runs.

## Step 3: commit quickly

Do not keep database locks or transactions open while a task executes.

## Step 4: dispatch

When resources and runner capacity are available:

```text
create EXECUTION_ATTEMPT
acquire resource leases
create DISPATCH_OUTBOX
```

all in one transaction.

## Step 5: publish

The outbox producer publishes the signed order.

## Step 6: runner durability

The runner persists the order to SQLite before acknowledging the broker.

## Step 7: execute

The runner starts the process and produces durable events through its SQLite outbox.

## Step 8: receive events

The control plane:

```text
persists event inbox row
applies legal state transition
updates attempt/run
commits
ACKs broker
```

## Step 9: resolve failures

If the runner disappears:

```text
lease expires
attempt becomes lost/unknown
retry or escalation follows task policy
```

## Step 10: cleanup

Logs, inbox/outbox rows, telemetry, and completed execution data are cleaned asynchronously according to retention policy.

---

# 33. Failure scenarios the design must explicitly answer

Before calling the scheduler production-ready, document the expected behavior for every case below.

| Failure scenario | Required defined behavior |
|---|---|
| Two scheduler replicas discover the same due schedule | Exactly one logical run is created |
| Scheduler crashes before committing run creation | Another scheduler may safely create it |
| Scheduler crashes immediately after committing run creation | Run remains durable and dispatchable |
| Dispatcher crashes after creating attempt/outbox but before publish | Outbox publisher eventually sends it |
| Broker delivers an order twice | Runner deduplicates it |
| Runner commits order then ACK is lost | Duplicate redelivery is treated as already accepted |
| Runner dies before process starts | Attempt can be safely reclassified/retried |
| Runner dies after process starts | Result may be ambiguous |
| Runner finishes but loses network before reporting | Ambiguous result policy applies |
| Old order arrives after lease expiry | Runner rejects it |
| Old cancel arrives after retry begins | It must not cancel the new attempt |
| Network partition leaves old runner executing | Fencing/idempotency limitations are understood |
| Scheduler down for several hours | Misfire/catch-up policy defines all missed occurrences |
| DST jumps forward | Explicit schedule behavior |
| DST repeats an hour | Explicit schedule behavior |
| Task edited while an old run is queued | Old run executes its pinned immutable task version |
| Schedule edited while old occurrence is queued | Old run keeps old schedule-version identity |
| Resource holder disappears | Lease expires and takeover is deterministic |
| NATS is unavailable | Outbox retains work |
| PostgreSQL is unavailable | Runner/control-plane behavior is predetermined |
| Runner SQLite is unavailable/corrupt | Safe fail-stop behavior is defined |
| Log persistence fails | Task outcome is not accidentally corrupted unless policy says so |
| User disables a task while one run is active | Future scheduling and current execution are handled separately |
| User requests cancellation as the process exits | Race resolves deterministically |
| Secret rotates between runs | Attempt records which secret version was resolved |

---

# 34. Recommended revised core entities

A mature version of the central model would approximately contain:

```text
TASKS
TASK_VERSIONS

SCHEDULES
SCHEDULE_VERSIONS

RUNS
EXECUTION_ATTEMPTS

RUNNER_POOLS
RUNNER_POOL_MEMBERS
RUNNERS
RUNNER_SESSIONS
RUNNER_KEYS
RUNNER_ENROLLMENTS

RESOURCES
TASK_RESOURCE_REQUIREMENTS
RESOURCE_LEASES

DISPATCH_OUTBOX
EVENT_INBOX
RUN_EVENTS
EXECUTION_LOG_CHUNKS

USERS
USER_PASSWORDS
USER_SESSIONS
SSO_PROVIDERS
USER_SSO_IDENTITIES
ROLES
PERMISSIONS
ROLE_ASSIGNMENTS

AUDIT_EVENTS
```

Potential later additions:

```text
RETRY_POLICIES
NOTIFIERS
NOTIFICATION_POLICIES
NOTIFICATION_DELIVERIES
APPROVALS
RUNNER_TELEMETRY
OBJECT_STORAGE_LOG_REFERENCES
SERVICE_ACCOUNTS
```

---

# 35. Recommended ERD changes before implementation

The highest-priority schema changes are:

## P0: correctness blockers

1. Add `TASKS.current_version_id`.
2. Add `SCHEDULES.current_version_id`.
3. Replace default static `TASK_VERSIONS.runner_id` with runner-pool or selector semantics.
4. Define formal `RUNS` and `EXECUTION_ATTEMPTS` state machines.
5. Add schedule misfire/catch-up/start-deadline/concurrency policies.
6. Add retry policy and ambiguous-result policy.
7. Fix resource lease expiry/takeover semantics.
8. Define deterministic fencing-token generation.
9. Add cancellation state and cancellation protocol.
10. Define runner recovery behavior for ambiguous local `CLAIMED/RUNNING` orders.

## P1: protocol hardening

11. Define signed message envelope and canonical encoding.
12. Include message expiry and target runner/session in orders.
13. Reject stale orders at the runner.
14. Define duplicate event handling as successful idempotent processing.
15. Separate state-event ordering from log-chunk ordering.
16. Add attempt state sequencing/version checks.
17. Verify SQLite durability settings at runner startup.
18. Enforce runner session uniqueness/arbitration.
19. Separate desired vs observed runner state.

## P2: auditability and operations

20. Record resolved secret version identifiers on attempts.
21. Add execution specification digests.
22. Pull critical run-event fields out of generic JSON.
23. Add hard log size and retention limits.
24. Clarify certificate artifact contents.
25. Make auth singleton constraints database-enforced.
26. Make last-admin protection concurrency-safe.
27. Expand audit actor model when service accounts/system actors are introduced.

---

# 36. What I would keep unchanged

Several design decisions are good enough that I would preserve them:

- PostgreSQL as the control-plane source of truth.
- SQLite as runner-local durable state.
- NATS JetStream as the transport.
- transactional dispatch outbox.
- event inbox deduplication.
- immutable task versions.
- immutable schedule versions.
- `RUNS` vs `EXECUTION_ATTEMPTS`.
- explicit `scheduled_for`.
- runner sessions with boot identity.
- resource leases.
- self-blocking represented as a synthetic resource.
- secret references instead of secret values.
- separate authentication identities and role assignments.
- seed-owned permission keys.
- detailed audit events.

---

# 37. Final assessment

The original model needed a fundamental execution-model redesign.

The current beta no longer does.

The core structure is now sound:

```text
definition
    ->
immutable version
    ->
scheduled logical occurrence
    ->
physical attempt
    ->
durable dispatch
    ->
runner-local durable order
    ->
execution
    ->
durable event
    ->
control-plane state transition
```

The remaining work is concentrated in distributed-systems semantics rather than broad schema architecture.

The design should not yet be marketed internally or externally as "critical-task safe" until the following are fully specified and tested:

```text
version activation
dynamic runner assignment
misfire handling
start deadlines
concurrency rules
retry rules
ambiguous-result handling
lease takeover
fencing limitations
event ordering
duplicate handling
runner restart recovery
cancellation races
signed protocol details
SQLite durability
```

Once those are defined, enforced by database constraints where possible, and covered by failure-injection tests, the architecture becomes a credible basis for a critical scheduler platform.

---

# 38. Recommended next engineering artifact

The next document should not be another ER diagram.

It should be a formal execution protocol containing:

```text
Run state machine
ExecutionAttempt state machine
Dispatch protocol
Lease acquisition and renewal protocol
Resource takeover protocol
Runner restart recovery protocol
Cancellation protocol
Retry and ambiguity policy
Event ordering and deduplication rules
Signed message envelope
Scheduler failover behavior
```

That document will determine the actual reliability of Glyphflow more than any additional database table.
