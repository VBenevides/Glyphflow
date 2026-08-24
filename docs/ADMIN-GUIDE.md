# Glyphflow administrator guide

This guide describes the current self-hosted Glyphflow console. It covers
administration tasks that are implemented today; it is not a promise that an
installation supplies an external identity provider, secret manager, reverse
proxy, backups, or host hardening.

For the system model and configuration reference, see the
[technical documentation](README.md), the [architecture document](../ARCHITECTURE.md),
and the [production Compose overlay](../compose.production.yaml). The
interactive API reference is available at `/docs` and `/openapi.json` on a
running control plane.

## Administration map

The console hides pages that the signed-in user cannot access. API requests
also enforce the permission listed below.

| Console area | Path | Permission |
| --- | --- | --- |
| Users and sessions | `/admin/users` | `users.read` or `users.manage` |
| Roles | `/admin/roles` | `roles.read` or `roles.manage` |
| Single sign-on | `/admin/sso` | `sso.read` or `sso.manage` |
| Authentication settings | `/admin/auth` | `auth.settings.manage` |
| Execution status | `/admin/execution-status` | `auth.settings.manage` |
| Runner pools and runners | `/runners` | `runners.read` or `runners.manage` |
| Global variables | `/global-variables` | `users.manage` |
| Resources | `/resources` | `resources.read` or `resources.manage` |
| Audit events | `/audit` | `audit.read` |

## Users and authentication

### Bootstrap and sign-in

Set `GLYPHFLOW_BOOTSTRAP_EMAIL` and `GLYPHFLOW_BOOTSTRAP_PASSWORD` together if
the first administrator must be created during startup. Set
`GLYPHFLOW_SYSTEM_ADMINS` to a space-, comma-, or semicolon-separated list of
administrator email addresses. Matching users receive the immutable `admin`
role and cannot be demoted or disabled through the normal user controls.

The sign-in page offers the methods enabled by the deployment:

- Password sign-in is controlled by `ENABLE_PASSWORD_LOGIN` and the
  **Enable password login** setting.
- Password self-registration is controlled by
  `ENABLE_PASSWORD_REGISTRATION` and **Allow password registration**.
- OIDC providers appear as single-sign-on buttons after an administrator
  configures them.

Production Compose defaults password login and registration to disabled. Keep
at least one working login method before changing authentication settings. The
API rejects a change that would leave the installation with no login method.

### Manage users

1. Open **Users and sessions** and select **Create user**.
2. Enter the user's email and a temporary password.
3. Open the user's **Details** page and use **Manage access** to assign roles.
4. Disable the account when access must stop. Do not use disabling as a
   substitute for runner revocation or secret rotation.

User details show status, login methods, roles, role sources, effective
permissions, linked identities, and active sessions. A user's email is the
login identifier in the current password flow. Users can manage their own
display name, password, linked OIDC identities, and sessions from **Account**.

### Sessions

Use **Users and sessions → Sessions** to review active sessions across users.
An administrator with `users.manage` can revoke one session. A user's details
page also provides **Revoke All**. Revocation immediately invalidates the
selected session(s); it does not disable the user.

Users can revoke their own sessions from **Account → Sessions**. The console
shows session ID, client information when available, last interaction, and
expiry.

### Configure OIDC

1. Open **Users and sessions → SSO**.
2. Select **Add provider**.
3. Enter a provider key, display name, issuer URL, client ID, and the
   deployment's secret reference.
4. Add claim mappings and optional group-to-role mappings.
5. Confirm that another administrator login method works before disabling a
   provider.

The console stores and displays the secret reference, not the resolved secret
value. The OIDC client secret must be available to the server-side resolver
used by the deployment. This repository does not provide a general secret
vault UI. Provider configuration is audited, and resolved secrets are not
returned by the provider listing.

Users can link an OIDC identity from **Account → Identities**. Keep another
working login method before unlinking an identity.

## Roles and permissions

Glyphflow seeds three immutable system roles:

| Role | Effective permissions |
| --- | --- |
| `admin` | Every permission in the catalog |
| `operator` | `tasks.read`, `tasks.manage`, `runs.read`, `runs.execute`, `resources.read`, `resources.manage`, `runners.read`, `runners.manage` |
| `user` | `tasks.read`, `runs.read`, `runs.execute`, `resources.read`, `runners.read` |

