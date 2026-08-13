# Glyphflow v1 implementation TODO

Implement one checkbox at a time. Test it, commit it, and add the commit hash before starting the next checkbox.

## Security

- [ ] **Critical:** Define the signed execution protocol.
  - Choose one canonical, versioned binary encoding.
  - Define execute, cancel, state-event, and log-event messages.
  - Sign every required field from revision 3.
  - Reject unknown versions, types, algorithms, and keys.
  - Add golden vectors and cross-process verification tests.

- [ ] **Critical:** Enforce order freshness before runner persistence.
  - Validate signature, issuer, recipient, runner, and runner session.
  - Validate message expiry, lease expiry, lease token, and fencing token.
  - Validate the task version and execution digest.
  - Test stale execute and cancel orders after retry creation.

- [ ] **Critical:** Replace the static API token with user sessions.
  - Add a required access-token signing secret.
  - Reject weak secrets and unsupported signing algorithms.
  - Put the user ID and session ID in each access token.
  - Check the active user and session for each request.
  - Remove `API_AUTH_TOKEN` after bootstrap administration works.

- [ ] **Critical:** Enforce a permission classification on every route.
  - Mark each route as public, authenticated, or permission-protected.
  - Load permissions through role assignments and role permissions.
  - Return `401` for failed authentication.
  - Return `403` for missing permission.
  - Reject unclassified routes in one registry test.

- [ ] **Critical:** Protect the last active administrator transactionally.
  - Lock the administrator coordination row before a destructive change.
  - Reject removal of the final active administrator assignment.
  - Reject disabling the final active administrator.
  - Reject mutation or deletion of system roles.
  - Test concurrent administrator removals.

- [ ] **High:** Implement control-plane and runner signing-key lifecycle.
  - Store public keys and validity windows.
  - Keep private keys outside PostgreSQL.
  - Include `signing_key_id` in every signed message.
  - Support overlap during rotation.
  - Reject expired and revoked keys.

- [ ] **High:** Secure runner enrollment and certificates.
  - Generate each private runner key on the runner.
  - Store only the issued public certificate chain centrally.
  - Bind the certificate identity to the runner ID.
  - Revoke NATS access when the runner or key is revoked.
  - Test cross-runner subject denial.

- [ ] **High:** Implement Argon2id password storage.
  - Store one encoded hash in `user_passwords`.
  - Use a random salt and deployment pepper.
  - Set bounded memory, time, and parallelism parameters.
  - Add valid, invalid, malformed, and parameter-upgrade tests.

- [ ] **High:** Implement secure access and refresh sessions.
  - Generate opaque refresh tokens with a cryptographic random source.
  - Store only refresh-token hashes.
  - Rotate refresh tokens in one transaction.
  - Reject replay of the replaced token.
  - Test expiry, logout, logout-all, and user disablement.

- [ ] **High:** Protect browser sessions from cross-site requests.
  - Set authentication cookies as `HttpOnly`.
  - Require `Secure` cookies outside local development.
  - Set narrow paths and an explicit SameSite policy.
  - Check the origin and CSRF token for unsafe requests.
  - Test allowed and blocked cross-site requests.

- [ ] **High:** Implement strict generic OIDC validation.
  - Use Authorization Code flow with PKCE S256.
  - Validate state, nonce, signature, issuer, audience, and expiry.
  - Permit only configured callback URIs.
  - Set bounded discovery, JWKS, token, and user-info timeouts.
  - Do not add signature or issuer validation bypasses.

- [ ] **High:** Keep OIDC client secrets outside PostgreSQL.
  - Store one secret reference for each credential version.
  - Resolve the secret only during token exchange.
  - Redact references and resolved values from logs and audits.
  - Test missing, expired, and revoked credentials.

- [ ] **High:** Add authentication abuse controls.
  - Rate-limit password login by username and source address.
  - Rate-limit OIDC challenges and callbacks.
  - Keep login errors generic.
  - Add timing and rate-limit tests.

- [ ] **Medium:** Persist complete security audit events.
  - Audit login, refresh replay, logout, and session revocation.
  - Audit user, role, permission, SSO, and setting changes.
  - Audit runner enrollment, key rotation, and cancellation.
  - Store actor type, session, endpoint, target, result, before, and after values.

