# Architecture decisions

## Protocol

Orders and events use a small signed envelope. The signature covers the exact
payload bytes and a fixed message domain. Ed25519 is the only signing
algorithm. Workers verify the envelope before parsing or executing a payload.

## Database

PostgreSQL is the control plane source of truth. Task-run state, leases, audit
events, inbox records, and dispatch outbox records are committed together when
they describe one state transition. Workers receive no PostgreSQL credentials
or client library.

## Queue

NATS JetStream provides durable, at-least-once delivery for signed orders and
worker events. PostgreSQL outbox records and worker SQLite outbox records keep
queue publication recoverable after a restart or outage.

## Security

Workers make outbound-only connections. Mutual TLS authenticates queue peers,
Ed25519 signatures authenticate messages, and worker private keys are generated
on the target machine. Commands use argument arrays and are constrained by
configured paths, identities, resource limits, and output limits.
