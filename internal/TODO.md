# Glyphflow implementation roadmap

This roadmap converts the local scheduler into a secure network orchestration platform.

The source is the local script scheduler's Network Scheduler Migration Report. All items remain unchecked until implementation and verification are complete.

## Project outcome

Glyphflow will have one central control plane and many workers on remote virtual machines.

The control plane will own all central data and task state changes. Workers will receive signed orders and return signed events.

PostgreSQL will be the source of truth. NATS JetStream will provide durable, at-least-once message delivery.

## Phase 0: Project foundation

| Task | Description | Impact |
|---|---|---|
| [ ] Confirm the MVP boundary | Limit the MVP to one control plane, one queue, and remote workers. | Prevents premature service splitting and reduces the first release risk. |
| [ ] Define project terms | Use `control plane`, `producer`, `worker`, `order`, `event`, and `task run` consistently. | Prevents protocol and documentation conflicts. |
| [ ] Define repository layout | Reserve `backend/` for Go and `frontend/` for the TypeScript and React application. | Gives contributors a stable project structure. |
| [ ] Create the Go backend module | Create one Go module for shared protocol code, the control plane, and the worker. | Allows both executables to share tested message types. |
| [ ] Create the default TypeScript and React frontend | Use the standard React TypeScript template without extra application frameworks. | Provides a familiar frontend with low maintenance cost. |
| [ ] Add local development services | Define local PostgreSQL and NATS JetStream services. | Makes development and integration tests reproducible. |
| [ ] Add configuration validation | Validate required addresses, paths, limits, and identities during startup. | Stops unsafe or incomplete deployments before work starts. |
| [ ] Record architecture decisions | Document protocol, database, queue, and security decisions. | Keeps future changes consistent with the system model. |
| [ ] Define the supported platform matrix | Select the first Linux and Windows architectures for workers. | Sets clear build, installer, and test requirements. |

Exit condition: contributors can start the empty backend, frontend, PostgreSQL, and JetStream development environment.

## Phase 1: Shared protocol

| Task | Description | Impact |
|---|---|---|
| [ ] Define the outer envelope | Add `key_id`, base64 payload bytes, and a base64 Ed25519 signature. | Gives orders and events one verifiable transport format. |
| [ ] Sign exact payload bytes | Sign stored payload bytes instead of rebuilt JSON. | Removes canonical JSON differences from signature verification. |
| [ ] Add signature domains | Use separate fixed domains for orders and events. | Prevents an event signature from authorizing an order. |
| [ ] Fix the signature algorithm | Accept Ed25519 only and ignore untrusted algorithm choices. | Prevents algorithm downgrade and confusion attacks. |
| [ ] Define the order payload | Include identifiers, version, attempt, lease, times, runner, command arguments, timeout, secrets, limits, and resources. | Gives workers all required execution data without database access. |
| [ ] Define the event payload | Include identifiers, type, attempt, lease, runner, sequence, time, result, metrics, digest, and bounded errors. | Gives the control plane enough data to validate each state change. |
| [ ] Define order types | Support execution and cancellation as signed order types. | Prevents cancellation from using an unverified side channel. |
| [ ] Define event types | Support accepted, rejected, started, heartbeat, completed, failed, timed out, and cancelled events. | Creates a complete task lifecycle. |
| [ ] Verify before parsing payloads | Parse only the small envelope before signature verification. | Reduces exposure to forged payloads. |
| [ ] Add message size limits | Reject oversized envelopes before base64 decoding or JSON parsing. | Controls memory use and denial-of-service risk. |
| [ ] Add protocol version checks | Reject unsupported protocol versions and message types. | Makes incompatible upgrades fail safely. |
| [ ] Add time validation | Validate issue, not-before, expiration, observation, and clock tolerance values. | Limits replay and catches clock problems. |
| [ ] Add identity validation | Check the assigned runner, run identifier, attempt, and lease token. | Stops one worker from applying another worker's work. |
| [ ] Add sequence validation | Require the expected event sequence for each attempt. | Stops skipped or reordered state transitions. |
| [ ] Add golden protocol vectors | Store cross-executable signing and verification examples. | Detects accidental wire-format changes. |
| [ ] Test altered payload rejection | Change one payload byte and require verification failure. | Proves message integrity. |
| [ ] Test wrong-key and revoked-key rejection | Reject unknown, invalid, expired, or revoked signing keys. | Proves key policy enforcement. |
| [ ] Test replay and wrong-runner rejection | Reuse identifiers and change the assigned runner in test messages. | Proves the main authorization checks. |
| [ ] Test key rotation overlap | Accept old and new valid keys during a defined rotation window. | Allows rotation without message loss. |

Exit condition: both executables reject altered, expired, duplicate, and wrong-runner messages.

## Phase 2: PostgreSQL state

