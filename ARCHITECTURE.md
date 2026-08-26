# Glyphflow architecture

Status: current implementation source of truth. Updated 2026-08-25 for the
`version/0.2.3` checkout. Dated reviews in `internal/` and `.agent-work/` are
historical evidence or target designs, not runtime contracts.

## Topology

```text
Browser
  -> Vite dev server, or Nginx in the image
  -> Go control plane (:8080)
       -> PostgreSQL (authoritative control-plane state)
       -> NATS JetStream (orders, events, heartbeats, control messages)
            -> outbound-only Go workers
                 -> SQLite (worker-local recovery and pending events)
                 -> child processes (argument-array execution)
```

In the container deployment, Nginx serves the built React application and
proxies `/api/`, `/docs`, and `/openapi.json` to the control plane
([`build/nginx.conf`](build/nginx.conf)). In local development, `dev_run.sh`
runs Vite and the control plane directly and starts only PostgreSQL and NATS
with Compose. Set `GLYPHFLOW_DISABLE_NGINX=true` when private ingress routes
directly to the control-plane listener on port `8080`.

The control plane is one process. Its HTTP API, scheduler, dispatcher,
heartbeat monitor, start-claim server, session cleanup, and readiness state
share one restart boundary ([`backend/cmd/controlplane/main.go`](backend/cmd/controlplane/main.go)).
Workers are separate processes or machines and do not connect to PostgreSQL.

## Ownership

| Boundary | Owns | Source |
|---|---|---|
| Frontend | React views, routing, and HTTP API client | [`frontend/src`](frontend/src) |
| API | HTTP routes, authentication, authorization, and request orchestration | [`backend/internal/api`](backend/internal/api) |
| Store | PostgreSQL repositories, canonical schema, transactions, and durable state transitions | [`backend/internal/store`](backend/internal/store), [`backend/migrations/001_canonical.sql`](backend/migrations/001_canonical.sql) |
| Control plane | Scheduling, dispatch, event ingestion, heartbeats, and start claims | [`backend/internal/controlplane`](backend/internal/controlplane) |
| Protocol | Signed order, event, and control-message formats and verification | [`backend/internal/protocol`](backend/internal/protocol) |
| Queue | NATS JetStream connection, subjects, consumers, and acknowledgements | [`backend/internal/queue`](backend/internal/queue) |
| Worker | Enrollment, order verification, execution, recovery, heartbeats, and event publishing | [`backend/cmd/worker`](backend/cmd/worker), [`backend/internal/worker`](backend/internal/worker) |

PostgreSQL is the source of truth for control-plane state. NATS transports
messages and provides durable delivery; it is not business state. SQLite is
only worker-local state for accepted orders, boot recovery, and events waiting
for publication ([`backend/internal/worker/store.go`](backend/internal/worker/store.go)).

## Runtime flow

1. The API stores task and schedule changes in PostgreSQL.
2. The scheduler creates due runs and snapshots the required execution data in
   a database transaction ([`backend/internal/store/schedules.go`](backend/internal/store/schedules.go)).
3. The dispatcher claims work, signs an order, and publishes it to the
   runner's JetStream subject ([`backend/internal/controlplane/dispatcher.go`](backend/internal/controlplane/dispatcher.go)).
4. The worker verifies and durably accepts the order, executes the command, and
   publishes signed lifecycle events ([`backend/internal/worker/runtime.go`](backend/internal/worker/runtime.go)).
5. The control plane verifies events and applies them to PostgreSQL. The
   frontend reads the resulting state through the API.

Delivery is at least once. IDs, leases, fencing tokens, inboxes, and outboxes
provide deduplication and recovery; arbitrary external command effects are not
exactly once.

## Health and readiness

- `/api/v1/healthz` returns `200` when the HTTP process is serving.
- `/api/v1/readyz` returns `503` until the database responds, a NATS JetStream
  client has been created, and all five control-plane components have reported healthy:
  `session-cleanup`, `heartbeat`, `dispatcher`, `start-claim`, and `scheduler`.
- Production composition enables the durable-repository guard before serving
  requests ([`api.Server.ValidateDurableRepositories`](backend/internal/api/api.go)
  and [`backend/internal/api/api_test.go`](backend/internal/api/api_test.go)).
- Component state is implemented by [`controlplane.Health`](backend/internal/controlplane/health.go)
  and covered by [`health_test.go`](backend/internal/controlplane/health_test.go).
  A component failure makes readiness fail until its loop recovers.

Readiness is process-level dependency and component readiness. It is not a
multi-replica leader election or a load/failover guarantee.

## Deployment

- Local development: [`dev_run.sh`](dev_run.sh) plus [`compose.yaml`](compose.yaml).
- Container image: [`build/Dockerfile`](build/Dockerfile), containing the built
  frontend, Nginx, the control-plane binary, the canonical schema, and runner
  binaries.
- Version 0.2.3 is clean-install-only. The control plane initializes a new
  PostgreSQL database from
  [`backend/migrations/001_canonical.sql`](backend/migrations/001_canonical.sql);
  it does not upgrade an earlier schema.
- Runtime services: one isolated stack per client containing one control-plane
  instance, PostgreSQL, NATS JetStream, Nginx/web, and any number of separately
  deployed workers. Shared tenancy and service replicas are unsupported in
  version 0.2.3.
- Outside development, configuration validation requires HTTPS, NATS TLS,
  signing-key material, and production client certificates where applicable
  ([`backend/internal/config/config.go`](backend/internal/config/config.go)).

The Compose defaults are for local development: they expose PostgreSQL/NATS
and contain development credentials. Supply production secrets, TLS settings,
and restricted network exposure before using the image outside a local
environment.

## Verification

The architecture boundaries are exercised by:

- [`backend/internal/controlplane`](backend/internal/controlplane) tests for
  scheduling, dispatch, heartbeats, start claims, and health state.
- [`backend/internal/worker`](backend/internal/worker) tests for execution,
  durable storage, signed messages, and recovery.
- [`backend/internal/api/api_test.go`](backend/internal/api/api_test.go) for
  readiness, routes, CORS/CSRF, and production repository validation.
- [`backend/internal/integration`](backend/internal/integration) for
  PostgreSQL/NATS flows when integration dependencies are available.

Baseline commands:

```bash
cd backend && GOCACHE=/tmp/glyphflow-go-cache go test ./...
cd backend && GOCACHE=/tmp/glyphflow-go-cache go vet ./...
cd frontend && npm test -- --run
cd frontend && npm run typecheck
cd frontend && npm run build
```

Historical reviews remain useful for rationale and unresolved work:
[`internal/v1/REPORT.md`](internal/v1/REPORT.md) explicitly describes a target
design, while [`internal/v2/REPORT.md`](internal/v2/REPORT.md),
[`internal/v3/REPORT.md`](internal/v3/REPORT.md), and the dated analysis
artifacts are snapshots of earlier checkouts.