## Bugs

- [ ] **Critical:** Activate task and schedule versions explicitly.
  - Add same-parent current-version foreign keys.
  - Insert and activate each task version in one transaction.
  - Insert and activate each schedule version in one transaction.
  - Recalculate `next_fire_at` in the schedule-version transaction.
  - Test concurrent edits and rollback.

- [ ] **Critical:** Make due-schedule creation safe across replicas.
  - Lock due schedule rows with skip-locked selection.
  - Read the current schedule version under the lock.
  - Create each logical occurrence once.
  - Advance `next_fire_at` in the same transaction.
  - Test scheduler crash before and after commit.

- [ ] **Critical:** Implement schedule misfire, deadline, and concurrency rules.
  - Implement every policy defined in revision 3.
  - Keep start deadlines separate from execution timeouts.
  - Define `REPLACE` cancellation races.
  - Test downtime, backlog, overlap, and daylight-saving transitions.

- [ ] **Critical:** Implement legal run and attempt state transitions.
  - Encode both state machines from revision 3.
  - Use compare-and-swap updates with `state_version`.
  - Reject terminal-to-nonterminal transitions.
  - Test concurrent and out-of-order transitions.

- [ ] **Critical:** Make resource lease takeover deterministic.
  - Lock each resource row during acquisition.
  - Mark an expired active lease as `EXPIRED`.
  - Increment the resource fencing counter.
  - Create the new active lease in the same transaction.
  - Test concurrent acquisition and stale release.

- [ ] **Critical:** Make event processing idempotent and ordered.
  - Deduplicate each message by event ID.
  - Treat an already committed duplicate as success.
  - Sequence state events independently from log chunks.
  - Apply only a newer legal state event.
  - Acknowledge NATS after the database commit.

- [ ] **Critical:** Handle ambiguous execution as a distinct outcome.
  - Mark an unproven attempt as `UNKNOWN`.
  - Apply the immutable task ambiguity policy.
  - Never treat `UNKNOWN` as a normal retryable failure.
  - Test network loss before and after an external side effect.

- [ ] **Critical:** Make cancellation attempt-specific.
  - Record cancellation on the run and active attempt.
  - Send a cancel order for one attempt, session, lease, and fencing token.
  - Keep success when process completion wins the race.
  - Prevent an old cancel from affecting a retry.
  - Test cancellation at each attempt state.

- [ ] **High:** Recover runner orders safely after restart.
  - Bind each claim to the runner boot ID.
  - Mark unfinished orders from an older boot as unknown.
  - Do not automatically rerun an ambiguous local order.
  - Publish the recovery outcome through the event outbox.
  - Test crashes around claim, process start, and completion.

- [ ] **High:** Enforce one active runner session per runner.
  - Enforce unique runner and boot ID pairs.
  - Serialize active-session replacement.
  - Reject orders for an inactive session.
  - Test duplicate credentials and overlapping connections.

- [ ] **High:** Record reproducible execution metadata.
  - Compute a canonical execution-specification digest.
  - Include the digest in the signed order.
  - Verify it before process creation.
  - Store resolved secret version identifiers on the attempt.
  - Never store resolved secret values.

- [ ] **High:** Bound execution logs through every durable hop.
  - Enforce a maximum log chunk size.
  - Enforce each task version's total output limit.
  - Bound runner SQLite and NATS messages.
  - Define truncation and task-result behavior.
  - Test excessive stdout and stderr.

- [ ] **High:** Consume each OIDC authorization state atomically.
  - Match provider, purpose, expiry, and unused state.
  - Require one affected row.
  - Store only the state and nonce hashes.
  - Encrypt the PKCE verifier at rest.
  - Test two concurrent callbacks.

- [ ] **High:** Prevent user and platform authentication lockout.
  - Reject removal of a user's last login method.
  - Require password login or one enabled SSO provider.
  - Require a valid default role before user creation.
  - Validate settings in one transaction.

- [ ] **Medium:** Make role assignment uniqueness null-safe.
  - Require a canonical source key for every assignment.
  - Enforce unique user, role, source type, and source key tuples.
  - Test duplicate manual, system, and SSO assignments.

