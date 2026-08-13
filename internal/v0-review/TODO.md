# Backend audit TODO

## Security

- [x] **High:** Remove the always-successful control-plane authenticator. `79906c6`
  - Refuse startup without configured authentication.
  - Require `task.create` for task POST requests.
- [x] **High:** Require NATS mutual TLS and exact worker subject permissions. `e19562a`
  - Bind certificate identity to the runner identifier.
  - Add real cross-worker denial tests.
- [x] **High:** Complete worker execution controls before wiring orders. `ef5065c`
  - Bound output.
  - Stop full process groups.
  - Enforce time, process, memory, identity, executable, and secret rules.
- [x] **High:** Add one complete order verifier and one complete event verifier. `ff18679`
  - Verify before typed payload use.
  - Persist replay decisions across restarts.
- [x] **High:** Replace presence-only security and release checks. `5c46925`
  - Uncheck unsupported `internal/TODO.md` claims.
  - Fail release checks when the SBOM has no packages.
- [x] **Medium:** Upgrade `github.com/jackc/pgx/v5` to `v5.9.2` or newer. `752b2d3`
  - Run tests, vet, and `govulncheck` again.
- [ ] **Medium:** Resolve symbolic links before allowed-root checks.
- [ ] **Medium:** Add HTTP timeouts and a bounded shutdown context.

## Bugs

- [ ] **High:** Set `cmd.Dir` to the validated worker directory.
  - Add one working-directory test.
- [ ] **High:** Remove the non-atomic JSON worker store.
  - Replace it with the planned SQLite transaction during worker wiring.
- [ ] **High:** Enforce allowed state transitions in the compare-and-swap update.
- [ ] **High:** Reject unsupported cron syntax and correct day-field behavior.
- [ ] **Medium:** Return `501` for task creation until it validates and stores tasks.
- [ ] **Medium:** Serialize migrations with a PostgreSQL advisory transaction lock.
- [ ] **Medium:** Preserve structured command arguments during SQLite import.
- [ ] **Low:** Delete the no-op `Plane.Stop` method.

## New Features

- [ ] **Critical:** Implement one secure end-to-end task path.
  - Create the run and outbox in one transaction.
  - Publish and durably accept one signed order.
  - Execute once and publish one signed final event.
  - Verify the event and update the run in one transaction.
- [ ] **High:** Add the required JetStream publisher, consumers, acknowledgements, and dead-letter flow.
- [ ] **High:** Add real task, run, event, runner, cancellation, and retry endpoints as workflows become available.
- [ ] **High:** Add enrollment, certificate issue, key rotation, and revocation.
- [ ] **Medium:** Add readiness checks, persistent security audits, and core queue and outbox metrics.

## Enhancements

- [ ] **High:** Add real PostgreSQL and NATS integration tests.
  - Test duplicate delivery and every durable commit boundary.
- [ ] **High:** Generate and verify an SBOM from actual release artifacts.
- [ ] **Medium:** Store migration names and checksums, then reject changed applied migrations.
- [ ] **Low:** Replace custom field splitting and integer parsing with the Go standard library.
- [ ] **Low:** Keep one backoff helper and one allowed-path helper.
