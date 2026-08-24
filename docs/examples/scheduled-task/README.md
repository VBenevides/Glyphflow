# Schedule recurring work

This flow creates a recurring schedule for the fake `Customer Export Check`
task.

## Flow

1. Open **Schedules** and select **Create schedule**.
2. Select `Customer Export Check` and name the schedule `Nightly Customer Export`.
3. Set **UTC offset** to `0` and **Cron expression** to `0 2 * * *`.
4. Keep **Misfire policy** as `SKIP_ALL` and **Concurrency** as `QUEUE`.
5. Select **Preview next occurrences** and review the result.
6. Select **Save schedule version**.
7. Confirm the schedule list shows `ACTIVE`, the next fire time, and the linked task.

The schedule creates runs from the task's active immutable version. Use the
run history to inspect scheduled executions; do not use real schedules or
production destinations for this example.

## Screenshots

Store numbered screenshots for the editor, preview, saved schedule, and first
scheduled result in [`screenshots/`](screenshots/).
