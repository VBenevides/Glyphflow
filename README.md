<p align="center">
  <img src="assets/glyphflow.png" alt="Glyphflow logo" width="160">
</p>

<h1 align="center">Glyphflow</h1>

<p align="center">
  <a href="VERSION"><img src="https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fraw.githubusercontent.com%2FVBenevides%2FGlyphflow%2Fmain%2FVERSION&amp;search=%28%5B0-9.%5D%2B%29&amp;replace=%241&amp;label=version&amp;prefix=v&amp;color=blue&amp;style=flat-square" alt="Version from VERSION"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/VBenevides/Glyphflow?style=flat-square" alt="MIT License"></a>
  <img src="https://img.shields.io/badge/phase-alpha-orange?style=flat-square" alt="Phase alpha">
  <a href="https://github.com/VBenevides/Glyphflow/actions/workflows/ci.yml"><img src="https://github.com/VBenevides/Glyphflow/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
  <a href="https://github.com/VBenevides/Glyphflow/actions/workflows/codeql.yml"><img src="https://github.com/VBenevides/Glyphflow/actions/workflows/codeql.yml/badge.svg?branch=main" alt="CodeQL"></a>
  <a href="https://github.com/VBenevides/Glyphflow/actions/workflows/security.yml"><img src="https://github.com/VBenevides/Glyphflow/actions/workflows/security.yml/badge.svg?branch=main" alt="Dependency Security"></a>
</p>

<p align="center">
  An open-source control plane for running commands across servers and virtual machines.
</p>

<p align="center">
  Define work once. Place it where it belongs. See every attempt, event, and log.
</p>

Glyphflow turns scattered scripts and manually maintained cron jobs into a
managed execution system. A central web console stores versioned task
definitions, schedules runs, chooses an eligible worker, and gives operators
one place to inspect what happened.

## Documentation

- [User guide](docs/USER-GUIDE.md)
- [Administrator guide](docs/ADMIN-GUIDE.md)
- [Production operations runbook](docs/OPERATIONS-RUNBOOK.md)
- [Examples](docs/examples/README.md)

Workers run on the machines that own the work. They connect outbound to the
control plane's message bus, execute commands locally, and report signed
lifecycle events and logs back to the console. PostgreSQL stays with the
control plane; workers do not need database credentials or a PostgreSQL client.

## Main features

Glyphflow is for teams that need more control than “run this script somewhere”
but less ceremony than a full container-orchestration platform.

- **Central operations:** define tasks, schedules, runner pools, resources, and permissions in one console.
- **Schedule Gantt chart:** project seven days of cron executions in week or daily views, group by runner or task, filter the timeline, and inspect occurrence details.
- **Resource conflict detection and notification:** reject schedules that would overlap on exclusive resources, mark conflicts in the Gantt, and notify operators in the Overview with a link to the affected projection.
- **Distributed execution:** run work on Linux or Windows machines without opening inbound worker ports.
- **Useful history:** inspect attempts, state transitions, streamed stdout/stderr, exit codes, and audit events.
- **Deliberate placement:** send work to a pool, pin it to a runner, or match runner capability tags.
- **Recoverable delivery:** durable state, leases, fencing, local worker recovery, and signed messages handle restarts and redelivery.
- **Self-hosted by design:** the required runtime is PostgreSQL, NATS JetStream, the Glyphflow control plane, and your workers.

## What you can do

### Define repeatable tasks

Task versions are immutable after publication. A task can
include:

- an argument-array command, with no shell parsing;
- a working directory and environment variables;
- global-variable references such as `$ENV:BACKUP_ROOT`;
- named secret references kept separate from command text; values are injected as
  environment variables only when a runner starts the task;
- a runner pool or a specific runner;
- capability selectors such as `platform=linux` and `architecture=amd64`;
- execution timeout and maximum attempts;
- ambiguity handling for uncertain delivery outcomes; and
- exclusive or non-blocking resource requirements.

The command editor accepts one argument per line. For example:

```text
/usr/local/bin/backup
--database
production
--output
$ENV:BACKUP_ROOT
```

Glyphflow sends those arguments directly to the operating system. It does not
turn the command into a shell string.

### Schedule and observe execution

Run tasks manually or trigger them with cron schedules. Schedules support
explicit UTC offsets, occurrence previews, missed-occurrence policies, and
overlap policies:

- queue overlapping runs;
- skip when a run is already active;
- replace the active run; or
- allow overlap up to a configured limit.

