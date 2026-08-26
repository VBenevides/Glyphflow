# Production operations runbook

## Supported topology

Glyphflow 0.2.3 supports one isolated stack per client: one control-plane
process connected to one PostgreSQL database and one NATS JetStream deployment.
Workers are separate processes or hosts and connect outbound. Shared tenancy
and service replicas are unsupported in this release; it does not claim
control-plane, PostgreSQL, or NATS high availability.

A reverse proxy may provide the public HTTPS endpoint, but it does not make
the control plane redundant. Keep PostgreSQL and NATS on private networks.
The included Compose deployment does not provision per-runner NATS accounts or
ACLs; select and configure that credential authority before treating workers
as mutually isolated tenants.

## Service boundaries and targets

Record the values for each client before onboarding. The values below are
operating targets, not a service guarantee:

| Item | Baseline | Owner |
| --- | --- | --- |
| Control-plane availability | Restart downtime is expected; no HA target | Deployment operator |
| RPO | The interval between verified PostgreSQL backups | Deployment operator |
| RTO | Restart or restore time measured in the client environment | Deployment operator |
| Audit retention | Append-only until the client's approved retention policy is implemented | Deployment operator |
| Worker published-event retention | 24 hours after successful publish; pending events are retained | Glyphflow worker |
| Run/log retention | No automatic PostgreSQL deletion in this release | Deployment operator |

Do not advertise an RPO, RTO, or uptime value until it is measured against
the client's backup, restore, network, and restart procedures.

## Normal restart

1. Stop scheduling or enable scheduler lockdown in the console.
2. Confirm no deployment or migration is in progress.
3. Restart the control-plane container/process.
4. Check `/api/v1/readyz` and confirm the database and NATS dependencies are
   ready.
5. Confirm active runs, outbox delivery, and runner heartbeats in the console.
6. Disable lockdown and start a controlled smoke task.

Workers keep their SQLite state and signing identity in `DATA_DIR`. Restart a
worker with the same directory. Deleting that directory removes its recovery
state and identity; archive the old runner and enroll a replacement instead.

## Backup and restore

Back up PostgreSQL with a consistent dump and protect the dump like the
database:

```bash
pg_dump --format=custom --file=glyphflow-$(date -u +%Y%m%dT%H%M%SZ).dump "$DATABASE_URL"
```

Include the PostgreSQL dump, deployment secrets, TLS material, and the
control-plane signing key in the protected backup scope. Do not back up
worker private keys to a shared location unless the client's security policy
requires it; preserve each worker's `DATA_DIR` on its host.

Restore into an isolated PostgreSQL instance first, apply the release
migrations, start the control plane against the restored database, and verify
readiness, login, task listing, run history, audit history, and runner
enrollment before replacing the production instance. Record elapsed restore
time and the newest recoverable timestamp as the measured RTO/RPO.

## Upgrade and re-enrollment

1. Take and verify a PostgreSQL backup.
2. Build and checksum the release image and worker artifacts.
3. Enable scheduler lockdown and drain workers.
4. Stop the control plane, deploy the new image, and let migrations finish.
5. Verify readiness and run a harmless smoke task.
6. Restart workers with their existing `DATA_DIR`.
7. If a worker identity or data directory is lost, archive that runner,
   revoke its old enrollment, and create a new one-use enrollment artifact.
8. Record migration output, smoke results, and recovery timings.

Never reuse a consumed enrollment token. Treat a lost worker data directory
as a new identity, not as a recoverable session.

## Incident checks

- `readyz` fails: inspect PostgreSQL connectivity, NATS TLS/credentials, and
  migration status before restarting repeatedly.
- Runs remain waiting: inspect runner heartbeat, pool/capacity, selectors,
  resource leases, and the placement blocker shown on the run.
- Events are delayed: inspect NATS health and the worker's local pending
  event outbox; do not delete pending rows.
- A host is untrusted: revoke or archive the runner, then rotate any secrets
  it could read.
