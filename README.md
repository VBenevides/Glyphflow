<p align="center">
  <img src="assets/glyphflow.png" alt="Glyphflow logo" width="160">
</p>

<h1 align="center">Glyphflow</h1>

<p align="center">
  A self-hosted control plane for reliable command execution across servers and virtual machines.
</p>

<p align="center">
  Define work once. Place it where it belongs. See every attempt, event, and log.
</p>

Glyphflow gives operations teams one place to define tasks, schedule work,
choose the right machine, and understand what happened. Workers execute near
the systems they manage, while the control plane provides durable state and a
clear operational history.

## Why Glyphflow

- Replace scattered scripts and manually maintained cron jobs with versioned tasks and schedules.
- Run work on Linux and Windows machines without opening inbound worker ports.
- Place execution by runner pool, specific runner, capability tags, capacity, and shared resources.
- Give operators live output, attempts, state transitions, exit codes, and audit history.
- Keep the platform self-hosted with PostgreSQL, NATS JetStream, the control plane, and your workers.

## Useful for

- Database backups and maintenance jobs
- Deployments and release operations
- Data processing and file movement
- Infrastructure checks and remediation
- Scheduled internal automation that needs ownership and traceability

## Security and reliability

Glyphflow is designed to keep execution boundaries explicit:

- Workers connect outbound and do not receive PostgreSQL credentials.
- Orders and worker events are authenticated with Ed25519 signatures.
- Production configuration requires TLS for PostgreSQL and NATS, HTTPS browser origins, and protected secret files.
- Authentication includes password or OIDC login, sessions, CSRF protection, RBAC, and audited administration.
- Durable state, leases, fencing, inbox/outbox records, and worker-local recovery handle restarts and redelivery.
- Task arguments are passed directly to the operating system without shell parsing.

Delivery is at least once, not exactly once. Commands that change external
systems must be safe to retry or provide their own idempotency.

## Start locally

```bash
./dev_run.sh
```

Open <http://localhost:5173> and use the development account described in the
[technical documentation](docs/README.md#quick-start).

## User guide and examples

Start with the [User Guide](docs/USER-GUIDE.md), then follow the end-to-end
[Examples](docs/examples/README.md) using isolated fake data.

- [Admin guide](docs/ADMIN-GUIDE.md)
- [Architecture](ARCHITECTURE.md)
- [Deployment and configuration](docs/README.md#production-deployment)
- [Complete documentation README](docs/README.md)

Glyphflow is released under the [MIT License](LICENSE).