The run view makes operations concrete instead of opaque. Filter by task,
runner, state, trigger, and time range; then inspect attempts, state events,
streamed logs, exit-code meanings, and downloadable log output. Cancel, retry,
or reconcile runs when the situation requires an operator decision.

### Manage a worker fleet

Create a one-use enrollment artifact in the web console and run it on the
target machine. The worker enrolls itself, receives its connection details,
persists its identity locally, and begins sending heartbeats.

From the console you can:

- create and manage runner pools;
- set execution capacity;
- see desired state, observed state, heartbeat freshness, and active work;
- drain a runner while allowing current work to finish;
- revoke or re-enable a runner; and
- archive a runner permanently.

Enrollment currently supports Linux and Windows AMD64 binaries in three forms:

- **GUI:** desktop status window and system-tray controls;
- **TUI:** terminal UI with lower memory usage; or
- **Headless:** no graphical or terminal UI, intended for services and VMs.

Workers keep a local SQLite store for accepted orders, boot recovery, and
events waiting to be published. A worker can therefore recover its durable
state after a process, network, or queue interruption.

### Keep administration accountable

Glyphflow includes password authentication and OIDC, session management, CSRF
protection, permission-aware routes, role-based access control, user and SSO
administration, and audited administrative actions. The built-in roles are
`admin`, `operator`, and `user`; permissions determine which operational and
administrative views each role can use.

The console also provides global variables, resource leases, exit-code
meanings, and public API documentation. Once the control plane is running:

- interactive API docs: `http://localhost:8080/docs`
- OpenAPI document: `http://localhost:8080/openapi.json`
- liveness: `http://localhost:8080/api/v1/healthz`
- readiness: `http://localhost:8080/api/v1/readyz`

## How it works

```text
Browser
  │ task, schedule, and operator actions
  ▼
Control plane ───── PostgreSQL
  │                  durable state, versions, leases, audit history
  │
  └─────────────── NATS JetStream
                       signed orders and lifecycle events
                           │
                           ▼
                     Outbound-only worker
                       local SQLite + child process
```

For a scheduled run, the control plane:

1. creates the run and dispatch outbox record in PostgreSQL;
2. selects a healthy worker with capacity, matching capabilities, and available resources;
3. signs and publishes the execution order through NATS JetStream;
4. lets the worker verify, persist, and execute the order locally; and
5. verifies signed worker events and applies them to the durable run history.

The control plane is currently one process containing the HTTP API,
scheduler, dispatcher, event ingestion, heartbeats, start claims, session
cleanup, and readiness checks. Workers remain separate processes or machines.
See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the implementation source of
truth.

## Quick start

### Requirements

- Docker Compose
- Go 1.25 or newer
- Node.js 22.22.2 or newer
- npm

### Start the development environment

```bash
git clone https://github.com/VBenevides/Glyphflow.git
cd Glyphflow
./dev_run.sh
```

`dev_run.sh` starts PostgreSQL and NATS with Docker Compose, builds local
runner binaries, and starts the Go control plane and React development server.
The provisional, non-guaranteed development targets are in
[`docs/DEV-PROFILE.md`](docs/DEV-PROFILE.md).

Open <http://localhost:5173> and sign in with the development bootstrap
account:

```text
email:    admin@example_domain.com
password: admin-password-123
```

This account and these credentials are for local development only. Press
`Ctrl-C` to stop the processes. Development Docker volumes are retained so a
later run can reuse local PostgreSQL and NATS data.

### Make your first run

1. Open **Runners and pools → Enroll runner**.
2. Choose the default pool, `linux` / `amd64`, and **Headless** for a local service, or **GUI** / **TUI** when you want a visible worker.
3. Download the one-use binary and run it on the target machine.
4. Wait for the runner to show **Online**.
5. Create a task with an argument-array command, then start a manual run.
6. Open the run to watch live output and inspect its final event history.

Enrollment credentials expire after 15 minutes by default and can be used only
once. A downloaded binary is bound to the runner configuration that created
it; generate a new artifact if the enrollment expires or is consumed.

## Production deployment

The released [deployment contract](docs/DEPLOYMENT.md) covers both the full
Compose stack and a partial deployment with separately managed NATS and
PostgreSQL services.

The release workflow publishes a ready-to-use image to GitHub Container
Registry when a GitHub release is published. The release tag must match
`VERSION`, with or without a leading `v`. Stable releases also receive the
`latest` tag.

