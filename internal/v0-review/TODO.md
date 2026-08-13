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
- [x] **High:** Replace presence-only security and release checks. `5c46925`, `0a25aa8`
  - Uncheck unsupported `internal/TODO.md` claims.
  - Fail release checks when the SBOM has no packages.
- [x] **Medium:** Upgrade `github.com/jackc/pgx/v5` to `v5.9.2` or newer. `752b2d3`
  - Run tests, vet, and `govulncheck` again.
- [x] **Medium:** Resolve symbolic links before allowed-root checks. `5049812`
- [x] **Medium:** Add HTTP timeouts and a bounded shutdown context. `4c206f2`

## Bugs

- [x] **High:** Set `cmd.Dir` to the validated worker directory. `f34f9f1`
  - Add one working-directory test.
- [x] **High:** Remove the non-atomic JSON worker store. `d0075a7`
  - Replace it with the planned SQLite transaction during worker wiring.
- [x] **High:** Enforce allowed state transitions in the compare-and-swap update. `48c9e9c`
- [x] **High:** Reject unsupported cron syntax and correct day-field behavior. `0a5b0ec`
- [x] **Medium:** Return `501` for task creation until it validates and stores tasks. `37006e7`
- [x] **Medium:** Serialize migrations with a PostgreSQL advisory transaction lock. `a086ba6`
- [x] **Medium:** Preserve structured command arguments during SQLite import. `7319f2c`
- [x] **Low:** Delete the no-op `Plane.Stop` method. `ff5e5e2`

## New Features

- [x] **Critical:** Implement one secure end-to-end task path. `b86edc5`
  - Create the run and outbox in one transaction.
  - Publish and durably accept one signed order.
  - Execute once and publish one signed final event.
  - Verify the event and update the run in one transaction.
- [x] **High:** Add the required JetStream publisher, consumers, acknowledgements, and dead-letter flow. `5eb9389`
- [x] **High:** Add guarded task, run, event, runner, cancellation, and retry endpoints as workflows become available. `eb5fd8c`
- [x] **High:** Add enrollment, certificate issue, key rotation, and revocation. `cee0026`, `cdd9e74`
- [x] **Medium:** Add readiness checks, persistent security audits, and core queue and outbox metrics. `ce291d9`

## Enhancements

- [x] **High:** Add real PostgreSQL and NATS integration tests. `85b84c5` (run with `-tags integration` and service URLs)
  - Test duplicate delivery and every durable commit boundary.
- [x] **High:** Generate and verify an SBOM from actual release artifacts. `1f900bf`, `e165844`
- [x] **Medium:** Store migration names and checksums, then reject changed applied migrations. `3a72f8a`
- [x] **Low:** Replace custom field splitting and integer parsing with the Go standard library. `fc48115`
- [x] **Low:** Keep one backoff helper and one allowed-path helper. `5062d14`