| Task | Description | Impact |
|---|---|---|
| [ ] Add versioned database migrations | Apply ordered PostgreSQL schema changes during deployment. | Makes database upgrades repeatable and reviewable. |
| [ ] Create `task_definitions` | Store schedules, versions, commands, time zones, selectors, resources, and retry policy. | Creates the central task configuration. |
| [ ] Create `task_runs` | Store each occurrence, assignment, state, attempt, lease, timestamps, and result. | Creates the authoritative execution state. |
| [ ] Prevent duplicate occurrences | Add a unique constraint for each task definition and scheduled occurrence. | Stops multiple producer replicas from creating the same run. |
| [ ] Create `run_events` | Store accepted worker events as an append-only audit history. | Preserves the complete task lifecycle. |
| [ ] Enforce event uniqueness | Add unique constraints for event identifiers and run-attempt sequences. | Makes duplicate events harmless. |
| [ ] Create `runners` | Store worker identity, pool, capacity, capabilities, state, and heartbeat time. | Allows safe worker selection and health reporting. |
| [ ] Create `runner_keys` | Store public keys, key identifiers, validity periods, and revocation data. | Supports signature verification and key lifecycle operations. |
| [ ] Create `runner_enrollments` | Store token hashes, expiry, use, requester, target, and artifact data. | Supports one-use worker enrollment without stored plain tokens. |
| [ ] Limit unused enrollments | Allow only one unused enrollment for each runner. | Prevents multiple valid installers for one identity. |
| [ ] Create `resource_leases` | Store resource ownership, task run, lease token, and expiration. | Prevents conflicting tasks from using exclusive resources. |
| [ ] Create `dispatch_outbox` | Store exact signed order bytes, subjects, and publication state. | Prevents database commits from losing queue publication. |
| [ ] Create `event_inbox` | Store received event identifiers in the state update transaction. | Prevents duplicate events from changing state twice. |
| [ ] Add state constraints | Restrict task runs and runners to documented states. | Stops invalid values outside application code. |
| [ ] Add optimistic or locked updates | Use a state version or row lock for each task state change. | Prevents concurrent event races. |
| [ ] Add retention indexes | Index due tasks, pending outbox rows, recent runs, heartbeats, and audit queries. | Keeps core operations efficient as history grows. |
| [ ] Add SQLite import tooling | Import existing task definitions from the local scheduler database. | Preserves current schedules during migration. |
| [ ] Test transactional run creation | Create the task run, leases, and outbox row in one transaction. | Proves that no order exists without durable central state. |

Exit condition: one transaction creates a unique task run, its resource leases, and its outbox record.

## Phase 3: Go control plane and producer

| Task | Description | Impact |
|---|---|---|
| [ ] Create the control plane executable | Run the API, scheduler, dispatcher, event ingester, housekeeping, and notifications in one process. | Delivers the MVP without unnecessary services. |
| [ ] Add graceful shutdown | Stop HTTP, database, queue, and background loops with bounded waits. | Prevents lost in-process work during deployments. |
| [ ] Implement schedule calculation | Support manual, fixed-time, and cron schedules with explicit time zones. | Preserves local scheduler behavior in the central producer. |
| [ ] Lock due occurrences | Use PostgreSQL row locks so one producer claims each due occurrence. | Allows multiple control plane replicas without duplicate runs. |
| [ ] Select active workers | Match runner selectors, status, capabilities, lease, and available capacity. | Sends work only to eligible workers. |
| [ ] Reserve exclusive resources | Create resource leases in the task run transaction. | Prevents conflicting tasks from starting together. |
| [ ] Create task runs transactionally | Store the occurrence, assignment, lease, and outbox record together. | Keeps task state and delivery state consistent. |
| [ ] Store signed envelopes before publication | Create and retain the exact order envelope in the outbox. | Makes retries publish identical signed bytes. |
| [ ] Dispatch with `SKIP LOCKED` | Let control plane replicas share pending outbox work. | Adds safe horizontal dispatch capacity. |
| [ ] Mark publication after confirmation | Update outbox state only after JetStream confirms the publish. | Prevents silent order loss. |
| [ ] Verify worker events | Validate the certificate context, signature, assignment, attempt, lease, and sequence. | Stops forged or stale events from changing task state. |
| [ ] Apply allowed transitions only | Implement the documented task run state machine. | Prevents workers from selecting arbitrary states. |
| [ ] Commit before queue acknowledgement | Store the inbox record, event, state change, and lease release before acknowledgement. | Makes event processing recoverable. |
| [ ] Keep old-attempt events for audit | Record stale events without changing the current attempt. | Preserves evidence while protecting current state. |
| [ ] Add runner lease recovery | Mark workers offline and runs lost after defined safety intervals. | Gives operators accurate failure state. |
| [ ] Release resources on final states | Release leases only after a verified final state. | Prevents early resource reuse. |
| [ ] Add housekeeping | Remove expired audit data and abandoned leases by policy. | Controls storage use and stale reservations. |
| [ ] Add final-state notifications | Send email only after the control plane verifies a final event. | Keeps SMTP credentials away from workers. |
| [ ] Add bounded retry backoff | Retry transient database and queue operations with limits and jitter. | Avoids overload during outages. |