```bash
export GLYPHFLOW_VERSION="$(tr -d '[:space:]' < VERSION)"
export GLYPHFLOW_IMAGE="ghcr.io/vbenevides/glyphflow:${GLYPHFLOW_VERSION}"
docker pull "$GLYPHFLOW_IMAGE"
```

The GitHub release also contains a Linux AMD64 image archive and SHA-256
checksum for environments without registry access:

```bash
export GLYPHFLOW_VERSION="$(tr -d '[:space:]' < VERSION)"
archive="glyphflow-${GLYPHFLOW_VERSION}-linux-amd64.tar.gz"
sha256sum -c "$archive.sha256"
docker load -i "$archive"
```

The current release is clean-install-only. On first start, the image applies
its single canonical PostgreSQL schema to a new database; it does not upgrade
databases from earlier releases.

The base [`compose.yaml`](compose.yaml) is intentionally convenient for local
use. It exposes PostgreSQL and NATS and contains development credentials. Do
not use those defaults for a public deployment.

For production, provide the required values and secret files described in
[`compose.production.yaml`](compose.production.yaml), then start the stack:

```bash
COMPOSE_PROJECT_NAME=client-example docker compose -f compose.yaml -f compose.production.yaml up -d
```

The deployment contract supports one isolated production stack per client. Use
a unique `COMPOSE_PROJECT_NAME` for each client so its PostgreSQL data, NATS
authority, network, secrets, backups, and administrator scope stay separate.
Shared tenancy and scaling any service beyond one replica are unsupported in
this release. The HA trigger is an approved client RTO or uptime target that
the measured singleton restart recovery cannot meet. Until that trigger is
recorded, do not run control-plane replicas.

Production configuration requires, at minimum:

| Area | Required configuration |
| --- | --- |
| Database | Protected `DATABASE_URL_FILE` containing a URL with `sslmode=verify-full`; PostgreSQL CA, certificate, key, and password files |
| Messaging | Protected `NATS_URL_FILE` containing a `tls://` URL plus client certificate, key, and CA files |
| Web security | An HTTPS `WEB_ORIGIN`, explicit `CORS_ORIGIN`, and `CSRF_ORIGINS` |
| Application secrets | Protected `ACCESS_TOKEN_SECRET_FILE`, `PASSWORD_PEPPER_FILE`, `CONTROL_PLANE_SIGNING_PRIVATE_KEY_FILE`, and `GLYPHFLOW_BOOTSTRAP_PASSWORD_FILE`; the 32-byte secret encryption key is loaded from or generated in `DATA_DIR` |
| Bootstrap | `GLYPHFLOW_BOOTSTRAP_EMAIL`, protected `GLYPHFLOW_BOOTSTRAP_PASSWORD_FILE`, and `GLYPHFLOW_SYSTEM_ADMINS` |
| Network | `GLYPHFLOW_PORT` for the web listener; keep PostgreSQL and NATS private |

Set `ENVIRONMENT=production` and keep `ALLOW_INSECURE_TRANSPORT=false`.
The `*_FILE` and `*_SOURCE` values name protected host files; the overlay
mounts them read-only and does not render their contents into Compose output.
Do not commit secret files or put their values in task commands and logs.

The production overlay binds the web listener to `127.0.0.1` by default so a
reverse proxy can terminate public TLS. If the application is exposed through
a different proxy or hostname, update `WEB_ORIGIN`, `CORS_ORIGIN`, and
`CSRF_ORIGINS` together.

## Worker deployment

The recommended worker workflow is enrollment from the console:

1. create an enrollment for a runner name, pool, platform, architecture, capacity, and UI;
2. download the generated binary;
3. copy it to the target machine; and
4. run it as a service or supervised process.

The binary contains a short-lived bootstrap credential and the control-plane
public key. On first start it enrolls over HTTP(S), receives its NATS
connection, generates and persists its own signing key, and stores local state
under its data directory. Set `GLYPHFLOW_CONTROL_PLANE_URL` or
`GLYPHFLOW_NATS_ENDPOINT` on the target machine when the embedded endpoints
are not reachable from that network.

For maintainers building a worker from source:

```bash
cd backend
go build -o bin/glyphflow-worker ./cmd/worker
go build -tags workerui -o bin/glyphflow-worker-gui ./cmd/worker
go build -tags workerui_tui -o bin/glyphflow-worker-tui ./cmd/worker
```

