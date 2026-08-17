# Glyphflow Backend Audit

Audit date: 2026-08-13

## Result

The repository contains useful backend primitives, but it does not contain a working orchestration backend.

The control plane serves one stub task route. The worker validates configuration and exits. Neither command connects the protocol, store, queue, scheduler, or worker packages.

The checked roadmap does not match the implementation. It marks 245 items complete. It credits 129 items to one security and release commit. The security script checks only files, names, and one source-code search.

Do not deploy this backend or use `internal/TODO.md` as production-readiness evidence.

## Scope and method

The audit covered:

- All 12 files under `internal/`, including the 403-line roadmap and the SPDX document.
- All Go commands, packages, tests, migrations, and dependencies under `backend/`.
- `README.md`, `compose.yaml`, and both repository check scripts.
- Production entry points and every caller of security, state, queue, worker, and protocol primitives.

The repository has no TokenSave graph or database. The audit used direct source tracing and repository-native checks.

## Verification results

| Check | Result | Evidence |
|---|---|---|
| `env GOCACHE=/tmp/glyphflow-go-cache-race go test -race -cover ./...` | Pass | Package coverage ranges from 37.8% to 78.3%. Command packages have no tests. |
| `env GOCACHE=/tmp/glyphflow-go-cache-vet go vet ./...` | Pass | No vet finding. |
| `go mod verify` | Pass | All downloaded modules match their checksums. |
| `scripts/security-check.sh` | Pass, but not a security test | It checks three files, one forbidden string search, and three symbol names. |
| `scripts/release-check.sh` | Pass, but not a release test | It runs Go tests and checks that two files exist. |
| `govulncheck ./...` | Fail | GO-2026-5004 reaches `pgx/v5` through `store.ApplyMigrations`. |

No test starts PostgreSQL or NATS. Store tests inspect SQL text. Queue tests cover only an in-memory queue and a subject string.

## Available features

### Configuration validation

**Impact / Severity Level:** Low

**Description:** The configuration package validates roles, URL schemes, absolute data paths, runner identifiers, and positive size limits.

**Evidence:** `backend/internal/config/config.go:29-78` and two unit tests.

**Suggested Modification:** Keep this package. Add deployment-mode TLS rules when the queue connection is implemented.

### Signed protocol primitives

**Impact / Severity Level:** Medium

**Description:** The protocol package provides Ed25519 envelopes, separate order and event domains, versions, key validity, time checks, identity checks, and sequence checks.

**Evidence:** `backend/internal/protocol/envelope.go`, `signature.go`, `keyring.go`, and `validation.go`.

**Suggested Modification:** Reuse these primitives in one complete verification function. Do not duplicate validation in callers.

### PostgreSQL schema primitives

**Impact / Severity Level:** Medium

**Description:** Sixteen migrations define tasks, runs, events, runners, keys, enrollments, leases, an inbox, and an outbox.

**Evidence:** `backend/migrations/001_schema_migrations.sql` through `016_retention_indexes.sql`.

**Suggested Modification:** Keep the schema as a starting point. Add real PostgreSQL integration tests before relying on its transaction rules.

### Transaction and state helpers

**Impact / Severity Level:** Medium

**Description:** `CreateTaskRun` writes a run, leases, and an outbox row in one transaction. `UpdateTaskRunState` uses an optimistic version.

**Evidence:** `backend/internal/store/runs.go:47-68` and `state.go:9-19`.

**Suggested Modification:** Wire these helpers into the control plane after state-transition enforcement is added.

### Limited scheduler and API helpers

**Impact / Severity Level:** Low

**Description:** The scheduler supports manual times, fixed times, and a small cron subset. The API exposes health and stub task routes.

**Evidence:** `backend/internal/controlplane/scheduler.go` and `backend/internal/api/api.go:21-43`.

**Suggested Modification:** Label both as prototypes until the findings below are fixed.

## Potential new features

### N1. Connect one end-to-end task path

**Impact / Severity Level:** Critical

**Description:** The control plane imports only configuration and API packages. The worker imports only configuration. No task can reach a worker.

**Evidence:** `backend/cmd/controlplane/main.go:11-30` and `backend/cmd/worker/main.go:7-15`.

**Suggested Modification:** Implement one path: create run, write outbox, publish order, verify and store order, execute, publish event, verify event, and update state. Reuse existing packages. Delete unused primitives that do not serve this path.

### N2. Implement durable JetStream delivery

**Impact / Severity Level:** High

**Description:** The NATS adapter creates one stream. It has no publisher, pull consumer, acknowledgement flow, dead-letter flow, or outbox dispatcher.

**Evidence:** `backend/internal/queue/nats.go:11-31`. The adapter has no production caller.

