# Place work on the right runner

This flow demonstrates pool, capability, and resource-aware placement with the
fake `Warehouse Snapshot` task.

## Flow

1. Open the `Warehouse Snapshot` task and confirm its active version uses the
   `Northstar Linux` pool.
2. Confirm its tags require `platform=linux` and `architecture=amd64`.
3. Confirm it requires the exclusive `warehouse-report-lock` resource.
4. Start a manual run.
5. Open the run details and inspect the selected runner and resource lease.
6. Confirm the run executes on `northstar-runner-01` only while the runner is
   online, enabled, capable, and below capacity.

## Screenshots

![Real white-mode console screenshot of runner placement with fake data](screenshots/01-placement-lease.png)

This screenshot was captured from the local Glyphflow console and shows the
fake task's `Northstar Linux` pool and online runner. The resource lease is
visible after the run is dispatched.