- [ ] **Medium:** Apply role changes without token renewal.
  - Keep permissions out of access tokens.
  - Resolve permissions for each request.
  - Test grant and revoke with an existing access token.

- [ ] **Medium:** Preserve manual roles during SSO synchronization.
  - Reconcile only the current provider's assignments.
  - Remove only obsolete SSO group assignments.
  - Test changed, duplicate, and missing group claims.

- [ ] **Medium:** Prevent unsafe SSO account matching.
  - Match SSO users only by provider and subject.
  - Require authenticated linking for an existing username or email.
  - Test account takeover with a shared email claim.

- [ ] **Low:** Normalize identity and authorization keys.
  - Canonicalize usernames, role keys, provider keys, and permission keys.
  - Add case-insensitive unique indexes for usernames and non-null emails.
  - Test mixed-case duplicates.

## New Features

- [ ] **Critical:** Write the formal execution protocol document.
  - Specify scheduler ownership and failover.
  - Specify dispatch and NATS acknowledgement boundaries.
  - Specify lease acquisition, renewal, expiry, and takeover.
  - Specify runner restart recovery.
  - Specify retry, ambiguity, cancellation, and event ordering.
  - Include every failure scenario from the external review.

- [ ] **Critical:** Add revision 3 control-plane migrations.
  - Create runner pools and pool membership.
  - Add version activation and schedule policy fields.
  - Add run, attempt, cancellation, sequencing, and recovery fields.
  - Add resource fencing and lease-state fields.
  - Add control-plane signing keys and normalized event tables.
  - Add every check, unique index, composite foreign key, and retention index.

- [ ] **Critical:** Replace the worker message table with revision 3 SQLite tables.
  - Create the order inbox and event outbox.
  - Configure WAL, FULL synchronization, and foreign keys.
  - Verify each pragma at startup.
  - Fail closed when SQLite is unavailable or corrupt.
  - Add migration and power-loss recovery tests.

- [ ] **Critical:** Add identity and SSO migrations.
  - Create users, passwords, sessions, roles, permissions, and assignments.
  - Create the singleton authentication settings row.
  - Create SSO providers, credentials, identities, states, and group mappings.
  - Add all revision 3 identity constraints and indexes.

- [ ] **Critical:** Seed permissions and system roles.
  - Derive stable UUIDs from keys.
  - Seed the report's permission catalog.
  - Seed immutable `admin` and `user` roles.
  - Grant every seeded permission to `admin`.
  - Leave `user` empty and never change custom roles.
  - Test repeated seed runs and changed descriptions.

- [ ] **Critical:** Add bootstrap administration.
  - Read one canonical bootstrap username from the environment.
  - Assign `admin` when that password or SSO user first appears.
  - Store the assignment source as `system`.
  - Refuse production startup without a bootstrap path or active administrator.
  - Test password-disabled SSO bootstrap.

- [ ] **High:** Implement dynamic runner placement.
  - Select enabled pool members with matching capabilities.
  - Exclude draining, disabled, offline, and full runners.
  - Honor an optional pinned runner.
  - Record the selected runner and active session on the attempt.
  - Test fair selection and runner loss.

- [ ] **High:** Implement the dispatch transaction and producer.
  - Create the attempt, leases, and execute outbox row together.
  - Publish pending outbox rows with the message ID.
  - Mark publication after broker confirmation.
  - Retry without creating another attempt.
  - Test a crash at each commit boundary.

- [ ] **High:** Implement retry and run aggregation.
  - Classify exit codes and termination reasons from the task version.
  - Calculate bounded backoff.
  - Enforce maximum attempts.
  - Update the logical run from attempt outcomes.
  - Test retryable, terminal, exhausted, and unknown outcomes.

- [ ] **High:** Add authentication and authorization stores.
  - Add transaction-safe user, password, session, role, and permission queries.
  - Add source-aware role assignment queries.
  - Add SSO provider, credential, identity, state, and mapping queries.
  - Keep authentication transactions short.

- [ ] **High:** Add password authentication endpoints.
  - Add registration when enabled.
  - Add username and password login.
  - Add refresh, logout, and logout-all.
  - Assign the default role in the user creation transaction.
  - Return no credential or token hashes.

