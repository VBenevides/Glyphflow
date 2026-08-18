# V3 runner live test

Date: 2026-08-18 UTC  
Host: Linux `amd64` (`x86_64`)  
Control plane: one local Go process  
Workers: three local Linux/amd64 processes

## Setup

- PostgreSQL and NATS ran from `compose.yaml`.
- Workers were enrolled as `runner-v3-a` (capacity 1), `runner-v3-b` (capacity 1), and `runner-v3-c` (capacity 2).
- Created `v3-exclusive` (`exclusive`) and `v3-shared` (`non-blocking`).
- Tasks used [`sleep_task.py`](sleep_task.py), with durations passed as the command argument.
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
