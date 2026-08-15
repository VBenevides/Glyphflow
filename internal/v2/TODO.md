# Glyphflow prioritized TODO

Check an item only after its listed behavior has a runnable test.

## Security

- [ ] **High: Keep browser tokens out of JSON responses**
  - Return token-free login, refresh, and OIDC responses.
  - Test that only `HttpOnly` cookies contain tokens.

- [ ] **High: Remove signing and connection secrets from PostgreSQL config**
  - Load the control-plane private key and credentialed URLs from external secrets.
  - Test that a database dump contains no private key or connection password.

- [ ] **High: Authenticate runner key registration and heartbeats**
  - Bind the public key during one-use HTTPS enrollment.
  - Sign each later heartbeat and verify its boot ID, subject, and timestamp.

- [ ] **High: Bound all HTTP request bodies**
  - Apply one shared body limit before audit capture and JSON decoding.
  - Test oversized public and authenticated requests.

- [ ] **High: Make security configuration fail closed**
  - Reject malformed booleans.
  - Require secure transport unless an explicit development override is set.

- [ ] **Medium: Block OIDC server-side request forgery**
  - Reject private, loopback, and link-local targets.
  - Recheck redirect destinations.

- [ ] **Medium: Stop returning internal errors**
  - Map database failures to stable API codes.
  - Keep private details in correlated server logs.

- [ ] **Medium: Bound authentication rate-limit state**
  - Remove expired keys and cap stored entries.
  - Add a source-address limit.

- [ ] **Low: Return a public OIDC provider projection**
  - Expose only provider ID, display name, and public icon.

## Bugs

- [ ] **Critical: Implement real cancellation**
  - Persist the cancellation request.
  - Publish and execute a signed attempt-specific cancel order.
  - Test completion and cancellation races.

- [ ] **High: Make retry and reconciliation dispatch new attempts**
  - Apply attempt limits, backoff, and ambiguity policy.
  - Move eligible runs into the dispatcher claim path transactionally.

- [ ] **High: Repair worker restart recovery**
  - Persist the worker signing key.
  - Publish a signed `UNKNOWN` event for unfinished old-boot orders.
  - Verify queued old-key events after restart.

- [ ] **High: Enforce legal state-event order**
  - Lock the attempt and use `last_applied_state_sequence`.
  - Accept only newer legal transitions.

- [ ] **High: Complete the OIDC callback contract**
  - Use one callback path.
  - Verify the ID-token nonce against stored state.
  - Resolve confidential-client secret references.

- [ ] **High: Implement authenticated OIDC identity linking**
  - Add a link-purpose state flow or remove the dead UI action.

- [ ] **High: Repair the tagged integration suite**
  - Replace removed legacy store calls with canonical repositories.
  - Run PostgreSQL, TLS NATS, control-plane, worker, and restart checks.

- [ ] **Medium: Return complete task versions to the editor**
  - Include every stored active-version field.
  - Preserve unchanged values when creating a new version.

- [ ] **Medium: Honor collection filters and pagination**
  - Implement task, schedule, run, runner, and resource filters in SQL.
  - Test more than one page for each collection.

- [ ] **Medium: Correct dashboard data**
  - Filter due schedules and offline runners correctly.
  - Use totals or aggregate queries for metrics.

- [ ] **Medium: Make log URLs follow the API deployment contract**
  - Prefer same-origin routing for API, streams, downloads, and OIDC.
  - Remove the unused cross-origin option if same-origin remains required.

- [ ] **Medium: Make local user provisioning atomic**
  - Create the user, password, and initial role in one transaction.

- [ ] **Low: Return multiple schedule preview occurrences**
  - Return five deterministic occurrences.

## New Features

- [ ] **High: Execute task environment variables**
  - Show `Variable Name | Variable Value` rows with add and remove actions.
  - Validate names and include the environment map in the signed order.
  - Merge task values into the process environment and test override behavior.

- [ ] **High: Execute remaining immutable task specifications**
  - Apply selectors, secrets, retry policy, ambiguity policy, and resources.
  - Sign and verify the resulting execution digest.

- [ ] **High: Enforce schedule policies**
  - Apply misfire, catch-up, deadline, concurrency, and replacement rules in the due transaction.

- [ ] **High: Add a real run attempt timeline**
  - Return attempts, events, sessions, leases, cancellation, and log gaps.
  - Render them on run detail.

- [ ] **High: Add restart acceptance coverage**
  - Restart the control plane and worker while PostgreSQL and NATS remain active.
  - Verify sessions, execution, recovery, logs, and audit data.

- [ ] **Medium: Add global environment variables**
  - Add `global_variables` with validated names, values, RBAC, and audit events.
  - Resolve `$(VAR_NAME)` in supported task and schedule fields.
  - Suggest variable names with a native `datalist`.
  - Store references when task or schedule versions are published.
  - Block referenced deletion and renaming with a conflict response.
  - Snapshot resolved values or their digest when each run is created.
  - Reject unresolved variables and keep credentials in secret references.

- [ ] **Medium: Add dead-letter inspection and audited retry**
  - Store safe message metadata and rejection reasons.
  - Retry only after operator confirmation.

- [ ] **Medium: Add explicit schedule enable and disable actions**
  - Keep operational state separate from immutable schedule versions.

- [ ] **Medium: Wire retention and health metrics**
  - Clean sessions, OIDC state, inbox, outbox, and logs in bounded batches.
  - Expose low-cardinality scheduler, queue, runner, and recovery metrics.

## Enhancements

- [ ] **Medium: Remove production in-memory fallbacks**
  - Require repositories in production services.
  - Keep small repository fakes only in tests.

- [ ] **Medium: Add transaction and timeout conventions**
  - Pass request contexts through services.
  - Bound database operations and map PostgreSQL conflicts consistently.

- [ ] **Medium: Keep only cron schedules**
  - Convert or remove existing interval schedules before migration.
  - Drop `schedule_type` and remove interval code, UI, and tests.
  - Keep one cron parser and one cron-only contract test.

- [ ] **Medium: Hide inapplicable form fields**
  - Audit task, run, and schedule creation forms and dialogs.
  - Hide the runner selector until a runner pool is selected.
  - Show catch-up only for `RUN_UP_TO_N`.
  - Show maximum concurrency only for `ALLOW`.
  - Reset hidden values and omit them from requests.
  - Keep backend conditional validation authoritative.

- [ ] **Low: Consolidate route contract checks**
  - Test method, path, access class, and permission against runtime and OpenAPI.
  - Delete unused placeholder routes.

- [ ] **Low: Replace remaining raw task JSON fields with key/value rows**
  - Cover placement selectors and secret references.
  - Reuse native inputs and existing form components.

- [ ] **Low: Improve operator formatting**
  - Format dates and durations with `Intl`.
  - Store filters and page state in the URL.

- [ ] **Low: Keep the current React stack**
  - Add no new frontend framework now.
  - Reassess only when missing shared primitives slow repeated feature work.
