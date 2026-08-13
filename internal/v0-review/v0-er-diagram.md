# Glyphflow v0 ER diagram

This diagram describes the storage that the current backend creates. PostgreSQL holds control-plane state. Each worker has a separate SQLite database.

## PostgreSQL control plane

```mermaid
erDiagram
    SCHEMA_MIGRATIONS {
        integer version PK
        text name
        text checksum
        timestamptz applied_at
    }

    TASK_DEFINITIONS {
        text id PK
        text name UK
        text schedule
        text timezone
        integer version
        jsonb command
        jsonb selectors
        jsonb resources
        jsonb retry_policy
        boolean enabled
        timestamptz next_due_at "nullable"
        timestamptz created_at
        timestamptz updated_at
    }

    TASK_RUNS {
        text id PK
        text task_definition_id FK
        timestamptz occurrence_at
        text runner_id "nullable; logical RUNNERS link"
        text state
        integer attempt
        text lease_token "nullable"
        timestamptz queued_at
        timestamptz started_at "nullable"
        timestamptz finished_at "nullable"
        jsonb result "nullable"
        bigint state_version
        timestamptz created_at
        timestamptz updated_at
    }

    RUN_EVENTS {
        text event_id PK
        text task_run_id FK
        integer attempt
        bigint sequence
        text event_type
        jsonb payload
        timestamptz accepted_at
    }

    RUNNERS {
        text id PK
        text pool
        integer capacity
        jsonb capabilities
        text state
        timestamptz heartbeat_at "nullable"
        timestamptz created_at
        timestamptz updated_at
    }

    RUNNER_KEYS {
        text key_id PK
        text runner_id FK
        bytea public_key
        timestamptz not_before
        timestamptz not_after "nullable"
        timestamptz revoked_at "nullable"
        timestamptz created_at
    }

    RUNNER_ENROLLMENTS {
        text id PK
        text runner_id FK
        bytea token_hash
        timestamptz expires_at
        timestamptz used_at "nullable"
        text requester
        text target
        jsonb artifact
        timestamptz created_at
    }

    RESOURCE_LEASES {
        text id PK
        text resource_key
        text task_run_id FK
        text lease_token
        timestamptz expires_at
        timestamptz released_at "nullable"
        timestamptz created_at
    }

    DISPATCH_OUTBOX {
        text id PK
        text task_run_id FK
        bytea order_bytes
        text subject
        text state
        integer attempts
        timestamptz available_at
        timestamptz published_at "nullable"
        text last_error "nullable"
        timestamptz created_at
    }

    EVENT_INBOX {
        text event_id PK
        text task_run_id FK
        integer attempt
        timestamptz received_at
    }

    TASK_DEFINITIONS ||--o{ TASK_RUNS : creates
    TASK_RUNS ||--o{ RUN_EVENTS : records
    TASK_RUNS ||--o{ RESOURCE_LEASES : owns
    TASK_RUNS ||--o{ DISPATCH_OUTBOX : dispatches
    TASK_RUNS ||--o{ EVENT_INBOX : receives
    RUNNERS ||--o{ RUNNER_KEYS : authenticates_with
    RUNNERS ||--o{ RUNNER_ENROLLMENTS : enrolls_with
    RUNNERS o|--o{ TASK_RUNS : logical_assignment_no_FK
    RUN_EVENTS o|--o| EVENT_INBOX : logical_event_id_no_FK
```

`SCHEMA_MIGRATIONS` is independent. The migration runner creates it before it applies the versioned schema.

## Worker SQLite

```mermaid
erDiagram
    MESSAGES {
        text id PK
        blob value
    }
```

`MESSAGES.id` stores an accepted order ID. `MESSAGES.value` stores its JSON state. The table has no relational links because each worker database is local.

## Constraints and logical links

- `TASK_RUNS` permits one occurrence per `(task_definition_id, occurrence_at)`.
- `RUN_EVENTS` permits one event per `(task_run_id, attempt, sequence)`. A trigger makes the table append-only.
- `RUNNER_ENROLLMENTS` permits one unused enrollment per runner.
- `RESOURCE_LEASES` permits one unreleased lease per resource key.
- `TASK_RUNS.runner_id` logically refers to `RUNNERS.id`, but PostgreSQL does not enforce this relation.
- `EVENT_INBOX.event_id` logically matches `RUN_EVENTS.event_id`, but PostgreSQL does not enforce this relation.
- Task run states are `queued`, `dispatched`, `accepted`, `running`, `completed`, `failed`, `timed_out`, `cancelled`, and `lost`.
- Runner states are `enrolling`, `active`, `offline`, `disabled`, and `revoked`.
- Run event types are `accepted`, `rejected`, `started`, `heartbeat`, `completed`, `failed`, `timed_out`, and `cancelled`.
- Dispatch outbox states are `pending`, `published`, and `failed`.

The audit JSON file, replay log, NATS JetStream messages, and imported legacy SQLite tables are not relational application entities. The current command entry points also do not connect the PostgreSQL schema to the HTTP server or worker process.
