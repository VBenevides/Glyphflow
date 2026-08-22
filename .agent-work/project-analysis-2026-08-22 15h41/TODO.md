# TODO

Planned from [REPORT.md](./REPORT.md) and all detailed analysis artifacts. No work has started. The duplicate cron findings CODE-002 and OPT-002 are one consolidated item.

## Features

- [ ] Define runtime supervision and readiness
  - Importance Level: High
  - Description: Track scheduler, dispatcher, heartbeat, start-claim, and housekeeping liveness separately from PostgreSQL/NATS connectivity; define singleton and replica-safe loop ownership. Evidence: [ARCHITECTURE.md#arc-001](./ARCHITECTURE.md#arc-001).
  - Test Description: Exercise loop failure, recovery, readiness, shutdown, and concurrent control-plane instances with PostgreSQL row locking.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Require durable repositories in production
  - Importance Level: High
  - Description: Fail composition when a required repository is absent; keep in-memory fakes explicit and test-only. Evidence: [ARCHITECTURE.md#arc-002](./ARCHITECTURE.md#arc-002).
  - Test Description: Verify production construction rejects missing repositories and test constructors still use explicit fakes without silently losing persistence.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Add route and JSON contract checks
  - Importance Level: Medium
  - Description: Designate one API contract authority and validate Go routes, permissions, status codes, representative JSON, and frontend API models. Evidence: [ARCHITECTURE.md#arc-004](./ARCHITECTURE.md#arc-004).
  - Test Description: Run route, permission, and JSON contract tests against representative frontend requests and responses.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Consolidate schedule semantics
  - Importance Level: Medium
  - Description: Route API preview, validation, scheduler calculation, timezone, misfire, concurrency, and catch-up behavior through one canonical schedule-policy implementation. Evidence: [ARCHITECTURE.md#arc-005](./ARCHITECTURE.md#arc-005).
  - Test Description: Cover timezone, cron, interval, missed-fire, concurrency, catch-up, and invalid-expression cases at API and durable due-run boundaries.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Narrow the NATS orchestration seam
  - Importance Level: Medium
  - Description: Keep concrete JetStream types inside the queue adapter or a narrow integration seam and use existing queue ports where substitution is needed. Evidence: [ARCHITECTURE.md#arc-006](./ARCHITECTURE.md#arc-006).
  - Test Description: Add a real NATS integration path for consumer setup, acknowledgement, retry, and delivery behavior before replacing production signatures.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Publish current architecture documentation
  - Importance Level: Medium
  - Description: Mark v0-v3 reviews as historical or target documents and maintain one current topology, ownership, health, and deployment source of truth. Evidence: [ARCHITECTURE.md#arc-007](./ARCHITECTURE.md#arc-007).
  - Test Description: Review current documents against source and link each unresolved claim to current code or tests.
  - Test Result: Not run
  - Commit Hash: Not committed

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

- [ ] Measure and optimize cron parsing
  - Importance Level: Medium
  - Description: Benchmark the duplicated cron parsing in `nextCronMinute`; only reuse parsed fields if measurements show scheduler CPU or lock time is material. Evidence: [CODE.md#code-002](./CODE.md#code-002), [OPTIMIZATION.md#opt-002](./OPTIMIZATION.md#opt-002).
  - Test Description: Compare dense, sparse, unsatisfiable, next-day, near-one-year, and 1,000-occurrence catch-up cases with `-benchmem`; preserve cron semantics.
  - Test Result: Not run
  - Commit Hash: Not committed

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

- [ ] Harden non-local Compose deployments
  - Importance Level: High
  - Description: Keep PostgreSQL and NATS private or loopback-only, remove reusable/default credentials and bootstrap values from deployment configuration, and require explicit production secrets plus authenticated TLS transport. Evidence: [SECURITY.md#sec-001](./SECURITY.md#sec-001).
  - Test Description: Render the production Compose configuration; verify no exposed infrastructure ports or known defaults remain, and confirm unauthenticated PostgreSQL/NATS connections fail in an isolated network.
  - Test Result: Not run
  - Commit Hash: Not committed

- [x] Reject wildcard credentialed CORS
  - Importance Level: Medium
  - Description: Wildcard origins are ignored; exact trusted origins retain credentialed CORS headers, and untrusted state-changing requests remain subject to CSRF. Evidence: [SECURITY.md#sec-002](./SECURITY.md#sec-002).
  - Test Description: `GOCACHE=/tmp/glyphflow-go-cache-todo-solver go test ./internal/api -run 'TestCORS|TestCSRF' -count=1`
  - Test Result: PASS — focused CORS and CSRF tests passed.
  - Commit Hash: `21b8894ae7918850f0c3b29d1a1e3f957aca1e43`

## Bug Fixes

- [ ] Bound pagination arithmetic
  - Importance Level: Medium
  - Description: Prevent integer overflow in the shared page helper before calculating slice offsets; return a bounded empty page or client error for out-of-range page values. Evidence: [CODE.md#code-001](./CODE.md#code-001).
  - Test Description: Cover maximum integer, negative, malformed, and very large `page` values on repository-backed and in-memory endpoints; verify no handler panic.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Handle corrupt worker boot metadata
  - Importance Level: Medium
  - Description: Distinguish missing boot metadata from read failures, reject or report malformed records, and handle `os.Setenv` failures so durable recovery is not silently skipped. Evidence: [CODE.md#code-003](./CODE.md#code-003).
  - Test Description: Exercise missing, corrupt, unreadable, and environment-setting failure cases; verify recovery is attempted or startup fails explicitly.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Close control-plane resources on startup errors
  - Importance Level: Low
  - Description: Install one cleanup path immediately after database/NATS resource creation and avoid startup exits that bypass deterministic close calls. Evidence: [CODE.md#code-004](./CODE.md#code-004).
  - Test Description: Inject failures at each startup stage and assert every opened resource is closed.
  - Test Result: Not run
  - Commit Hash: Not committed
