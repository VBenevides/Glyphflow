# Glyphflow user guide

This guide covers the implemented Glyphflow web console: sign-in, runner
enrollment, task publication, execution, scheduling, and run recovery.

Start a local instance with the [quick start](README.md#quick-start). The local
console is available at <http://localhost:5173>. In a deployed environment,
use the URL provided by your administrator.

## Access and permissions

Glyphflow shows pages and actions according to your permissions. The main
permissions used in this guide are:

- `tasks.read` and `tasks.manage` for tasks and schedules;
- `runners.read` and `runners.manage` for runners and enrollment;
- `runs.read` and `runs.execute` for viewing and starting runs;
- `runs.cancel` and `runs.retry` for run actions; and
- `logs.read` for events and stdout/stderr logs.

If a page or button is missing, ask an administrator to check your role. Do not
use production credentials, enrollment binaries, tokens, or secret values in
examples, screenshots, task commands, or logs.

## Sign in

1. Open the Glyphflow console.
2. On **Sign in**, enter **Email** and **Password** when password sign-in is
   enabled.
3. Select **Sign in**.

When single sign-on is configured, the page also shows **Single sign-on** and a
**Continue with _provider_** button. Select the provider and complete the
provider's sign-in flow. You return to Glyphflow after the callback is
verified.

Self-registration is available only when enabled. If it is enabled, select
**Create an account**, enter **Email** and **Password**, then select
**Register**. Otherwise, an administrator must create or enable your account.

If no sign-in method is configured, the page shows **No sign-in methods are
configured. Contact an administrator.** A session-expiry response sends you
back to sign in.

## Enroll a runner

A runner is the machine that executes a task. Workers connect outbound to the
control plane; they do not need PostgreSQL credentials. See [worker
deployment](README.md#worker-deployment) and the [worker build
details](../backend/cmd/worker/README.md) for deployment-specific details.

### Create the enrollment

1. Open **Runners & Pools** and select **Enroll runner**.
2. Enter a **Runner name**.
3. Set **Control plane endpoint** if the target machine cannot reach the
   server's default endpoint. The target machine can override it with
   `GLYPHFLOW_CONTROL_PLANE_URL`.
4. Set **Embedded NATS Endpoint** when the target machine needs a different
   messaging endpoint. It can be overridden with
   `GLYPHFLOW_NATS_ENDPOINT`.
5. Select an enabled **Pool**.
6. Select **Platform** (`linux` or `windows`) and **Architecture**
   (`amd64`).
7. Set **Capacity**, the number of tasks the runner can accept concurrently.
8. Select **Worker UI**:
   - **GUI - Default UI**;
   - **TUI - Lower Memory Usage**; or
   - **Headless - Lowest Memory Usage**.
9. Select **Create enrollment**.

Glyphflow then shows **Runner binary ready**. Select **Download runner**, copy
the binary to the target machine, and run it once. The binary contains a
short-lived, one-use enrollment credential. Run it before the expiry displayed
in the console, and keep the downloaded file private.

The console shows **Waiting Enrollment** until the worker connects. When it
connects, the status changes to **Runner Enrolled** and the console returns to
the runners list after a short delay. If the artifact expires or is consumed,
create a new enrollment.

### Check runner readiness

On **Runners and pools**, inspect **Desired**, **Observed**, **Capacity**, and
**Heartbeat**. Select a runner to see its current runs, endpoints, and
lifecycle state. A runner must be online, enabled, and have capacity before it
can receive work.

## Create and publish a task

Tasks are versioned. A published version is immutable; editing a task creates a
new version.

1. Open **Tasks** and select **Create task**.
2. Enter a task **Name**.
3. Enter **Command arguments**, one argument per line. Glyphflow passes the
   arguments directly to the operating system; it does not parse them as a
   shell command.
4. Optionally set **Working directory**.
5. Select a **Runner pool**. Optionally select a specific **Runner**; the
   default **Any in Pool** lets placement choose an eligible runner.
6. Set **Execution Timeout Seconds**, **Maximum attempts**, and **Ambiguity
   policy**.
7. Optionally add **Environment variables**, **Resources**, and **Tags**.
   Tags are key/value requirements that must match runner capabilities, such as
   `os` = `linux`.
8. Select **Create task**.

For global values, use the `$ENV:VARIABLE_NAME` form where supported. When a
command contains such a reference, the editor displays a read-only **Final
Command** preview. Keep secret values out of command text and logs.

To publish a change:

1. Open the task from the **Tasks** list.
2. Select **Edit version**.
3. Update the fields.
4. Select **Publish version**.

The task detail page shows **Active version** and **Version history**. The
active version is marked **(active)**. The task detail page also links to its
**Schedules**, **Runs**, and **Audit events**.

## Start a manual run

You can start a run from a task or from the runs list.

### From a task

1. Open the task detail page.
2. Select **Run now**.

### From the runs list

1. Open **Runs**.
2. Select **Start manual run**.

In **Start manual run**, select a **Task**, optionally enter **Reason (optional)**,
and select **Start run**. Manual execution uses the task's active
immutable version. After the run is created, open its run detail page to watch
it.

## Inspect a run

Open **Runs** and select a run ID. Use the filters **Task**, **Runner**,
**State**, **Trigger**, **From (UTC)**, and **To (UTC)**. The **Trigger** filter
distinguishes `SCHEDULE`, `MANUAL`, and `RETRY` runs.

The run detail page reports:

- **State**, **Attempt**, **Runner**, **Exit Code**, and **Exit Code Meaning**;
- maximum and average memory used, when reported;
- **Task version**, **Schedule version**, and **Audit events** under
  **Immutable references**;
- the **Attempt timeline**, including attempts, events, runner sessions,
  resource leases, cancellation details, and detected **Log gaps**; and
- the `stdout` and `stderr` panels when you have `logs.read`.

The log panels provide **Live View**, **Pause** or **Resume**, **Reconnect**,
and **Download source**. A terminal run stops its live stream. A reported log
gap is a signal to investigate the run history and worker connection.

Common run states include `WAITING`, `DISPATCHED`, `RUNNING`,
`RETRY_WAIT`, `CANCELLING`, `SUCCEEDED`, `FAILED`, `TIMED_OUT`,
`CANCELLED`, and `UNKNOWN`. For a `WAITING` run, read the placement
message shown above the run details.

## Schedule recurring work

1. Open **Schedules**.
2. Select **Create schedule**.
3. Select the **Task** and enter a unique **Name**.
4. Set the **UTC offset**. The editor accepts whole-hour offsets from `-23`
   to `+23`; `0` means UTC.
5. Enter a **Cron expression**. Fields are minute, hour, day, month, and
   weekday. For example, `*/5 * * * *` means every five minutes.
6. Select a **Misfire policy**: `SKIP_ALL`, `RUN_LATEST`, `RUN_ALL`,
   `RUN_UP_TO_N`, or `FAIL_AND_ALERT`. If you select `RUN_UP_TO_N`, set
   **Catch-up limit**.
7. Optionally set **Start deadline seconds**.
8. Select a **Concurrency** policy: `QUEUE`, `SKIP`, `REPLACE`, or
   `ALLOW`. If you select `ALLOW`, set **Max concurrent runs**.
9. Select **Preview next occurrences** and review **Next occurrences**.
10. Select **Save schedule version**.

Editing a schedule creates a new immutable schedule version. The schedule list
shows its **Next fire** time and **State**. The control-plane scheduler creates
due runs from the saved schedule; see the [architecture
document](../ARCHITECTURE.md#runtime-flow) for the execution path.

## Cancel, retry, or reconcile a run

Open a run and use the **Actions** section. Actions appear only when the run
state and your permissions allow them. Each action requires a reason.

### Cancel

**Cancel** is available for active or pending states, including `WAITING`,
`DISPATCHED`, `RUNNING`, `RETRY_WAIT`, and `CANCELLING`.

1. Select **Cancel**.
2. Read the warning in **Cancel task**.
3. Enter a **Reason**.
4. Select **Confirm task cancellation**. Select **Abandon task cancellation**
   to close the dialog without changing the run.

Refresh the run and inspect its cancellation details and final state. A cancel
request can be visible as `CANCELLING` while the worker processes it.

### Retry

**Retry** is available for `FAILED` and `TIMED_OUT` runs.

1. Select **Retry**.
2. Confirm that repeating the task's external effects is safe.
3. Enter a **Reason** and confirm the action.

The new attempt is represented by the run's attempt history and normally enters
`RETRY_WAIT` before placement.

### Reconcile an unknown run

**Reconcile unknown** is available for an `UNKNOWN` run. Use it only after
checking whether the original worker may have executed the command. The action
warns that the command can run again.

1. Select **Reconcile unknown**.
2. Review the external side effect risk.
3. Enter a **Reason** and confirm the action.

Reconciliation moves the run back into retry processing. Glyphflow provides
at-least-once delivery, not exactly-once command execution. Make commands
idempotent or otherwise safe to repeat before using retry or reconciliation.
Read the [delivery and security model](README.md#delivery-and-security-model)
for the recovery guarantees and boundaries.

## Further reading

- [Technical documentation](README.md)
- [Production deployment and configuration](README.md#production-deployment)
- [Architecture](../ARCHITECTURE.md)
- [Examples index](examples/README.md)
- Runtime API documentation at <http://localhost:8080/docs> and
  <http://localhost:8080/openapi.json> when the control plane is running.