**Suggested Modification:** Add only the publisher and consumers required by N1. Acknowledge orders after local commit and events after central commit.

### N3. Implement the management API and real authentication

**Impact / Severity Level:** High

**Description:** The API has no task persistence, runs, events, runners, enrollment, cancellation, retry, keys, or audit query endpoints.

**Evidence:** `backend/internal/api/api.go` registers only health and `/api/v1/tasks`.

**Suggested Modification:** Add endpoints only as their backing workflow becomes real. Use one configured authenticator and explicit permission per method.

### N4. Implement enrollment and key lifecycle

**Impact / Severity Level:** High

**Description:** Token and key primitives exist, but no endpoint claims tokens, issues certificates, enrolls workers, rotates keys, or revokes queue access.

**Evidence:** No production caller uses `platform.NewEnrollmentToken` or `protocol.Keyring`.

**Suggested Modification:** Implement enrollment after mTLS queue identity exists. Keep token claim and runner activation in PostgreSQL transactions.

### N5. Add operational state and telemetry

**Impact / Severity Level:** Medium

**Description:** Logs and audits live only in memory. Health always reports `ok`. No check measures database, queue, outbox, worker, or lease state.

**Evidence:** `backend/internal/platform/observability.go` and `backend/internal/api/api.go:23`.

**Suggested Modification:** Add readiness checks and the smallest metrics needed for the end-to-end path. Persist security audits before adding more metrics.

## Potential bugs and edge cases

### B1. The executor ignores the validated working directory

**Impact / Severity Level:** High

**Description:** `Executor.Run` validates `dir`, then starts the command without setting `cmd.Dir`. Commands run in the worker process directory.

**Evidence:** `backend/internal/worker/worker.go:15-30`.

**Suggested Modification:** Build the command, set `cmd.Dir = clean`, then run it. Add one test that prints its working directory.

### B2. The worker store can lose deduplication state

**Impact / Severity Level:** High

**Description:** The store rewrites one JSON file in place. A crash can truncate it. A failed write leaves the new ID in memory, so the next call reports success without durable data.

**Evidence:** `backend/internal/worker/store.go:31-49`.

**Suggested Modification:** Do not build the worker on this store. Replace it with the planned SQLite transaction when N1 is implemented. Delete the JSON store afterward.

### B3. State updates do not enforce the state machine

**Impact / Severity Level:** High

**Description:** `UpdateTaskRunState` accepts any database-allowed state. It never calls `TransitionAllowed`. A completed run can return to running.

**Evidence:** `backend/internal/store/state.go:9-19`. `platform.TransitionAllowed` has only a unit-test caller.

**Suggested Modification:** Pass expected source and target states to one update helper. Reject invalid transitions before one compare-and-swap query.

### B4. Cron schedules can run at the wrong time

**Impact / Severity Level:** High

**Description:** The parser supports only `*`, exact numbers, and comma lists. It silently treats ranges and steps as nonmatches. It also requires both day fields to match, unlike common cron behavior.

**Evidence:** `backend/internal/controlplane/scheduler.go:40-110`.

**Suggested Modification:** Define the supported cron grammar. Reject unsupported syntax at save time. Use one tested cron implementation only if the required grammar exceeds this small parser.

### B5. Task creation accepts no task

**Impact / Severity Level:** Medium

**Description:** POST `/api/v1/tasks` does not read, decode, validate, or store the request. It returns `202` for an empty or chunked body.

**Evidence:** `backend/internal/api/api.go:24-41`.

**Suggested Modification:** Return `501` until task creation exists. When added, use `http.MaxBytesReader`, strict JSON decoding, validation, and one database transaction.

### B6. Concurrent replicas can race migrations

**Impact / Severity Level:** Medium

**Description:** Each replica checks a migration before it opens the migration transaction. Two replicas can apply the same migration and make one startup fail.

**Evidence:** `backend/internal/store/migrations.go:66-86`.

**Suggested Modification:** Acquire one PostgreSQL advisory transaction lock before the applied check and migration execution.

### B7. SQLite import loses command arguments

**Impact / Severity Level:** Medium

**Description:** The importer places the full legacy command string into one argument. A command with arguments becomes an executable name that does not exist.

**Evidence:** `backend/internal/store/import_sqlite.go:47-56`.

**Suggested Modification:** Import a documented structured argument format. Reject ambiguous legacy command strings instead of guessing shell syntax.

### B8. Component shutdown and liveness are incomplete

**Impact / Severity Level:** Low

**Description:** `Plane.Stop` does nothing. A component that returns nil does not stop sibling components. The executable does not use `Plane`.

**Evidence:** `backend/internal/controlplane/controlplane.go:12-41`.

