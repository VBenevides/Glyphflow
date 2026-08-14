# Glyphflow v1 compatibility and completion roadmap

This roadmap closes the verified gaps between the v1 plans and the current frontend and backend.

All rows remain unchecked until implementation and verification are complete.

Implement one row at a time. Test it, commit it, and record the commit hash and result in the last column.

## Project outcome

Glyphflow will start one persistent control plane and any number of durable workers.

The frontend and backend will share one tested API, cookie-session, error, pagination, and log-stream contract.

An authorized user will sign in, create a task and schedule, execute it, and inspect its output.

## Phase 0: Security

| Task | Description | Impact | How to implement | How to test | Test Result |
|---|---|---|---|---|---|
| [x] Critical: Replace the query-trusted OIDC callback | Replace the query-trusted OIDC callback. | Prevents an unsafe identity flow from reaching production. | Exchange the authorization code server-side. Verify issuer metadata, JWKS signature, audience, nonce, expiry, state, and PKCE. | Test forged claims, forged signatures, wrong issuer, replay, expiry, and one successful provider flow. | commit: fca341d; PASS — `GOCACHE=/tmp/glyphflow-oidc-go-cache go test ./...` passes, including signed-token and callback exchange tests. |
| [x] High: Apply authentication rate limits | Apply authentication rate limits. | Reduces password and OIDC abuse. | Reuse `platform.RateLimiter`. Key password attempts by username and source address. Limit OIDC challenges and callbacks. | Test independent keys, window expiry, generic errors, and `429` responses. | commit: 2d3d1e4; PASS — `GOCACHE=/tmp/glyphflow-rate-limit-cache go test ./...` passes, including independent password and OIDC `429` endpoint tests. |
| [x] High: Enforce the backend password policy | Enforce the backend password policy. | Prevents clients from bypassing browser validation. | Validate password length and approved rules in `AuthService.Register`. Load the Argon2id pepper from a secret source. | Test short passwords, valid passwords, missing pepper configuration, and bootstrap behavior. | commit: 942ef8a; PASS — `GOCACHE=/tmp/glyphflow-password-policy-cache go test ./...` passes, including password, pepper, and bootstrap tests. |
| [x] High: Protect the final administrator and login method | Protect the final administrator and login method. | Prevents administrative lockout. | Call the existing guard logic inside each persistent user, role, provider, and setting transaction. | Test concurrent administrator removal, final provider disablement, and final password removal. | commit: 9fc267c; PASS — `GOCACHE=/tmp/glyphflow-admin-guard-cache go test ./...` passes, including final-admin and login-method tests. |
| [ ] High: Classify routes from the actual router | Classify routes from the actual router. | Prevents an unclassified route from bypassing authorization review. | Register each method, path, access rule, and handler from one route definition. Remove duplicate route lists. | Enumerate the built router. Fail when any route lacks a public, authenticated, or permission rule. | |

Exit condition: every authentication and administration route enforces verified identity, abuse controls, and lockout protection.

## Phase 1: Bugs

| Task | Description | Impact | How to implement | How to test | Test Result |
|---|---|---|---|---|---|
| [ ] Critical: Implement one browser session contract | Implement one browser session contract. | Blocks every authenticated frontend workflow. | Use the planned `HttpOnly` cookie session. Set access and refresh cookies during password and OIDC login. | Run login, `/me`, refresh, logout, expiry, revocation, and disabled-user checks through real HTTP. | |
| [ ] Critical: Add public runtime configuration and CSRF issuance | Add public runtime configuration and CSRF issuance. | Blocks frontend startup and every unsafe request. | Add `GET /api/v1/config`. Return safe flags and issue the readable CSRF cookie. | Test startup with password-only, SSO-only, mixed, and missing configuration. Test CSRF rejection and success. | |
| [ ] Critical: Make local browser origins consistent | Make local browser origins consistent. | Blocks the development frontend before API routing. | Prefer a Vite same-origin API proxy. Otherwise add one exact CORS allow-list and complete preflight handling. | Start `dev_run.sh`. Verify config, login, and `/me` from the displayed frontend URL. | |
| [ ] Critical: Publish one JSON contract | Publish one JSON contract. | Prevents runtime failures after authentication succeeds. | Add explicit DTOs and JSON tags. Return permission arrays and consistent session, token, provider, and error fields. | Decode every response with frontend models. Reject unknown required fields in contract tests. | |
| [ ] High: Align resource and action routes | Align resource and action routes. | Restores frontend calls to the intended backend handlers. | Move run actions under `/api/v1/runs/{id}`. Add every detail, version, preview, stream, and download path. | Compare all frontend API paths with the built router and OpenAPI document. | |
| [ ] High: Revoke the authenticated session on logout | Revoke the authenticated session on logout. | Prevents a signed-out browser session from remaining valid. | Read the current session from authentication. Revoke it and clear access, refresh, and CSRF cookies. | Confirm the old session fails after logout and the browser returns to login. | |
| [ ] High: Make worker recovery atomic | Make worker recovery atomic. | Prevents an unknown execution from losing its recovery event. | Mark old-boot orders unknown and insert recovery events in one SQLite transaction. | Inject a restart before and after commit. Verify one durable recovery event. | |
| [ ] Medium: Load task and schedule edit baselines | Load task and schedule edit baselines. | Prevents blank edit forms and accidental replacement values. | Fetch the active version before editing. Keep the loaded draft as the dirty-state baseline. | Test loading, edits, save conflicts, discard, and unchanged navigation. | |
| [ ] Medium: Fix the account dirty-state baseline | Fix the account dirty-state baseline. | Prevents false unsaved-change warnings. | Compare the edited display name with the loaded profile value. Reset the baseline after save. | Test unchanged profiles, edits, successful saves, failed saves, and navigation. | |

