# TODO

Planned from [REPORT.md](./REPORT.md) and all detailed analysis artifacts. No work has started. The duplicate cron findings CODE-002 and OPT-002 are one consolidated item.

## Features

- [x] Define runtime supervision and readiness
  - Importance Level: High
  - Description: Added explicit health state for session cleanup, heartbeat, dispatcher, start-claim, and scheduler loops; readiness now fails until each component starts or after a reported failure. The control plane remains a single process. Evidence: [ARCHITECTURE.md#arc-001](./ARCHITECTURE.md#arc-001).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/controlplane ./cmd/controlplane`; `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go vet ./internal/controlplane ./cmd/controlplane`; `git diff --check`
  - Test Result: PASS — health recovery tests, control-plane tests, vet, and diff checks passed.
  - Commit Hash: `cf3770a68ef7f78835f596e30d25ede8004734cb`

- [x] Require durable repositories in production
  - Importance Level: High
  - Description: Production composition now enables a durable-repository guard covering operations, runs, infrastructure, global variables, audit, and exit-code persistence; non-production services retain explicit in-memory defaults. Evidence: [ARCHITECTURE.md#arc-002](./ARCHITECTURE.md#arc-002).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/api ./cmd/controlplane`; `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go vet ./internal/api ./cmd/controlplane`; `git diff --check`
  - Test Result: PASS — repository validation regression, API/control-plane tests, vet, and diff checks passed.
  - Commit Hash: `5aa1a7932f2a0a59a94b2d74f413574d0eedfa8c`

- [x] Add route and JSON contract checks
  - Importance Level: Medium
  - Description: Added representative Go route registration, permission/status, and task JSON checks plus a TypeScript client request/response contract test. Evidence: [ARCHITECTURE.md#arc-004](./ARCHITECTURE.md#arc-004).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/api -run 'TestRepresentative|TestTaskJSON' -count=1`; `npm test -- --run src/api.contract.test.ts`; `npm run typecheck`; `git diff --check`
  - Test Result: PASS — Go contract tests, the frontend contract test, typecheck, and diff checks passed.
  - Commit Hash: `256a3b3db0942681f177ed8fcd112610e59af394`

- [x] Consolidate schedule semantics
  - Importance Level: Medium
  - Description: Scheduler calculation and durable schedule validation now reuse one canonical timezone parser for fixed offsets and IANA zones. Evidence: [ARCHITECTURE.md#arc-005](./ARCHITECTURE.md#arc-005).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/platform ./internal/controlplane ./internal/store`; `git diff --check`
  - Test Result: PASS — shared timezone semantics and scheduler/store package tests passed.
  - Commit Hash: `a379683459262df3002d5ea809fc13a7e5397ba9`

- [ ] Narrow the NATS orchestration seam
  - Importance Level: Medium
  - Description: Keep concrete JetStream types inside the queue adapter or a narrow integration seam and use existing queue ports where substitution is needed. Evidence: [ARCHITECTURE.md#arc-006](./ARCHITECTURE.md#arc-006).
  - Test Description: Add a real NATS integration path for consumer setup, acknowledgement, retry, and delivery behavior before replacing production signatures.
  - Test Result: Not run
  - Commit Hash: Not committed

- [x] Publish current architecture documentation
  - Importance Level: Medium
  - Description: Added a repository-root architecture source of truth for current topology, ownership, health/readiness, deployment, and verification; README now links to it and dated reviews are identified as historical. Evidence: [ARCHITECTURE.md#arc-007](./ARCHITECTURE.md#arc-007).
  - Test Description: `git diff --check`; review `ARCHITECTURE.md` links against current source paths.
  - Test Result: PASS — architecture document links and working-tree diff checks passed.
  - Commit Hash: `1d3c4af3db4c584e54bbbf8893185b0f7373b961`

- [ ] Reduce cross-layer dispatch coupling incrementally
  - Importance Level: Medium
  - Description: Keep new domain decisions in application services and durable reads/writes in repositories without introducing a generic framework or broad rewrite. Evidence: [ARCHITECTURE.md#arc-003](./ARCHITECTURE.md#arc-003).
  - Test Description: Add characterization tests around existing SQL transaction boundaries and verify one migrated policy at a time.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Cover control-plane startup lifecycle
  - Importance Level: Medium
  - Description: Add focused coverage for configuration precedence, migration/NATS failures, readiness, signal shutdown, and background-loop startup in the high-complexity entry point. Evidence: [CODE.md#code-005](./CODE.md#code-005).
  - Test Description: Run targeted startup tests under `go test -race` and assert cleanup and goroutine termination for each failure path.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Cover worker lifecycle boundaries
  - Importance Level: Medium
  - Description: Add narrow tests for persisted state, enrollment failure, consumer setup, cancellation, cleanup, and status reporting in `runWorker`. Evidence: [CODE.md#code-006](./CODE.md#code-006).
  - Test Description: Run lifecycle tests under `go test -race` and verify all background goroutines terminate after cancellation.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Measure run-list placement cost
  - Importance Level: Medium
  - Description: Benchmark the unbounded run list and per-waiting-run `placementBlocker` query before considering pagination or a set-based query. Evidence: [OPTIMIZATION.md#opt-001](./OPTIMIZATION.md#opt-001).
  - Test Description: Use 100, 1,000, and 10,000 runs with varying waiting-run counts; record query count, p50/p95 latency, rows scanned, heap, and `EXPLAIN (ANALYZE, BUFFERS)`.
  - Test Result: Not run
  - Commit Hash: Not committed

- [x] Measure and optimize cron parsing
  - Importance Level: Medium
  - Description: `nextCronMinute` now parses each cron field once per calculation, preserving existing matching semantics. Evidence: [CODE.md#code-002](./CODE.md#code-002), [OPTIMIZATION.md#opt-002](./OPTIMIZATION.md#opt-002).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/controlplane`; `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/controlplane -run '^$' -bench '^BenchmarkNextFireCron$' -benchmem -count=1`; `git diff --check`
  - Test Result: PASS — control-plane tests and benchmarks passed; dense 4,553 ns/op, sparse 18,361 ns/op, unsatisfiable 20,046,034 ns/op.
  - Commit Hash: `9e3683b4de08245cb567bd9a2e59d56ce77662f0`

- [ ] Measure worker log-stream contention
  - Importance Level: Medium
  - Description: Determine whether synchronous outbox publishing under `pendingEventsMu` and single-connection SQLite materially backpressures concurrent commands before changing locking or drain behavior. Evidence: [OPTIMIZATION.md#opt-003](./OPTIMIZATION.md#opt-003).
  - Test Description: Run 1, 4, and 16 concurrent output-heavy orders with controlled broker latency; record p95 completion, chunk throughput, mutex wait, SQLite latency, outbox depth, retries, and duplicates.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Measure audit query scalability
  - Importance Level: Medium
  - Description: Profile count-plus-page queries, leading-wildcard filters, JSON decoding, and the unbounded `all=true` response before changing SQL or API limits. Evidence: [OPTIMIZATION.md#opt-004](./OPTIMIZATION.md#opt-004).
  - Test Description: Run representative unfiltered, date, actor, and text-filtered queries with `EXPLAIN (ANALYZE, BUFFERS)` and record page/count latency, buffer reads, decode time, and response size.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Measure dispatch backlog behavior
  - Importance Level: Medium
  - Description: Measure sequential NATS publish plus database update cost and the pending-dispatch index/order mismatch before introducing batching, concurrency, or an index change. Evidence: [OPTIMIZATION.md#opt-006](./OPTIMIZATION.md#opt-006).
  - Test Description: Measure drain rate, publish/update latency, retries, backlog depth, and `EXPLAIN (ANALYZE, BUFFERS)` while preserving durable at-least-once delivery and required ordering.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Align the Vite and Vitest toolchain
  - Importance Level: Medium
  - Description: Remove the direct Vite 5/Vitest 4 peer mismatch and avoid the nested second Vite major; regenerate only the frontend lockfile after selecting compatible versions. Evidence: [DEPENDENCIES.md#dep-001](./DEPENDENCIES.md#dep-001).
  - Test Description: In a clean environment run `npm ci`, `npm ls --all`, `npm run typecheck`, `npm run build`, and `npm test`.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Validate a clean npm installation
  - Importance Level: Medium
  - Description: Confirm whether the reported invalid local `esbuild` peer state reproduces from the committed lockfile rather than stale `node_modules`. Evidence: [DEPENDENCIES.md](./DEPENDENCIES.md#high-priority-actions).
  - Test Description: Run `npm ci` in a clean workspace, then run `npm ls --all` and the frontend build/test commands.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Add a supported JavaScript runtime policy
  - Importance Level: Low
  - Description: Document and enforce the Node.js version required by React Router, Vitest, and jsdom instead of documenting only “Node.js.” Evidence: [DEPENDENCIES.md](./DEPENDENCIES.md#high-priority-actions).
  - Test Description: Verify the declared runtime against the build image and run a clean install, typecheck, test, and build on that runtime.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Measure global-variable schedule overhead
  - Importance Level: Low
  - Description: Measure repeated global-variable loading and snapshot resolution per due schedule before considering reuse or caching. Evidence: [OPTIMIZATION.md#opt-005](./OPTIMIZATION.md#opt-005).
  - Test Description: Compare scheduler throughput, transaction duration, row reads, and allocations with 1, 100, and 1,000 global variables across multiple due schedules while preserving freshness semantics.
  - Test Result: Not run
  - Commit Hash: Not committed

## Security Patches

- [x] Harden non-local Compose deployments
  - Importance Level: High
  - Description: Added a production Compose overlay with required secrets, TLS-authenticated PostgreSQL/NATS, private infrastructure ports, loopback-only web binding, and production PostgreSQL TLS validation. Evidence: [SECURITY.md#sec-001](./SECURITY.md#sec-001).
  - Test Description: `env <required production values> docker compose -f compose.yaml -f compose.production.yaml config --quiet`; `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/config`; `git diff --check`
  - Test Result: PASS — production Compose rendered with explicit test secret paths; infrastructure ports were removed, config TLS tests passed, and diff checks passed. Live isolated PostgreSQL/NATS authentication was not run because no deployment network was available.
  - Commit Hash: `39b7b11d618b46b40a2970c8d8ac6ad4746fa792`

- [x] Reject wildcard credentialed CORS
  - Importance Level: Medium
  - Description: Wildcard origins are ignored; exact trusted origins retain credentialed CORS headers, and untrusted state-changing requests remain subject to CSRF. Evidence: [SECURITY.md#sec-002](./SECURITY.md#sec-002).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/api -run 'TestCORS|TestCSRF' -count=1`
  - Test Result: PASS — focused CORS and CSRF tests passed.
  - Commit Hash: `21b8894ae7918850f0c3b29d1a1e3f957aca1e43`

## Bug Fixes

- [x] Bound pagination arithmetic
  - Importance Level: Medium
  - Description: Pagination offsets now avoid integer overflow and return bounded pages for extreme values. Evidence: [CODE.md#code-001](./CODE.md#code-001).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/api -run '^TestTaskCollectionBoundsPaginationPage$' -count=1`
  - Test Result: PASS — in-memory and repository-backed maximum, negative, malformed, and very large page cases passed.
  - Commit Hash: `3837b53f073a24f2e11bcd09976d449b50fb10d5`

- [x] Handle corrupt worker boot metadata
  - Importance Level: Medium
  - Description: Worker startup now distinguishes missing, unreadable, malformed, and empty boot metadata and fails explicitly when environment updates fail. Evidence: [CODE.md#code-003](./CODE.md#code-003).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./cmd/worker -run 'Test(LoadPreviousBootID|SetWorkerEnvReportsFailure)$' -count=1`; `git diff --check`
  - Test Result: PASS — focused metadata and environment failure tests passed; diff check passed.
  - Commit Hash: `a347de5d10ee46e37d4243bcc7fde5231f5a696c`

- [x] Close control-plane resources on startup errors
  - Importance Level: Low
  - Description: Control-plane startup now returns errors through one deferred cleanup path; database cleanup begins immediately after pool creation and JetStream closes on termination. Evidence: [CODE.md#code-004](./CODE.md#code-004).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./cmd/controlplane`; `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go vet ./cmd/controlplane`; `git diff --check`
  - Test Result: PASS — control-plane tests, vet, and diff checks passed.
  - Commit Hash: `369cb176c4e642b2a65714294cf6ab9bcb4823d3`