Exit condition: the control plane produces, dispatches, ingests, and records one task run without direct worker database access.

## Phase 4: HTTP API and user access

| Task | Description | Impact |
|---|---|---|
| [ ] Define the versioned HTTP API | Add a stable API prefix and documented JSON error format. | Gives the frontend a predictable contract. |
| [ ] Add user authentication | Require a verified user identity for every management endpoint. | Prevents anonymous control of workers and tasks. |
| [ ] Add role-based authorization | Separate task, runner, enrollment, key, and audit permissions. | Limits damage from compromised user accounts. |
| [ ] Add task definition endpoints | Create, read, update, enable, disable, and version task definitions. | Supports the main frontend workflow. |
| [ ] Add manual run endpoints | Create immediate task runs through the same producer path. | Avoids a second execution path. |
| [ ] Add run history endpoints | Filter runs by task, runner, state, attempt, and time. | Supports operations and incident review. |
| [ ] Add run event endpoints | Return the ordered audit timeline for one run. | Shows why and when each state changed. |
| [ ] Add cancellation endpoints | Request cancellation and create a signed cancellation order. | Gives operators a controlled stop mechanism. |
| [ ] Add retry endpoints | Require an explicit retry-safe decision and create a new attempt. | Reduces duplicate external effects. |
| [ ] Add runner management endpoints | Create, inspect, disable, reset, and revoke runners. | Supports the complete worker lifecycle. |
| [ ] Add enrollment download permission | Restrict installer creation to authorized users. | Protects worker identities and bootstrap tokens. |
| [ ] Add input limits and validation | Limit bodies, strings, arrays, timeouts, paths, and selectors. | Protects trust boundaries and database integrity. |
| [ ] Add pagination | Paginate tasks, runs, events, runners, and audit records. | Prevents large responses from degrading the control plane. |
| [ ] Add no-store download headers | Prevent shared caches from storing enrollment artifacts. | Reduces bootstrap token exposure. |
| [ ] Add API audit records | Record actor, action, target, address, time, and result. | Makes administrative changes traceable. |

Exit condition: authorized users can manage tasks, workers, and runs without direct database access.

## Phase 5: TypeScript and React frontend

| Task | Description | Impact |
|---|---|---|
| [ ] Create the default React TypeScript application | Start with the standard template and build setup. | Keeps the frontend familiar and easy to maintain. |
| [ ] Add typed API models | Define frontend types for tasks, workers, runs, events, and errors. | Catches contract mismatches during development. |
| [ ] Add authentication flow | Handle sign-in, sign-out, session expiry, and forbidden actions. | Protects management functions in the browser. |
| [ ] Add application navigation | Provide pages for tasks, workers, runs, and audit history. | Makes primary operations easy to find. |
| [ ] Add task list and filters | Show name, state, schedule, runner selector, and version. | Gives operators a clear task inventory. |
| [ ] Add task editor | Edit schedules, argument arrays, paths, timeouts, selectors, resources, and retry policy. | Exposes the full producer configuration safely. |
| [ ] Validate schedules in the UI | Show time zone and next occurrences before save. | Reduces scheduling mistakes. |
| [ ] Add manual run action | Start an immediate run from a task page. | Supports testing and urgent operations. |
| [ ] Add worker list and status | Show enrollment state, capacity, capabilities, heartbeat age, and key state. | Helps operators identify unavailable workers. |
| [ ] Add runner enrollment action | Create and download the correct installer for an operating system and architecture. | Provides the primary one-download worker setup. |
| [ ] Add enrollment reset and revocation actions | Require confirmation and show the security effect. | Supports failed installs and compromised workers. |
| [ ] Add run list and filters | Show state, attempt, assignment, schedule, duration, and result. | Makes active and failed work easy to inspect. |
| [ ] Add run detail timeline | Show each verified event and state transition. | Gives operators a complete audit view. |
| [ ] Add cancellation and retry actions | Show current eligibility and require confirmation. | Reduces accidental duplicate or destructive work. |
| [ ] Add clear loading and error states | Show API, authorization, validation, and connection failures. | Prevents hidden failures and repeated actions. |
| [ ] Add accessible forms and tables | Support labels, keyboard use, focus, contrast, and screen readers. | Makes core operations available to more users. |
| [ ] Add responsive layouts | Keep management pages usable on common screen sizes. | Supports operators outside a desktop-only workflow. |
| [ ] Add frontend tests | Test critical forms, permissions, and state actions. | Detects regressions in management workflows. |

