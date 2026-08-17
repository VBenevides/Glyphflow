# Glyphflow v1 execution protocol

## Ownership and delivery

The scheduler is the only writer that creates a scheduled occurrence. It locks a due schedule, reads its active version, creates one logical run, and advances `next_fire_at` in one PostgreSQL transaction. Dispatch creates one attempt, its leases, and one execute-outbox row in one transaction.

Outbox producers publish the stored signed envelope with its message ID. They mark the row published only after NATS confirms it. Consumers insert the event in an inbox transaction, apply an idempotent state or log update, and acknowledge NATS only after commit. Duplicate message IDs are successful no-ops.

## Leases and placement

Placement chooses an enabled, non-draining pool member with matching capabilities and an active session; a pinned runner is an explicit exception. Lease acquisition locks each resource, expires an old active lease, increments its fencing counter, and creates the new lease atomically. Every order carries the lease token, fencing token, session, digest, and expiry.

## Runner recovery

Each runner boot has a unique boot ID. The consumer durably accepts an order before execution. On restart, unfinished claims from an older boot become `UNKNOWN` and emit a recovery event; they are never silently rerun. Orders for inactive sessions, expired leases, stale fencing tokens, wrong recipients, or wrong execution digests are rejected before persistence.

## Outcomes and races

Run and attempt transitions use compare-and-swap state versions. Retryable failures enter bounded backoff until the immutable task-version attempt limit is reached. An unproven external result becomes `UNKNOWN` and follows the task ambiguity policy (`RETRY`, `REQUIRE_MANUAL_RECONCILIATION`, or `MARK_FAILED`). Cancellation targets one run attempt, session, lease, and fencing token. A completion that commits first wins; a late cancel cannot affect a retry.

State events and log chunks have independent sequences. State events apply only when newer and legal; log chunks are independently deduplicated and bounded. External exactly-once side effects are not guaranteed, so cooperative tasks should use `run_id` as an idempotency key.