- [ ] **High:** Add generic OIDC endpoints.
  - List enabled login providers.
  - Create login and authenticated link challenges.
  - Complete callbacks and account linking.
  - Enforce auto-provision and last-login-method rules.
  - Create identity, roles, and session in one transaction.

- [ ] **High:** Add authentication administration endpoints.
  - Add protected authentication setting updates.
  - Add SSO provider, credential, and group-mapping operations.
  - Add user status and session revocation operations.
  - Require the report's permission keys.

- [ ] **High:** Add custom role and assignment endpoints.
  - List roles, permissions, assignments, and effective grants.
  - Create, update, and delete non-system roles.
  - Replace a custom role's permission set transactionally.
  - Add and remove manual assignments.
  - Keep system and SSO assignments read-only through manual endpoints.

- [ ] **High:** Apply permissions to Glyphflow endpoint groups.
  - Apply task permissions to tasks and schedules.
  - Apply run permissions to execute, read, cancel, and retry.
  - Apply log, resource, runner, and audit permissions.
  - Keep only health and authentication entry routes public.

- [ ] **Medium:** Add current-user endpoints.
  - Return the current profile, roles, and effective permissions.
  - Return linked SSO identities and active sessions.
  - Allow a user to revoke an owned session.

- [ ] **Medium:** Add SSO group-role synchronization.
  - Read configured group claim paths.
  - Map external groups to local roles.
  - Reconcile source-aware assignments after each SSO login.
  - Audit each changed assignment.

## Enhancements

- [ ] **Critical:** Add distributed failure-injection tests.
  - Start real PostgreSQL and NATS services.
  - Kill each scheduler, dispatcher, consumer, producer, and runner at commit boundaries.
  - Test duplicate and out-of-order delivery.
  - Test partitions, lease takeover, stale orders, and stale cancellation.
  - Verify one logical occurrence and documented ambiguity behavior.

- [ ] **High:** Add scheduler policy integration tests.
  - Test each misfire, catch-up, deadline, and concurrency policy.
  - Test two scheduler replicas.
  - Test daylight-saving gaps and repeated local times.
  - Test task and schedule edits while runs wait.

- [ ] **High:** Add end-to-end authentication integration tests.
  - Start PostgreSQL and a local OIDC test provider.
  - Test password-only, SSO-only, and mixed modes.
  - Test custom roles and immediate permission changes.
  - Test seed idempotency, system roles, CSRF, and account linking.

- [ ] **High:** Add route authorization coverage tests.
  - Enumerate every route and method.
  - Require a public classification or permission key.
  - Test one allowed and one denied request for each permission group.

- [ ] **Medium:** Add retention workers.
  - Delete expired sessions and consumed SSO states in bounded batches.
  - Delete committed inbox and outbox rows after their retention period.
  - Delete old log chunks according to policy.
  - Retain security and execution audit events.

- [ ] **Medium:** Add low-cardinality operational metrics.
  - Measure scheduler lag, dispatch lag, active leases, and unknown attempts.
  - Measure inbox duplicates, outbox retries, and rejected stale orders.
  - Measure login failure, refresh replay, and permission denial.
  - Do not use identity or run IDs as metric labels.

- [ ] **Medium:** Document the reliability and recovery contract.
  - State the logical-run guarantee and at-least-once delivery behavior.
  - State the external exactly-once and fencing limitations.
  - Document manual reconciliation for unknown outcomes.
  - Add operator procedures for runner loss, NATS loss, and database loss.

- [ ] **Low:** Document authentication deployment modes.
  - Document password-only, SSO-only, and mixed modes.
  - Document bootstrap administrator recovery.
  - Document provider secret references and callback URLs.
  - Document cookie, proxy, origin, CSRF, and TLS requirements.

## Deferred after v1

- [ ] Add shared or capacity-based resources after a concrete workload requires them.
- [ ] Add refresh-token families when incident response needs token lineage.
- [ ] Move log bodies to object storage when measured volume exceeds PostgreSQL limits.
- [ ] Add service accounts when a machine API requires non-user identity.
- [ ] Add notifications, approvals, and runner telemetry when product requirements include them.