The permission catalog is:

```text
users.read              users.manage
roles.read              roles.manage
sso.read                sso.manage
auth.settings.manage
tasks.read              tasks.manage
runs.read               runs.execute       runs.cancel       runs.retry
logs.read
resources.read          resources.manage
runners.read            runners.manage
audit.read
```

To create a least-privilege custom role:

1. Open **Roles** and select **Create role**.
2. Choose only the permissions required by the job.
3. Assign the role from the user's **Manage access** dialog.
4. Revoke the role when it is no longer needed.

Custom roles may be renamed, edited, and deleted. System roles are immutable.
The administrator guard prevents removing the last administrator or changing
the protected system-administrator assignment.

Use `users.read` for read-only identity review, `users.manage` for account,
role assignment, session, and global-variable administration, and
`auth.settings.manage` only for authentication and execution-status changes.
Operational permissions are independent: reading runs does not grant the
ability to execute, cancel, retry, or read logs.

## Runner pools and runner lifecycle

### Pools

Open **Runners and pools → Pools**. Create a pool with a name, description,
and enabled state. Tasks and enrollments select an enabled pool. Archiving a
pool removes it from active placement; the durable store rejects archival when
the pool is still used by runners or task versions, so archive those dependents
first. Historical task versions are retained.

### Enroll a runner

1. Open **Runners** and select **Enroll runner**.
2. Enter a runner name and select an enabled pool.
3. Select `linux` or `windows`, `amd64`, capacity, and the worker UI:
   **GUI**, **TUI**, or **Headless**.
4. Set the control-plane and NATS endpoints when the embedded defaults are not
   reachable from the target machine.
5. Select **Create enrollment**, download the binary, and run it once on the
   target machine.
6. Wait for the runner to report `ONLINE` before placing work on it.

Enrollment artifacts are one-use and expire after 15 minutes by default. A
consumed or expired artifact must be replaced with a new enrollment. Persist
the worker `DATA_DIR`: it contains the worker's local SQLite recovery state and
identity. Deleting it can require re-enrollment. See the
[worker build and UI notes](../backend/cmd/worker/README.md).

### Lifecycle controls

The runner list separates desired state from observed state:

| Control/state | Meaning |
| --- | --- |
| `ENABLED` | Eligible to receive work when online, capable, and below capacity |
| `DRAINING` | Stops new work while current work can finish |
| `DISABLED` | Does not receive new work |
| `PENDING` | Registered but has not completed its current connection cycle |
| `ONLINE` | Reporting a current heartbeat |
| `OFFLINE` | Not reporting a current heartbeat or disconnected |
| `REVOKED` | Runner is marked revoked and should not receive work until it is reset/re-enabled |

Use **Drain** for planned maintenance. Use **Revoke** when the runner host or
its identity is no longer trusted; re-enable it only after the host is safe.
Use **Archive** for permanent removal. Archiving disconnects the runner,
revokes its keys and unused enrollments, cancels waiting work, and cancels or
stops active work according to the run lifecycle. Archived runners cannot be
recovered; create a new enrollment instead.

Runner placement also considers pool, optional pinned runner, capability tags,
capacity, current heartbeat, and resource availability. A runner being
`ONLINE` alone does not guarantee that it can accept a particular task.

## Global variables

Open **Global Variables** to manage reusable, non-secret values. Names must
match:

```text
[A-Z_][A-Z0-9_]*
```

Reference a value in supported task and schedule fields as `$ENV:NAME`, for
example `$ENV:BACKUP_ROOT`. Resolution is performed when the execution
definition is prepared. An undefined reference fails resolution; do not use
global variables for passwords, tokens, private keys, or other secrets because
their values are displayed as ordinary variable values.

Delete a variable only after removing its task and schedule references. The
console reports the reference count, and durable storage blocks deletion while
references remain.

## Secrets and secret references

Glyphflow separates a reference from a secret value. Task versions can carry
secret references, and dispatch sends the reference names to the worker rather
than embedding the secret values in the execution payload. OIDC providers use
the same pattern for their client secret reference.

The current console has no general secret creation, rotation, or vault
integration screen. Configure the resolver or secret manager used by the
deployment outside the task editor, and put only a reference in Glyphflow.
Never place a secret in a command argument, global variable, screenshot,
commit, or log. Rotate a secret in its owning secret system and update the
reference or provider configuration as required by that system.