Exit condition: a user can enroll a worker, create a task, start it, and inspect its verified history through the frontend.

## Phase 6: JetStream delivery

| Task | Description | Impact |
|---|---|---|
| [ ] Create the order stream | Store `glyphflow.orders.<runner_id>` subjects with durable retention. | Makes orders survive control plane and queue client restarts. |
| [ ] Create the event stream | Store `glyphflow.events.<runner_id>` subjects with durable retention. | Makes worker results survive control plane outages. |
| [ ] Add per-worker durable consumers | Give each worker one pull consumer for its order subject. | Isolates delivery and preserves worker progress. |
| [ ] Match pending delivery to capacity | Set maximum pending orders from the worker parallel task limit. | Prevents workers from accepting excess work. |
| [ ] Use explicit acknowledgements | Acknowledge an order only after durable local storage. | Prevents accepted work from disappearing after a restart. |
| [ ] Add the shared event consumer group | Let control plane replicas share event ingestion. | Adds safe event processing capacity. |
| [ ] Configure delivery limits | Limit message size, attempts, storage, retention, and acknowledgement time. | Bounds queue resource use and poison-message retries. |
| [ ] Add dead-letter subjects | Move exhausted or invalid messages to an inspectable subject. | Stops endless delivery loops and preserves diagnostics. |
| [ ] Add mutual TLS | Require trusted client certificates for all queue connections. | Authenticates clients and encrypts transport traffic. |
| [ ] Add per-subject permissions | Let a worker subscribe and publish only with its own runner identifier. | Stops cross-worker message access. |
| [ ] Match certificates to subjects | Derive or validate the runner identity from the certificate. | Prevents configuration from bypassing queue permissions. |
| [ ] Test broker restart recovery | Restart JetStream during order and event publication. | Proves durable delivery behavior. |

Exit condition: one worker completes a task through JetStream with no PostgreSQL route or credentials.

## Phase 7: Go worker

| Task | Description | Impact |
|---|---|---|
| [ ] Create the worker executable | Build a standalone binary with no PostgreSQL package. | Enforces the control plane data boundary. |
| [ ] Load one worker identity | Require one runner identifier and one capacity configuration. | Keeps queue permissions and audit ownership clear. |
| [ ] Connect outbound only | Connect to enrollment and JetStream without an inbound listener. | Simplifies VM firewall rules. |
| [ ] Consume only the assigned subject | Bind the durable consumer to the configured runner identifier. | Prevents accidental cross-worker execution. |
| [ ] Verify each order | Apply size, signature, version, identity, time, attempt, and replay checks. | Stops unauthorized or stale commands. |
| [ ] Create the local SQLite database | Store orders, message identifiers, execution state, and outgoing events. | Makes worker restarts recoverable. |
| [ ] Enforce unique message identifiers | Reject duplicate orders in SQLite. | Prevents duplicate queue delivery from repeating execution. |
| [ ] Store before acknowledgement | Commit an accepted order before acknowledging JetStream. | Prevents task loss between receipt and execution. |
| [ ] Publish accepted events | Sign and queue acceptance after durable storage. | Tells the control plane that the worker owns the order. |
| [ ] Limit parallel execution | Use configured capacity to start accepted orders. | Protects VM resources and preserves queue backpressure. |
| [ ] Execute argument arrays | Start executables directly without a shell. | Reduces command injection risk. |
| [ ] Restrict working directories | Allow task paths only below configured roots. | Prevents arbitrary filesystem access. |
| [ ] Validate environment names | Accept only valid names and approved secret references. | Prevents malformed environments and secret leakage. |
| [ ] Resolve secrets safely | Get secret values from an approved service at execution time. | Keeps secret values out of orders and central logs. |
| [ ] Use dedicated service accounts | Run the worker and tasks with restricted operating system identities. | Limits damage from a task or compromised worker. |
| [ ] Apply resource limits | Limit process count, memory, CPU, file access, and execution time where supported. | Protects the VM from runaway tasks. |
| [ ] Use separate process groups | Put each task in a process group and stop the full group. | Prevents child processes from surviving cancellation or timeout. |
| [ ] Bound captured output | Limit error text and calculate a digest for full external logs. | Protects queue storage and reduces secret exposure. |
| [ ] Publish lifecycle events | Sign accepted, started, heartbeat, and final events in sequence. | Gives the control plane a complete verified lifecycle. |
| [ ] Persist the event outbox | Keep signed events until JetStream confirms publication. | Prevents final results from disappearing during outages. |
| [ ] Recover after restart | Classify unfinished local work without signaling unverified process identifiers. | Avoids killing unrelated processes after PID reuse. |
| [ ] Handle cancellation orders | Match run, attempt, lease, and process group before stopping work. | Stops only the intended execution. |
| [ ] Report worker heartbeats | Publish signed capacity, active count, and health information. | Supports safe scheduling and offline detection. |
| [ ] Remove central database configuration | Reject or omit all PostgreSQL settings from the worker. | Makes the database isolation rule testable. |

