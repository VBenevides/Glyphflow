# Development profile

This profile is for local development and disposable CI runs only. It is a
best-effort starting point, not an SLA, production capacity claim, security
boundary, backup policy, or release acceptance result. Record observations;
do not publish them as guarantees.

## Provisional values

| Area | Development target |
| --- | --- |
| Topology | One control plane, one SQLite database, embedded NATS, and one worker |
| Host budget | 4 vCPU, 8 GiB RAM, and at least 20 GiB free persistent disk |
| Runner | 1 runner with capacity 1 and 1 active execution |
| Throughput | 10 short task completions per minute, best effort |
| API latency | p95 below 2 seconds for one local user, best effort |
| Restart / queue drain | 60 / 120 seconds as local observation targets |
| Availability / RPO / RTO | No target or guarantee; record observed values only |
| Message / output size | 1 MiB each (`MAX_MESSAGE_BYTES` and inherited worker output limit) |
| Run-log download | 1,000 chunks or 8 MiB; HTTP write timeout is 30 seconds |
| Dead-letter retry | 1-second initial delay, at most 10 attempts, exponential delay capped at 128 seconds |
| Run and log retention | 3 calendar months |
| Audit retention | 12 calendar months |
| Session retention | 14 days |
| Published worker events | 24 hours after successful publication; pending events remain recoverable |
| Storage pressure | Warning at 20%, critical at 10%, emergency at 5% free; cleanup targets 15% |
| Operational alerts | Queue lag 30s/5m, open dead letters 1/10, oldest dead letter 5m/30m, stuck runs 1/10 |

The direct local profile does not enforce the host or service budget. The
resource rows are sizing targets for local tests. The configured data path must
remain disposable; `.dev-data/controlplane.sqlite` and `.dev-data/nats` hold
control-plane state, while each worker has its own `runner.sqlite`. No
retention claim is made for NATS data, backups, or release evidence.

These values provide repeatable development inputs for OQ-003, OQ-004, OQ-010,
OQ-015, and OQ-016. They do not close those production questions or unblock
production rollout acceptance.
