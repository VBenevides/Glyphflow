# Reliability and recovery contract

Glyphflow creates one durable logical run for each scheduled occurrence accepted by the schedule transaction. NATS delivery is at least once; inbox and outbox IDs make duplicate delivery safe. External side effects are not exactly once. Cooperative tasks should use `run_id` as an idempotency key.

Fencing tokens protect only systems that enforce them. An unproven process result is `UNKNOWN`; the immutable task policy selects retry, manual reconciliation, or failure. Operators must reconcile unknown attempts before repeating non-idempotent work.

If a runner is lost, wait for lease expiry, inspect its last heartbeat, and reconcile unknown attempts. If NATS is lost, leave outbox rows pending and restart producers; do not create a second attempt. If PostgreSQL is lost, stop schedulers and consumers, restore the database, then replay pending outbox/inbox work after migrations complete.