Exit condition: a worker executes one verified order once and preserves its final event across restart and network failure.

## Phase 8: Enrollment, keys, and installers

| Task | Description | Impact |
|---|---|---|
| [ ] Protect the control plane key | Use a TPM when available or an encrypted root-owned file. | Reduces the risk of forged execution orders. |
| [ ] Generate a random enrollment token | Use a cryptographically secure token for each download. | Prevents token guessing. |
| [ ] Store only the token hash | Never store or log the plain enrollment token. | Reduces exposure after a database or log leak. |
| [ ] Expire enrollment tokens | Use a 15-minute default validity period. | Limits replay after an installer leak. |
| [ ] Enforce one successful use | Claim the token in one PostgreSQL transaction. | Stops a second machine from taking the same identity. |
| [ ] Bind enrollment to the runner | Reject a valid token for a different runner identifier. | Stops token substitution. |
| [ ] Bind optional platform fields | Validate the selected operating system and architecture. | Stops installation with the wrong artifact. |
| [ ] Build common worker releases | Build one tested worker artifact for each supported platform. | Avoids compiling a unique worker for every download. |
| [ ] Sign worker releases | Sign each common release artifact and publish its digest. | Detects changed or substituted worker binaries. |
| [ ] Create bootstrap data | Include runner, control plane, trust, token, expiry, version, and digest fields. | Gives the installer only the data needed for enrollment. |
| [ ] Sign bootstrap data | Use the control plane Ed25519 key for detached bootstrap signatures. | Detects changed enrollment configuration. |
| [ ] Exclude private keys and certificates | Do not place worker private keys or issued certificates in installers. | Keeps worker identity creation on the target VM. |
| [ ] Create Linux and Windows installers | Package the common worker, signatures, and bootstrap data. | Provides one download for each supported platform. |
| [ ] Verify before installation | Verify release and bootstrap signatures before writing service files. | Stops tampered installers from starting. |
| [ ] Generate keys on first start | Create Ed25519 and mutual TLS private keys on the target VM. | Prevents private key transfer from the control plane. |
| [ ] Protect local keys | Use a TPM when available or a root-owned encrypted file. | Reduces worker key theft. |
| [ ] Enroll through HTTPS | Send the token and public keys through a trusted TLS connection. | Protects bootstrap data in transit. |
| [ ] Issue the client certificate | Return the worker certificate, queue address, subjects, trust, and limits. | Enables authenticated queue access without sharing private keys. |
| [ ] Activate after signed heartbeat | Mark the worker active only after a verified registration heartbeat. | Confirms that the enrolled machine controls its signing key. |
| [ ] Delete bootstrap secrets | Remove the token and temporary files after enrollment. | Reduces secret persistence on the VM. |
| [ ] Support interrupted installation | Reuse protected keys after successful enrollment instead of creating new identities. | Makes safe recovery possible. |
| [ ] Detect clock differences | Stop enrollment with a clear error when clock skew exceeds policy. | Preserves expiration and signature time checks. |
| [ ] Add key rotation | Allow two active keys during a defined overlap period. | Supports planned rotation without downtime. |
| [ ] Add key and certificate revocation | Reject new messages and remove queue access immediately. | Limits damage from compromised workers or control plane keys. |
| [ ] Add signed worker updates | Verify version, signature, and digest while preserving identity keys. | Allows safe worker maintenance. |

Exit condition: one downloaded installer creates an active worker without embedded or transferred private keys.

## Phase 9: Failure handling and recovery

| Task | Description | Impact |
|---|---|---|
| [ ] Stop production during database failure | Do not create orders when durable task state is unavailable. | Prevents untracked execution. |
| [ ] Leave events unacknowledged after commit failure | Let JetStream redeliver events after database recovery. | Prevents lost results. |
| [ ] Retry pending outbox rows | Resume order publication after broker recovery. | Prevents queued runs from disappearing. |
| [ ] Retry worker event publication | Resume local outbox delivery after network recovery. | Preserves accepted and final events. |
| [ ] Make duplicate orders idempotent | Return the stored state without starting the command again. | Reduces duplicate external effects. |
| [ ] Make duplicate events idempotent | Treat an existing event identifier as success without a second state update. | Keeps control plane state correct. |
| [ ] Handle poison messages | Terminate or dead-letter invalid messages with bounded diagnostics. | Prevents endless invalid-message delivery. |
| [ ] Mark interrupted execution safely | Report recovery state without trusting stored process identifiers. | Prevents signals to unrelated processes. |
| [ ] Detect worker loss | Mark expired workers offline and active runs lost after a safety interval. | Gives operators a clear recovery decision. |
| [ ] Gate automatic retry | Require task owners to declare retry safety and idempotency. | Reduces repeated external effects. |
| [ ] Use a new lease for each attempt | Increment the attempt and replace the lease token. | Stops stale workers from changing a retried run. |
| [ ] Keep leases after unconfirmed loss | Release resources only after policy permits recovery. | Prevents overlapping use during uncertain worker state. |
| [ ] Test every outage boundary | Stop PostgreSQL, JetStream, the control plane, and workers at commit boundaries. | Proves documented recovery behavior. |