This boundary does not make a compromised worker safe: a worker host that is
authorized to execute work may still observe data made available to its
process. Protect worker hosts and revoke their runner identities when they
are no longer trusted.

## Resources

Open **Resources** to create a named scheduling resource. Choose:

- **Exclusive** when only one eligible execution may hold the resource at a
  time; or
- **Non-blocking** when the resource is metadata or a placement requirement
  that must not block another execution.

Add the resource to a task version's resource requirements. The run details
show active leases, holder, expiry, and fencing token. A resource is available
when it has no active lease, and leased when an execution owns it.

Do not delete a resource that is referenced by a task version or has an active
lease. The durable store rejects that deletion. An expired lease is fenced by
a new fencing token; an old holder cannot release a newer lease with the wrong
token.

## Exit codes

Open **Execution Status** to maintain the meanings shown beside completed run
exit codes. The current seeded system meanings are:

| Code | Meaning |
| ---: | --- |
| `0` | Success |
| `1` | Generic/unhandled error |
| `2` | Invalid arguments / usage |
| `5` | Start Failure |
| `6` | Timeout |

System meanings cannot be edited or deleted. Administrators can create custom
integer meanings, but a custom code cannot be changed or deleted while an
execution attempt uses it. Exit-code meanings explain a process result; they
do not by themselves make a command retry-safe.

## Audit history

Open **Audit events** to inspect administrative and scheduler changes. Filter
by actor, action, target, result, correlation ID, and time range. The default
view excludes audit-read events and run-log events; clear those filters when a
full request history is needed.

An event's details include timestamp, actor, description, HTTP method,
endpoint, result, event ID, correlation ID, input/output, before/after values,
and traceback when present. Values whose keys contain password, secret, or
token are redacted before they are returned to the console. Audit rows are
append-only in the database: the implementation rejects update and delete
operations on audit events.

Audit history is evidence of recorded requests, not proof that a remote system
accepted a command or that a compromised worker reported a truthful result.
Use run attempts, signed lifecycle events, logs, and the worker lifecycle
controls together when investigating execution.

## Production deployment boundaries

The repository's base `compose.yaml` is a local-development quick start. It
uses development credentials, exposes PostgreSQL and NATS, and allows
insecure transport. Do not expose that configuration publicly.

For a self-hosted deployment, review the
[production configuration reference](README.md#production-deployment) and use
the [production Compose overlay](../compose.production.yaml). At minimum, the
production boundary requires:

- `ENVIRONMENT=production` and `ALLOW_INSECURE_TRANSPORT=false`;
- PostgreSQL with `DATABASE_URL` using `sslmode=verify-full`, plus protected
  PostgreSQL CA, certificate, key, and password files;
- NATS with a `tls://` URL, client certificate/key/CA, and credentials;
- an HTTPS `WEB_ORIGIN` and explicit `CORS_ORIGIN` and `CSRF_ORIGINS` values;
- protected `ACCESS_TOKEN_SECRET`, `CONTROL_PLANE_SIGNING_PRIVATE_KEY`, and,
  when password login is enabled, `PASSWORD_PEPPER` values;
- bootstrap and system-administrator settings supplied through deployment
  secrets or protected environment handling; and
- a private PostgreSQL and NATS network.

The production overlay binds the web container to `127.0.0.1` by default. Put
an appropriately configured reverse proxy or equivalent network boundary in
front of it to provide public TLS and the intended hostname. The overlay does
not create DNS, terminate public TLS, protect worker hosts, operate a secret
manager, perform backups, or replace monitoring and incident response.

Workers connect outbound to the control plane and NATS and do not need
PostgreSQL credentials. Keep worker `DATA_DIR` persistent, use TLS material in
production, restrict NATS permissions as appropriate for each worker, and
revoke a runner when its host is no longer trusted. Glyphflow provides
at-least-once delivery, not exactly-once command execution; commands that
change external systems must be safe to retry or provide their own idempotency.

## Related references

- [Commercial overview](../README.md)
- [Technical documentation and configuration](README.md)
- [Architecture](../ARCHITECTURE.md)
- [Worker build and UI notes](../backend/cmd/worker/README.md)
- [Example flow index](examples/README.md)