The default build is headless. Cross-platform release builds and embedded
enrollment artifacts are produced by
[`backend/build_runner_binaries.sh`](backend/build_runner_binaries.sh). See
[`backend/cmd/worker/README.md`](backend/cmd/worker/README.md) for UI build
details and desktop dependency requirements.

## Configuration reference

The development script supplies safe local defaults. Outside development,
Glyphflow validates the important security boundary at startup rather than
silently accepting an insecure deployment.

### Control plane variables

The control-plane binary consumes the variables below directly. The production
Compose overlay loads sensitive values from the protected file variables named
with `_FILE` or `_SOURCE`.

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` / `DATABASE_URL_FILE` | PostgreSQL connection URL; production Compose reads the protected file |
| `DATABASE_STORAGE_CAPACITY_BYTES` | Positive application-database storage budget used with PostgreSQL `pg_database_size`; required outside development |
| `NATS_URL` / `NATS_URL_FILE` | Control-plane NATS endpoint; use `tls://` outside development; production Compose reads the protected file |
| `NATS_CERT_FILE`, `NATS_KEY_FILE`, `NATS_CA_FILE` | NATS client TLS material |
| `ACCESS_TOKEN_SECRET` / `ACCESS_TOKEN_SECRET_FILE` | Session and access-token signing secret; minimum 32 bytes |
| `CONTROL_PLANE_SIGNING_PRIVATE_KEY` / `CONTROL_PLANE_SIGNING_PRIVATE_KEY_FILE` | Persistent base64 raw Ed25519 private key outside development |
| `PASSWORD_PEPPER` / `PASSWORD_PEPPER_FILE` | Password hashing pepper; minimum 16 bytes when password login is enabled |
| `WEB_ORIGIN` | Canonical browser origin, HTTPS outside development |
| `CORS_ORIGIN` | Comma-separated CORS allowlist |
| `CSRF_ORIGINS` | Comma-separated CSRF origin allowlist |
| `ENVIRONMENT` | Use `development` locally and `production` for the production overlay |
| `ALLOW_INSECURE_TRANSPORT` | Development-only escape hatch; keep `false` in production |
| `GLYPHFLOW_BOOTSTRAP_EMAIL` / `GLYPHFLOW_BOOTSTRAP_PASSWORD` / `GLYPHFLOW_BOOTSTRAP_PASSWORD_FILE` | Optional first administrator credentials; production Compose reads the protected file |
| `GLYPHFLOW_SYSTEM_ADMINS` | Administrator emails separated by spaces, commas, or semicolons |
| `ENABLE_PASSWORD_LOGIN` | Enable password sign-in; production defaults to disabled in the overlay |
| `ENABLE_PASSWORD_REGISTRATION` | Enable self-service password registration; production defaults to disabled |
| `DEFAULT_ROLE_ID` | Role assigned to newly created users |
| `DATA_DIR` | Persistent control-plane data, including `secret-encryption.key` and the development signing key |
| `SECRET_ENCRYPTION_KEY_FILE` | Optional path to an existing base64-encoded 32-byte key file; defaults to `DATA_DIR/secret-encryption.key` |
| `GLYPHFLOW_DISABLE_NGINX` | Set to `true` when private ingress targets the control-plane listener directly on port `8080`; defaults to `false` and starts Nginx on port `80` |

For private container ingress such as ACA, set `GLYPHFLOW_DISABLE_NGINX=true` and
route ingress to port `8080`.

The control plane stores the AES-256-GCM encryption key in
`DATA_DIR/secret-encryption.key`. Back up this file with the database. If it is
deleted, lost, or replaced, encrypted secrets cannot be recovered and must be
re-entered; startup warns when a new key is generated.

Manage named secrets in **Administration → Secrets**. Task versions map an
environment variable name to a named secret. The control plane decrypts the
selected values only for the assigned runner immediately before execution;
secret values are not stored in task orders, logs, or API responses.

To deploy with an existing key, copy the unchanged file to the persistent
`DATA_DIR/secret-encryption.key` location before the first start, or set
`SECRET_ENCRYPTION_KEY_FILE` to a mounted local file path. The file must contain
one base64-encoded 32-byte key and be readable only by its owner (`0600` or
`0400`). A new key can be created with:

```bash
umask 077
openssl rand -base64 32 > secret-encryption.key
chmod 600 secret-encryption.key
```