Exit condition: each supported failure either recovers automatically or stops with a clear and safe operator state.

## Phase 10: Process and network security

| Task | Description | Impact |
|---|---|---|
| [ ] Isolate the PostgreSQL network | Allow connections only from control plane hosts. | Makes worker database access impossible at the network layer. |
| [ ] Remove inbound worker requirements | Use outbound enrollment and queue connections only. | Reduces the VM attack surface. |
| [ ] Synchronize clocks | Configure and monitor time synchronization on all hosts. | Keeps message expiry, leases, and certificates reliable. |
| [ ] Define trust bundles | Manage control plane, queue, and certificate authority trust separately. | Supports safe certificate replacement and revocation. |
| [ ] Add certificate expiry monitoring | Alert before control plane and worker certificates expire. | Prevents avoidable service interruption. |
| [ ] Review command allow rules | Define allowed executables, roots, users, and resource ceilings per worker pool. | Limits what a valid order can do. |
| [ ] Add secret redaction | Remove secret values from logs, events, errors, and frontend responses. | Reduces credential disclosure. |
| [ ] Add dependency vulnerability checks | Scan Go, npm, container, and release dependencies. | Detects known security risks before release. |
| [ ] Add a security response process | Document revocation, rotation, incident evidence, and recovery actions. | Reduces response time after compromise. |

Exit condition: network policy, identity checks, process restrictions, and secret handling pass a security review.

## Phase 11: Logs, metrics, alerts, and audit

| Task | Description | Impact |
|---|---|---|
| [ ] Add structured control plane logs | Include message, run, worker, key, and verification identifiers. | Makes distributed task traces searchable. |
| [ ] Add structured worker logs | Include local order state without secret values or full environments. | Supports VM troubleshooting without data leakage. |
| [ ] Add immutable security audit records | Record enrollment, key, permission, installer, cancellation, retry, and revocation actions. | Supports incident investigation and compliance review. |
| [ ] Measure due task count | Report producer backlog. | Shows schedule processing problems. |
| [ ] Measure dispatch delay and outbox age | Report the time from due occurrence to confirmed publication. | Shows database or broker delivery problems. |
| [ ] Measure event consumer delay | Report the age of unprocessed worker events. | Shows state update delays. |
| [ ] Measure delivery and dead-letter counts | Report redelivery, exhaustion, and invalid message rates. | Shows reliability or attack problems. |
| [ ] Measure signature rejections | Count failures by bounded reason and identity. | Detects configuration errors and forged traffic. |
| [ ] Measure worker heartbeat age and capacity | Report worker availability and active task count. | Supports scheduling and offline alerts. |
| [ ] Measure task duration and final state | Report execution latency and outcomes. | Supports service health and task tuning. |
| [ ] Alert on stale outbox rows | Trigger when the oldest pending order exceeds policy. | Detects blocked task delivery. |
| [ ] Alert on worker lease expiry | Trigger when a worker heartbeat expires. | Detects VM or network failure. |
| [ ] Alert on repeated signature failure | Trigger on abnormal verification rejection rates. | Detects attack or key mismatch. |
| [ ] Alert on dead-letter messages | Trigger when any message reaches a dead-letter subject. | Requires operator review of lost workflow progress. |
| [ ] Alert on stuck task states | Trigger when a run exceeds its state timeout. | Detects incomplete lifecycle processing. |
| [ ] Alert on stale resource leases | Trigger when a final run retains a resource. | Detects blocked future tasks. |
| [ ] Add external log storage only when required | Use self-hosted object storage when bounded messages cannot meet retention needs. | Defers an extra service until log volume requires it. |

Exit condition: operators can trace one run from schedule through final state and receive alerts for each critical delay.

## Phase 12: Availability, backup, and operations