Exit condition: frontend startup, authentication, JSON decoding, routing, logout, recovery, and edit flows behave consistently.

## Phase 2: New Features

| Task | Description | Impact | How to implement | How to test | Test Result |
|---|---|---|---|---|---|
| [ ] Critical: Wire the production control-plane runtime | Wire the production control-plane runtime. | Blocks the planned durable scheduler. | Connect PostgreSQL and NATS. Apply migrations. Start scheduler, dispatcher, consumers, producers, retention, metrics, API, and shutdown handling. | Start two control planes. Test one occurrence, failover, duplicate delivery, readiness, and graceful shutdown. | |
| [ ] Critical: Wire the production worker runtime | Wire the production worker runtime. | Blocks all remote execution. | Open SQLite. Recover old claims. Connect NATS with mTLS. Consume orders, execute processes, publish events, and send heartbeats. | Test enrollment, restart recovery, stale orders, cancellation, timeout, output limits, and event replay. | |
| [ ] High: Implement task and schedule APIs | Implement task and schedule APIs. | Enables the first scheduler authoring workflow. | Use revision 3 tables. Add list, detail, create, version activation, preview, filter, and pagination operations. | Create and edit tasks and schedules. Test conflicts, rollback, DST, misfires, deadlines, and concurrency. | |
| [ ] High: Implement run and log APIs | Implement run and log APIs. | Enables execution and diagnosis. | Add run list, detail, execute, cancel, retry, reconcile, events, resumable logs, and download handlers. | Test every legal action state, duplicate requests, stream resume, gaps, terminal states, and bounded downloads. | |
| [ ] High: Implement runner, enrollment, and resource APIs | Implement runner, enrollment, and resource APIs. | Enables infrastructure management. | Add runner detail, lifecycle, pools, sessions, enrollment artifacts, resources, leases, and guarded deletion. | Test expiry, second enrollment use, stale heartbeats, lease takeover, fencing, and active-reference conflicts. | |
| [ ] High: Implement administration and account APIs | Implement administration and account APIs. | Enables planned identity management. | Add users, roles, assignments, SSO, settings, profile, password, identities, and owned-session operations. | Test immediate grants, system-role protection, last-admin protection, linking, unlinking, and session ownership. | |
| [ ] Medium: Implement the audit query API | Implement the audit query API. | Enables security and execution traceability. | Query durable audit events with time, actor, action, target, result, request, and correlation filters. | Test redaction, deleted targets, system actors, pagination, and correlation lookup. | |

Exit condition: one authorized user can author, schedule, execute, inspect, and administer work through persistent services.

## Phase 3: Enhancements

| Task | Description | Impact | How to implement | How to test | Test Result |
|---|---|---|---|---|---|
| [ ] High: Add one frontend-backend contract smoke test | Add one frontend-backend contract smoke test. | Prevents another incompatible green build. | Start the real control plane on an ephemeral port. Run config, login, `/me`, one protected request, refresh, and logout. | Run the smoke test in the default backend and frontend verification commands. | |
| [ ] High: Add real distributed failure tests | Add real distributed failure tests. | Verifies the reliability claims in `internal/v1`. | Use PostgreSQL, NATS, and worker processes. Kill each component at documented commit boundaries. | Verify one logical occurrence, durable retries, duplicate safety, lease takeover, and explicit unknown outcomes. | |
| [ ] Medium: Keep OpenAPI synchronized with runtime routes | Keep OpenAPI synchronized with runtime routes. | Keeps documentation accurate for users and contract checks. | Generate paths from route definitions, or compare the static document with the built router. | Fail when a method, path, access rule, request, or response schema differs. | |
| [ ] Medium: Add real lint and accessibility checks | Add real lint and accessibility checks. | Improves frontend maintenance and release confidence. | Keep type checking. Add focused lint rules and one browser accessibility pass after core workflows work. | Run lint, keyboard, focus, landmark, contrast, zoom, and reduced-motion checks. | |
| [ ] Medium: Reconcile the v1 roadmaps | Reconcile the v1 roadmaps. | Prevents checked plans from hiding unfinished production work. | Mark an item complete only after its production path and acceptance check pass. Record the exact command and result. | Compare each checked row with source wiring, persistent services, and an acceptance result. | |

Exit condition: contract, distributed recovery, documentation, quality, and roadmap checks describe the real production system.

## Deferred work

| Task | Description | Impact | How to implement | How to test | Test Result |
|---|---|---|---|---|---|
| [ ] Low: Add a third theme only after v1 acceptance | Add a third theme only after v1 acceptance. | Adds optional value after core workflows work. | Reuse semantic tokens. Do not change component markup. | Test contrast and pre-paint selection. | |
| [ ] Low: Add localization only after locales are approved | Add localization only after locales are approved. | Avoids a translation system without product requirements. | Select one small mechanism when a second locale is approved. | Test fallback strings and stored locale selection. | |
