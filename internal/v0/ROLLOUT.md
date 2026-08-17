# Migration rollout

Migrate one task group at a time. A routing flag selects either the local
scheduler or Glyphflow; both must never execute the same definition. Import
definitions, compare next occurrences, register one canary worker, and observe
duplicates, state recovery, logs, metrics, and restarts before expanding.

Rollback disables the Glyphflow route, waits for in-flight leases to reach a
safe state, and re-enables the local scheduler. Retire local queue access only
after the final group is verified.
