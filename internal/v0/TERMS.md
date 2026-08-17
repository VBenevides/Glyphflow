# Glyphflow terms

| Term | Meaning |
|---|---|
| Control plane | The Go service that owns PostgreSQL state, schedules runs, dispatches signed orders, verifies worker events, and serves the HTTP API. |
| Producer | The control-plane loop that turns due schedules or manual requests into task runs and dispatch outbox records. |
| Worker | The standalone Go process on a target VM that verifies orders, executes commands, and publishes signed events. |
| Order | A control-plane-signed instruction for a worker, such as execution or cancellation. |
| Event | A worker-signed lifecycle message reporting acceptance, progress, or a final result. |
| Task run | One execution occurrence of a task definition, including its assignment, attempt, lease, state, and events. |