| Task | Description | Impact |
|---|---|---|
| [ ] Deploy multiple control plane replicas | Run active replicas coordinated by PostgreSQL locks. | Removes one control plane process as a single point of failure. |
| [ ] Deploy a three-node JetStream cluster | Replicate streams across separate failure zones. | Preserves message delivery after one queue node fails. |
| [ ] Define PostgreSQL high availability | Use Patroni and etcd for production failover when required. | Reduces central state downtime. |
| [ ] Configure PostgreSQL backups | Use pgBackRest with point-in-time recovery. | Protects schedules, runs, public keys, and audit data. |
| [ ] Exclude plaintext private keys from backups | Back up public state without unprotected signing keys. | Limits key exposure through backup systems. |
| [ ] Test database failover | Move the primary during producer and event processing. | Proves lock and retry behavior. |
| [ ] Test database restoration | Restore to a new environment and verify task and audit consistency. | Proves backups can recover the platform. |
| [ ] Test JetStream replica loss | Remove a queue node during delivery. | Proves message availability. |
| [ ] Define recovery objectives | Set recovery time and recovery point targets for each component. | Gives operations measurable service goals. |
| [ ] Write operator runbooks | Document outages, stuck runs, dead letters, rotation, revocation, and restore. | Gives operators tested response steps. |

Exit condition: tested failover and restore procedures meet the defined recovery objectives.

## Phase 13: Test plan

| Task | Description | Impact |
|---|---|---|
| [ ] Add unit tests | Test schedule rules, validators, transitions, selectors, limits, and retry policy. | Detects logic regressions quickly. |
| [ ] Add PostgreSQL integration tests | Test locks, uniqueness, transactions, outbox, inbox, and leases. | Proves central consistency rules. |
| [ ] Add JetStream integration tests | Test redelivery, acknowledgements, durability, limits, and dead letters. | Proves queue behavior. |
| [ ] Add worker execution tests | Test success, failure, timeout, cancellation, child processes, paths, limits, and output bounds. | Proves safe process handling. |
| [ ] Add enrollment tests | Test expiry, second use, wrong runner, wrong platform, and changed artifacts. | Proves bootstrap security. |
| [ ] Add queue permission tests | Deny cross-worker subscriptions and publications. | Proves worker isolation. |
| [ ] Add mutual TLS tests | Deny missing, expired, revoked, and wrong-identity certificates. | Proves transport authentication. |
| [ ] Add secret leakage tests | Scan logs, responses, events, installers, and state files. | Prevents accidental secret disclosure. |
| [ ] Add API authorization tests | Test every protected operation for each role. | Prevents permission regressions. |
| [ ] Add frontend workflow tests | Test enrollment, task creation, manual runs, history, cancellation, and retry. | Protects the main user workflows. |
| [ ] Add end-to-end tests | Run one task from the React UI through a remote worker and back. | Proves all components work together. |
| [ ] Add duplicate delivery tests | Publish each order and event twice. | Proves idempotent processing. |
| [ ] Add restart tests | Restart each component at every durable handoff. | Proves crash recovery. |
| [ ] Add load tests | Measure due tasks, dispatch, events, workers, and run history at target scale. | Finds capacity limits before production. |
| [ ] Add clock-skew tests | Test accepted tolerance and rejection beyond policy. | Proves time-based security checks. |

Exit condition: automated tests cover protocol, delivery, state, security, recovery, API, frontend, and end-to-end execution.

## Phase 14: Open-source supply chain and releases

| Task | Description | Impact |
|---|---|---|
| [ ] Record SPDX identifiers | Record a license identifier for each direct dependency. | Makes license review automatic. |
| [ ] Enforce the approved license list | Reject unknown or disallowed dependency licenses in continuous integration. | Prevents incompatible components from entering releases. |
| [ ] Review transitive licenses | Review dependency license changes before each release. | Reduces legal and redistribution risk. |
| [ ] Generate a software bill of materials | Create an SBOM for control plane, worker, frontend, and installer artifacts. | Gives operators a complete component inventory. |
| [ ] Sign release artifacts | Sign executables, containers, installers, checksums, and SBOM files. | Detects changed release files. |
| [ ] Publish reproducible build instructions | Document tool versions and release commands. | Lets contributors verify official artifacts. |
| [ ] Confirm no proprietary dependency | Test that deployment works without proprietary services or closed-source agents. | Preserves the open-source project goal. |
| [ ] Publish protocol documentation | Document subjects, envelopes, payloads, signatures, limits, and version policy. | Lets other open-source tools interoperate safely. |
| [ ] Publish deployment documentation | Document development, single-node, and production topologies. | Helps operators select an appropriate setup. |
| [ ] Publish security documentation | Document trust, keys, enrollment, isolation, limitations, and response. | Sets correct security expectations. |

Exit condition: every release is signed, documented, license-approved, and accompanied by an SBOM.

## Phase 15: Migration and production rollout

