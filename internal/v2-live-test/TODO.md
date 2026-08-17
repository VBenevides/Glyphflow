# v2 Live-Test TODO

## Features

- [ ] Add a dispatched run state
  - Importance Level: High
  - Description: Show `DISPATCHED` after the control plane sends a run to a worker and before the worker reports that it started. Keep the state distinct from `RUNNING`.
  - Test Description: Run a task against a delayed worker and verify the API and UI show `DISPATCHED` until a start event arrives.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Detect worker start failures
  - Importance Level: High
  - Description: Set the default maximum start delay to one minute. If a dispatched task does not start within the configured schedule limit, mark it `Start Failure` and use system error code 5.
  - Test Description: Dispatch a task without a start event, verify the one-minute default and configured delay, then verify `Start Failure` and error code 5.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Enforce maximum task execution time
  - Importance Level: High
  - Description: Rename the task timeout setting to `Max task execution time`. Cancel tasks that exceed it and report status `Timeout` with system error code 6.
  - Test Description: Run a task longer than its configured limit and verify cancellation, `Timeout`, error code 6, and the updated task label.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Add task resources table
  - Importance Level: High
  - Description: Add a task-editor table like Tags that selects resources from the resource list and identifies exclusive resources.
  - Test Description: Add exclusive and non-exclusive resources to a task, verify their labels in the editor, save the task, and verify dispatch uses the selected resources.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Add manual task execution
  - Importance Level: Medium
  - Description: Add an action in Task View to run the task manually.
  - Test Description: Trigger a manual run from Task View and verify one run is created and enters the normal execution lifecycle.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Add task version diffs
  - Importance Level: Medium
  - Description: Allow users to select a task version numbered 2 or later and compare it with the previous version.
  - Test Description: Create two task versions, open the second version, and verify the diff shows changes against version 1.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Improve worker terminal context
  - Importance Level: Medium
  - Description: Prefix each worker-terminal line with `>` and show task name and version before the task ID.
  - Test Description: Execute a task and verify stdout/stderr entries contain the prefix, task name, version, and task ID in that order.
  - Test Result: Not run
  - Commit Hash: Not committed

- [x] Reduce live log flush interval
  - Importance Level: Medium
  - Description: Reduce the worker log flush interval from 30 seconds to 10 seconds so live run logs reach NATS and storage sooner.
  - Test Description: Run a task that emits output for more than 10 seconds and verify log chunks are published during execution rather than waiting 30 seconds.
  - Test Result: Passed with `GOCACHE=/tmp/glyphflow-go-cache go test ./internal/worker`.
  - Commit Hash: `91473217aba8d91fd3cd5789a955357b338140f1`

- [ ] Remove Neon theme
  - Importance Level: Low
  - Description: Remove the Neon theme option and its associated UI styling and state.
  - Test Description: Verify the appearance controls and rendered application contain only the remaining themes.
  - Test Result: Not run
  - Commit Hash: Not committed

## Security Patches

No security items were identified in `ERRORS.md`.

## Bug Fixes

- [ ] Fix unreadable Light Theme logs
  - Importance Level: High
  - Description: Fix Light Theme stdout/stderr colors so log text remains readable against its background.
  - Test Description: Open stdout and stderr panels in Light Theme and verify sufficient text/background contrast and readable log content.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Hide the Windows command window
  - Importance Level: Medium
  - Description: Prevent the Windows worker from opening a visible black CMD window while a task starts.
  - Test Description: Run a Windows worker task and verify no command window appears while execution and worker logging continue.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Prevent duplicate task cancellation actions
  - Importance Level: High
  - Description: Make one task-cancel action produce one cancellation request and one visible cancellation result.
  - Test Description: Cancel a running task once and verify the UI, API request, and run history show only one cancellation.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] Restore collapsed sidebar access
  - Importance Level: Medium
  - Description: Make the collapsed sidebar reopenable and align the user area at the bottom of the sidebar.
  - Test Description: Collapse and reopen the sidebar at supported viewport sizes and verify the toggle works and the footer is aligned.
  - Test Result: Not run
  - Commit Hash: Not committed
