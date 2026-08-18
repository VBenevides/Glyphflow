# V3 runner live test

Date: 2026-08-18 UTC  
Host: Linux `amd64` (`x86_64`)  
Control plane: one local Go process  
Workers: three local Linux/amd64 processes

## Setup

- PostgreSQL and NATS ran from `compose.yaml`.
- Workers were enrolled as `runner-v3-a` (capacity 1), `runner-v3-b` (capacity 1), and `runner-v3-c` (capacity 2).
- Created `v3-exclusive` (`exclusive`) and `v3-shared` (`non-blocking`).
- Tasks used [`sleep_task.py`](../../local/v3-runners/sleep_task.py), with durations passed as the command argument.
- Created a `* * * * *` UTC schedule for the scheduled-run case.

## Results

| Case | Evidence | Result |
|---|---|---|
| Multiple concurrent tasks in one runner | Two `v3-parallel` runs on `runner-v3-c`; starts `14:17:48.024` and `14:17:49.018`, both finish four seconds later | PASS |
| Exclusive resource across runners | `cross-a` on `runner-v3-a`: `14:18:14.019–18.019`; `cross-b` on `runner-v3-b`: `14:18:18.518–20.518` | PASS; second started after release |
| Exclusive resource in one runner | `same-b`: `14:18:53.017–54.017`; `same-a`: `14:18:54.518–58.518`, both on `runner-v3-c` | PASS; second started after release |
| Non-blocking resource in one runner | `shared-a`: `14:19:11.516–14.516`; `shared-b`: `14:19:13.518–16.518`, both on `runner-v3-c` | PASS; runs overlapped |
| Manual runs | All manual cases completed `SUCCEEDED` | PASS |
| Scheduled run | Fresh occurrence at `14:22:00` completed `SUCCEEDED` on `runner-v3-c` | PASS |

## Scheduler fix

The first schedule attempt exposed a blocker: an unrelated old due `QUEUE` schedule with an active run was selected first and returned without advancing, preventing later schedules from being examined. `backend/internal/store/schedules.go` now excludes schedules currently blocked by `QUEUE` or `ALLOW` concurrency limits from the selection query; their due cursor remains unchanged.

After restarting the control plane, the v3 schedule advanced and ran successfully. Three stale catch-up occurrences correctly recorded `Start Failure` because their 30-second start deadlines had already expired; the current occurrence passed.

## Checks

```text
python3 local/v3-runners/sleep_task.py 0.01       PASS
GOCACHE=/tmp/glyphflow-go-cache go test ./internal/store ./internal/controlplane   PASS
```

Final live state: all three v3 workers `ONLINE`, capacities `1/1/2`, active counts `0`; both resources free.

## Long resource stress test

Date: 2026-08-18 UTC
Workers: 24 Linux/amd64 headless workers, each capacity 1
Resources: 36 exclusive resources
Tasks: 32 manual runs, each sleeping 10 seconds with a 30-second timeout

The task matrix contained 12 one-resource tasks, 8 two-resource tasks, 4 three-resource tasks, and 8 six-resource tasks. Resource sets overlapped in chains, including repeated single-resource contention and shared resources at positions 2, 3, 6, 13, 16, 19, 22, 26, and 31.

| Check | Evidence | Result |
|---|---|---|
| Worker fan-out | 24/24 workers online; early snapshot had 17 `RUNNING` and 15 `WAITING` runs | PASS |
| Single-resource blocking | `single-01` used `r01` from `14:58:45.916–55.916`; `single-02` used the same resource from `14:58:56.270–14:59:06.270` on another runner | PASS; serialized |
| Two-resource overlap | `pair-01` (`r12+r13`) ran before `pair-02` (`r13+r14`); the second started at `14:58:56.260` after the first ended at `14:58:55.907` | PASS |
| Three-resource overlap | `triple-01` (`r24+r25+r26`) ran at `14:58:46`; `triple-02` sharing `r26` ran at `14:59:17` | PASS |
| Six-resource overlap | `six-06` (`r28..r33`) ran at `14:58:57`; later six-resource chains started only after overlapping leases were released | PASS |
| Full stress batch | 32/32 `SUCCEEDED`, 0 failed, 0 waiting; all 36 resources free and all 24 runners had active count 0 | PASS |

The batch completed in multiple waves without deadlock or an unreleased exclusive lease.