The application loads an existing valid file and does not generate a replacement.
Copy the same file when migrating the database; changing it makes existing
encrypted secrets undecryptable.

`GLYPHFLOW_BOOTSTRAP_EMAIL` and either `GLYPHFLOW_BOOTSTRAP_PASSWORD` (local
development) or the protected `GLYPHFLOW_BOOTSTRAP_PASSWORD_FILE` (production
Compose) must be set to create the bootstrap account.
`GLYPHFLOW_SYSTEM_ADMINS` is independent: matching users receive the immutable
`admin` role and cannot be demoted or disabled.

### Worker variables

Normally the enrollment artifact supplies the worker connection. For a worker
started without an embedded bootstrap, configure:

| Variable | Purpose |
| --- | --- |
| `RUNNER_ID` | Stable runner identifier |
| `NATS_URL` | Worker NATS endpoint |
| `DATA_DIR` | Persistent worker directory containing `runner.sqlite` and signing keys |
| `MAX_MESSAGE_BYTES` | Maximum accepted protocol message size |
| `MAX_OUTPUT_BYTES` | Maximum captured process output; cannot exceed `MAX_MESSAGE_BYTES` |
| `ENVIRONMENT` | Use `development` with `ALLOW_INSECURE_TRANSPORT=true` only for plain local endpoints; production requires HTTPS and NATS TLS |
| `NATS_CERT_FILE`, `NATS_KEY_FILE`, `NATS_CA_FILE` | Worker NATS client TLS material in production |
| `GLYPHFLOW_CONTROL_PLANE_URL` | Optional override for the enrollment endpoint |
| `GLYPHFLOW_NATS_ENDPOINT` | Optional override for the NATS endpoint embedded in an artifact |

Outside development, the effective control-plane endpoint must use `https://`
and the NATS endpoint must use `tls://`. Plain `http://` and `nats://` values
require both `ENVIRONMENT=development` and `ALLOW_INSECURE_TRANSPORT=true`.

Persist `DATA_DIR`. Deleting it removes the worker's local recovery state and
identity, so the worker may need to be enrolled again.

## Delivery and security model

Glyphflow uses at-least-once message delivery. Restarts and network failures
can cause a message to be delivered more than once. Unique order and event
IDs, leases, fencing tokens, inbox/outbox records, and worker-local durable
state make the platform recoverable and deduplicable.

Glyphflow does **not** promise exactly-once command execution. An arbitrary
command can change an external system, and the platform cannot undo that
effect. Enable automatic retries only for commands that are safe to retry.

The security boundaries are:

- workers initiate outbound network connections;
- workers do not receive PostgreSQL credentials;
- NATS permissions can restrict each worker to its own subjects;
- Ed25519 signatures authenticate orders and worker events end to end;
- workers verify an order before parsing or executing its payload;
- each worker creates and persists its private signing key locally;
- enrollment credentials are short-lived and one-use;
- command arguments are passed without a shell;
- working directories, process resources, and output are bounded; and
- logs redact private keys, tokens, passwords, and secret values.

A valid signature proves the message source. It does not prove that a
compromised worker reports correct results. Protect the worker host and use
the runner lifecycle controls to revoke a worker that is no longer trusted.

## Development

Run the backend checks:

```bash
cd backend
GOCACHE=/tmp/glyphflow-go-cache go test ./...
GOCACHE=/tmp/glyphflow-go-cache go vet ./...
go mod verify
```

Run the frontend checks:

```bash
cd frontend
npm ci
npm test -- --run
npm run typecheck
npm run build
```

The release baseline is available as:

```bash
./scripts/release-check.sh
```

The repository's current topology and verification links are in
[`ARCHITECTURE.md`](ARCHITECTURE.md). Historical design and review notes live
under [`internal/`](internal/); they are useful context, but they are not
runtime contracts.

## Project structure

```text
backend/       Go control plane, worker, protocol, queue, and persistence
frontend/      React, TypeScript, Vite, and Vitest web console
build/         Container image and Nginx configuration
scripts/       Release, security, and SBOM checks
internal/      Historical design records, reviews, and roadmap notes
```

## License

Glyphflow's original source is released under the [MIT License](LICENSE).
Redistributed images and binaries also contain third-party components; keep
[`THIRD-PARTY-NOTICES`](THIRD-PARTY-NOTICES) with copies of the project.
PostgreSQL, NATS, and container base images referenced by Compose keep their
own licenses and terms.