**Suggested Modification:** Delete `Stop`. Make `Run` own cancellation and treat unexpected component exit as an error when N1 needs concurrent loops.

## Security risks and patches

### S1. The production entry point authenticates every request

**Impact / Severity Level:** High

**Description:** The control plane installs an authenticator that always succeeds. It grants `task.read`, which also authorizes POST.

**Evidence:** `backend/cmd/controlplane/main.go:22-24` and `backend/internal/api/api.go:24`.

**Suggested Modification:** Remove the fallback authenticator. Refuse startup without configured authentication. Require `task.create` for POST.

### S2. Queue connections do not use mutual TLS or identity permissions

**Impact / Severity Level:** High

**Description:** `nats.Connect(url)` receives no TLS, client certificate, trust bundle, or credential options. The stream accepts wildcard worker subjects.

**Evidence:** `backend/internal/queue/nats.go:16-27`. Configuration accepts plaintext `nats://` in `backend/internal/config/config.go:53`.

**Suggested Modification:** Require trusted TLS settings outside local development. Bind each certificate identity to exact publish and subscribe subjects. Add denial tests with real NATS.

### S3. Worker process controls are absent

**Impact / Severity Level:** High

**Description:** The executor has no output bound, process-group kill, resource ceiling, service identity change, executable allowlist, or secret resolver. `CombinedOutput` can exhaust memory.

**Evidence:** `backend/internal/worker/worker.go:30`.

**Suggested Modification:** Add controls before connecting the executor to orders. Stream into a bounded writer, stop the process group on timeout, and enforce platform limits.

### S4. No production path performs complete message verification

**Impact / Severity Level:** High

**Description:** Payload decoders accept an execute order with only version and type. Signature, key, time, identity, sequence, replay, and error checks remain separate and unused.

**Evidence:** `backend/internal/protocol/payload.go:56-82`, `validation.go`, and the absence of production callers.

**Suggested Modification:** Add one order verifier and one event verifier. Each function must return a typed payload only after all required checks pass.

### S5. Path checks allow symbolic-link escape

**Impact / Severity Level:** Medium

**Description:** Both path checks compare lexical absolute paths. A path below an allowed root can resolve through a symbolic link to another tree.

**Evidence:** `backend/internal/worker/worker.go:15-25` and `backend/internal/platform/security.go:49-55`.

**Suggested Modification:** Keep one shared path check. Resolve existing roots and working directories with `filepath.EvalSymlinks` before containment checks.

### S6. The backend uses a reachable vulnerable pgx version

**Impact / Severity Level:** Medium

**Description:** `govulncheck` reports GO-2026-5004 in `pgx/v5@v5.7.6`. The fixed version is `v5.9.2`.

**Evidence:** The trace reaches `store.ApplyMigrations` at `backend/internal/store/migrations.go:68`.

**Suggested Modification:** Upgrade `github.com/jackc/pgx/v5` to at least `v5.9.2`. Run tests, vet, and `govulncheck` again.

### S7. HTTP limits and shutdown do not resist slow clients

**Impact / Severity Level:** Medium

**Description:** The HTTP server has no read-header, read, write, or idle timeout. Shutdown uses an unlimited background context.

**Evidence:** `backend/cmd/controlplane/main.go:22-25`.

**Suggested Modification:** Set standard server timeouts. Use a bounded shutdown context. Terminate TLS at a documented trusted proxy or in the server.

### S8. Security and release checks create false assurance

**Impact / Severity Level:** High

**Description:** The security script does not test authentication, mTLS, permissions, execution limits, redaction, revocation, or dependencies. The SPDX file has an empty package list.

**Evidence:** `scripts/security-check.sh:4-9`, `scripts/release-check.sh`, and `internal/SBOM.spdx.json`.

**Suggested Modification:** Uncheck unsupported roadmap claims. Replace presence checks with behavior tests. Generate the SBOM from built artifacts and fail on an empty package list.

## Documentation conflicts

- `README.md` correctly says the project is in the design phase.
- `internal/TODO.md` marks nearly all backend, security, recovery, operations, and release work complete.
- `internal/SECURITY.md` states controls that the runtime does not enforce.
- `internal/OPERATIONS.md` calls unit tests and Compose validation production checks.
- `internal/SBOM.spdx.json` lists no packages despite direct Go dependencies.

Treat the README status as authoritative until behavior tests prove each roadmap item.

## Suggested delivery order

1. Fix the roadmap and remove false production-readiness claims.
2. Fix authentication, pgx, executor directory handling, and state transitions.
3. Implement one secure end-to-end task path.
4. Add PostgreSQL and NATS integration tests for its durable handoffs.
5. Add remaining API, enrollment, and operations features only after that path passes.