| Task | Description | Impact |
|---|---|---|
| [ ] Split the local producer and consumer behavior | Move scheduling and central writes to the control plane. Move execution to the worker. | Removes shared local database access. |
| [ ] Remove email and history writes from workers | Process notifications and final history in the control plane. | Keeps central credentials on central hosts. |
| [ ] Add a routing flag | Assign each task definition to either the local scheduler or Glyphflow during migration. | Prevents both systems from executing the same task. |
| [ ] Import task definitions | Convert local schedules, commands, directories, resources, and runner identifiers. | Preserves existing task configuration. |
| [ ] Validate imported schedules | Compare next occurrences between the local scheduler and Glyphflow. | Detects migration timing errors. |
| [ ] Register one canary worker | Use one production-like VM and a small task group. | Limits initial deployment risk. |
| [ ] Verify canary state and recovery | Check messages, runs, events, logs, metrics, duplicates, and restart behavior. | Proves the production path before expansion. |
| [ ] Stop on duplicate execution | Define duplicate execution as a canary stop condition. | Protects systems with external side effects. |
| [ ] Expand task groups gradually | Move tasks only after stable canary periods. | Limits the effect of unknown failures. |
| [ ] Test rollback | Return routed tasks to the local scheduler without duplicate occurrences. | Provides a safe exit from the migration. |
| [ ] Retire local queue access | Remove local consumer access after all tasks migrate. | Completes the network architecture boundary. |
| [ ] Complete the production readiness review | Verify all completion gates below. | Prevents an incomplete security or recovery model from reaching production. |

Exit condition: all routed tasks use Glyphflow, and the local consumer cannot execute them.

## Production completion gates

| Gate | Description | Impact |
|---|---|---|
| [ ] No worker can connect to PostgreSQL | Verify packages, configuration, credentials, routes, and firewall rules. | Protects central data from worker compromise. |
| [ ] Every order has a valid control plane signature | Reject all unsigned, altered, expired, or revoked-key orders. | Prevents unauthorized execution. |
| [ ] Every accepted event has a valid worker signature | Reject all unsigned, altered, stale, or wrong-worker events. | Protects task state integrity. |
| [ ] Mutual TLS protects every queue connection | Require valid control plane and worker certificates. | Protects transport identity and confidentiality. |
| [ ] Queue permissions isolate every worker | Test publish and subscribe denial for other worker subjects. | Prevents cross-worker access. |
| [ ] Duplicate orders execute once | Verify persistent worker message uniqueness. | Reduces duplicate command effects. |
| [ ] Duplicate events apply once | Verify central event uniqueness and transaction rules. | Preserves correct task state. |
| [ ] Both outboxes recover after failure | Test control plane order and worker event publication recovery. | Prevents lost work and results. |
| [ ] Key rotation and revocation pass | Test overlap, expiry, immediate rejection, and certificate removal. | Supports routine and emergency key operations. |
| [ ] Every run has a complete audit history | Trace schedule, order, events, state, user actions, and verification results. | Supports operations and incident review. |
| [ ] The primary worker setup uses one download | Verify the administrator enrollment workflow. | Makes secure deployment practical. |
| [ ] Private keys originate on the worker | Inspect installers and enrollment traffic. | Prevents central possession of worker private keys. |
| [ ] Enrollment tokens expire and work once | Test expiry, claim races, replay, and wrong-runner use. | Prevents worker identity theft. |
| [ ] All required software has an approved license | Validate direct and transitive dependencies. | Preserves legal open-source distribution. |
| [ ] Every release includes SPDX data and an SBOM | Publish and verify release metadata. | Gives operators supply-chain visibility. |
| [ ] Deployment requires no proprietary endpoint | Install and operate the complete platform with self-hosted components. | Preserves vendor independence. |
| [ ] Backup and restore tests pass | Recover central state and confirm audit consistency. | Proves disaster recovery. |
| [ ] Failure and canary tests pass | Complete outage, duplicate, restart, revocation, and rollout checks. | Proves production readiness. |

## Deferred work

| Task | Description | Impact |
|---|---|---|
| [ ] Evaluate service splitting | Split scheduler, dispatcher, ingester, or notifications only after measured scaling or ownership needs. | Avoids early distributed-system complexity. |
| [ ] Evaluate container task isolation | Add containers only when operating system accounts cannot meet isolation requirements. | Avoids an unnecessary runtime dependency. |
| [ ] Evaluate external log storage | Add self-hosted object storage only when bounded queue messages cannot meet retention needs. | Avoids operating unused storage infrastructure. |
| [ ] Evaluate automatic cross-worker reassignment | Add it only after task owners define retry safety and idempotency. | Avoids duplicate external effects. |
| [ ] Evaluate hardware attestation | Add it only when the threat model requires stronger worker trust. | Avoids complex security infrastructure without a defined need. |
