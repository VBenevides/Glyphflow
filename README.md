<p align="center">
  <img src="assets/glyphflow.png" alt="Glyphflow logo" width="160">
</p>

# Glyphflow

Glyphflow is an open-source platform for script orchestration across servers and virtual machines.

The platform has one central control plane and many remote workers. The control plane schedules work. Workers execute the work.

Glyphflow is an alpha application. The current version is defined in [`VERSION`](VERSION). The repository contains the Go control plane, Go workers, React frontend, PostgreSQL persistence, and NATS JetStream integration.

## Quick start

Requirements: Docker Compose, Go, Node.js 22.22.2 or newer, and npm.

Run the local environment:

```bash
./dev_run.sh
```

The script starts PostgreSQL and NATS, builds the Linux and Windows AMD64 worker binaries, and starts the frontend and control plane.

- Frontend: <http://localhost:5173>
- Control plane: <http://localhost:8080>
- Default email: `admin@example_domain.com`
- Default password: `admin-password-123`

Press `Ctrl-C` to stop the development processes. Docker volumes keep PostgreSQL and NATS data between runs.

## Deployment model

The current release uses one control-plane executable, one NATS JetStream deployment, and any number of outbound-only workers. PostgreSQL remains private to the control plane. The scheduler, dispatcher, event ingestion, HTTP API, housekeeping, health checks, and internal runtime metrics run in the same control-plane process.

Service splitting is deferred until measured scaling, deployment, or ownership needs justify it.

The base [`compose.yaml`](compose.yaml) is for local development. For a
non-local deployment, use [`compose.production.yaml`](compose.production.yaml)
with explicit application, database, and TLS secret values.

## Features

- Define tasks with immutable versions, cron schedules, environment variables, secret references, selectors, retry policies, resource policies, and ambiguity policies.
- Run tasks manually or on a schedule. View run attempts, state events, streamed logs, audit events, and task version history.
- Cancel and retry runs with durable state transitions. Reconcile stale work after worker, queue, database, or network failures.
- Enroll and manage workers and pools. Set execution capacity, view active runs, archive workers, and manage resource leases.
- Use password or OIDC authentication with sessions, CSRF protection, RBAC, SSO, account management, and audited administration.
- Use the responsive React console with light, dark, and neon themes, accessible dialogs, filters, pagination, and permission-aware routes.
- Run persistent workers with SQLite recovery, signed messages, streamed output, concurrent execution, and headless, Gio desktop, or Bubble Tea TUI builds. The Windows TUI build supports minimizing the console to the system tray.
- Select the worker UI during enrollment and binary generation: GUI (default), TUI (lower memory usage), or headless (lowest memory usage).

## Architecture

[`ARCHITECTURE.md`](ARCHITECTURE.md) is the current source of truth for
topology, ownership, runtime readiness, deployment, and verification links.

## Authentication environment

The bootstrap administrator is created only when both `GLYPHFLOW_BOOTSTRAP_EMAIL` (an email address) and `GLYPHFLOW_BOOTSTRAP_PASSWORD` are set. If either is missing, no bootstrap administrator is created.

`GLYPHFLOW_SYSTEM_ADMINS` accepts unique administrator emails separated by spaces, commas, or semicolons. Matching users receive the immutable `admin` role and cannot be disabled or demoted.

`./dev_run.sh` defaults to `admin@example_domain.com` with password `admin-password-123` and includes that email in `GLYPHFLOW_SYSTEM_ADMINS`.

The frontend uses TypeScript, React, React Router, TanStack Query, Vite, Vitest, and Lucide. The project does not require a second frontend framework.

## Task flow

1. A user creates a task in the web application.
2. The Go API stores the task definition in PostgreSQL.
3. The producer detects a due task and selects an active worker.
4. One transaction creates the task run and its outbox record.
5. The dispatcher signs the order and publishes it to JetStream.
6. The assigned worker verifies and stores the order.
7. The worker executes the command without a shell.
8. The worker publishes signed lifecycle events.
9. The control plane verifies each event and updates the task run.
10. The frontend shows the current state and complete event history.

## Delivery model

Glyphflow uses at-least-once delivery. A queue or service restart can deliver the same message again.

Each order has a unique message identifier. Each event has a unique event identifier.

Workers store accepted message identifiers. The control plane stores accepted event identifiers.

Glyphflow does not promise exactly-once command execution. Arbitrary commands can create external effects that the platform cannot reverse.

Automatic retry requires an explicit retry-safe policy. Each retry uses a new attempt number and lease token.

## Security model

- Workers make outbound network connections only.
- Workers do not contain PostgreSQL credentials or a PostgreSQL client.
- Mutual TLS protects queue connections.
- Queue permissions limit each worker to its own subjects.
- Ed25519 signatures protect orders and events end to end.
- The worker verifies an order before it parses or executes the payload.
- Each worker generates and persists its private keys on its target machine.
- A one-use enrollment token expires after 15 minutes by default.
- Commands use argument arrays instead of shell command strings.
- Workers restrict working directories, process resources, and output size.
- Logs do not contain private keys, tokens, or secret values.

A valid signature proves the message source. It does not prove that a compromised worker reports correct results.

## Open-source policy

Glyphflow uses open-source components with approved licenses. The preferred license is MIT.

The project also permits Apache-2.0, BSD, ISC, the PostgreSQL License, and public-domain software.

Each release includes SPDX license information and a software bill of materials. The required deployment does not depend on proprietary services.

## Project structure

```text
backend/    Go control plane and worker source
frontend/   Default TypeScript and React application
internal/   Design documents, migration notes, and project roadmap
```

## Development notes

Implementation and verification records are in [`internal/v2/TODO.md`](internal/v2/TODO.md) and [`internal/v2/TODO-2.md`](internal/v2/TODO-2.md).

The design records in `internal/` describe the migration from the local script scheduler.

## License

Glyphflow uses the MIT License. See [LICENSE](LICENSE).
