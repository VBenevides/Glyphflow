# Glyphflow prioritized TODO

All items below have runnable test evidence.

## Security

- [x] **High: Keep browser tokens out of JSON responses**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run 'Cookie|Docs|AuthService'` — PASS; tokens are only in HttpOnly cookies.
  - Commit: `1d25561` (`feat(controlplane): Complete durable execution and API safeguards`).
- [x] **High: Remove signing and connection secrets from PostgreSQL config**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/config ./internal/store` — PASS; config allowlist and external-secret validation pass.
  - Commit: `1d25561`.
- [x] **High: Authenticate runner key registration and heartbeats**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api ./internal/controlplane ./internal/store` — PASS.
  - Commit: `1d25561`.
- [x] **High: Bound all HTTP request bodies**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run TestRequestBodiesAreBoundBeforePublicAndAuthenticatedHandlers` — PASS.
  - Commit: `1d25561`.
- [x] **High: Make security configuration fail closed**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/config` — PASS.
  - Commit: `1d25561`.
- [x] **Medium: Block OIDC server-side request forgery**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run 'OIDCRejectsPrivateEndpoints|OIDCCallback'` — PASS.
  - Commit: `1d25561`.
- [x] **Medium: Stop returning internal errors**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run 'Infrastructure|Audit'` — PASS; responses are stable and audit records retain details.
  - Commit: `1d25561`.
- [x] **Medium: Bound authentication rate-limit state**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/platform ./internal/api` — PASS.
  - Commit: `1d25561`.
- [x] **Low: Return a public OIDC provider projection**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run OIDCProviderPublicProjection` — PASS.
  - Commit: `1d25561`.

## Bugs

- [x] **Critical: Implement real cancellation**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./internal/api ./internal/worker ./internal/store` — PASS; cancellation is durable, signed, attempt-specific, and completion-race safe.
  - Commit: `1d25561`.
- [x] **High: Make retry and reconciliation dispatch new attempts**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/platform ./internal/store ./internal/api` — PASS.
  - Commit: `1d25561`.
- [x] **High: Repair worker restart recovery**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./internal/worker` — PASS.
  - Commit: `1d25561`.
- [x] **High: Enforce legal state-event order**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./internal/store ./internal/controlplane` — PASS.
  - Commit: `1d25561`.
- [x] **High: Complete the OIDC callback contract**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run OIDC` — PASS.
  - Commit: `1d25561`.
- [x] **High: Implement authenticated OIDC identity linking**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run 'OIDCLinkChallenge|OIDCCallback'` — PASS.
  - Commit: `1d25561`.
- [x] **High: Repair the tagged integration suite**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -tags=integration ./internal/integration` — PASS; PostgreSQL/TLS NATS checks skipped because integration variables were unset.
  - Commit: `8a04f39` (`test(controlplane): Repair tagged integration suite`).
- [x] **Medium: Return complete task versions to the editor**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/store ./internal/api` — PASS; active-version fields are returned and omitted fields are preserved.
  - Commit: `1d25561`.
- [x] **Medium: Honor collection filters and pagination**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run 'Pagination|Filter'` — PASS.
  - Commit: `1d25561`.
- [x] **Medium: Correct dashboard data**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api ./internal/store` and `cd frontend && npm test` — PASS.
  - Commit: `1d25561`.
- [x] **Medium: Make log URLs follow the API deployment contract**
  - Test: `cd frontend && npm run typecheck && npm run build` — PASS; API, streams, downloads, and OIDC use same-origin paths.
  - Commit: `1d25561`.
- [x] **Medium: Make local user provisioning atomic**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api ./internal/store` — PASS.
  - Commit: `1d25561`.
- [x] **Low: Return multiple schedule preview occurrences**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run TestPreviewOccurrencesReturnsFiveIncreasingTimes` — PASS.
  - Commit: `1d25561`.

## New Features

- [x] **High: Execute task environment variables**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/worker ./internal/controlplane ./internal/api` and `cd frontend && npm test` — PASS.
  - Commit: `1d25561`.
- [x] **High: Execute remaining immutable task specifications**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -race ./internal/store ./internal/controlplane ./internal/worker` — PASS; selectors, secret references, retries, ambiguity, resources, and resolved digests are signed.
  - Commit: `1d25561`.
- [x] **High: Enforce schedule policies**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/controlplane ./internal/store ./internal/api` — PASS.
  - Commit: `1d25561`.
- [x] **High: Add a real run attempt timeline**
  - Test: `cd frontend && npm test && npm run typecheck` — PASS.
  - Commit: `8dec0dc` (`feat(frontend): Render run attempt timeline`).
- [x] **High: Add restart acceptance coverage**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test -tags=integration ./internal/integration` — PASS; external PostgreSQL/TLS NATS execution is skipped without its variables.
  - Commit: `1d25561`.
- [x] **Medium: Add global environment variables**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/platform ./internal/api ./internal/store` and `cd frontend && npm test` — PASS.
  - Commit: `1268611` (`feat(controlplane): Add global environment variables`) and `1d25561`.
- [x] **Medium: Add dead-letter inspection and audited retry**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/queue ./internal/controlplane ./internal/api` — PASS; bounded NATS dead-letter publication, redelivery, and audited retry paths pass.
  - Commit: `1d25561`.
- [x] **Medium: Add explicit schedule enable and disable actions**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run TestScheduleEnableDisableDoesNotCreateVersion` — PASS.
  - Commit: `cfe8033` (`fix(controlplane): Keep schedule state outside versions`).
- [x] **Medium: Wire retention and health metrics**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/platform ./internal/api` — PASS; bounded retention and low-cardinality metric snapshots pass.
  - Commit: `1d25561`.

## Enhancements

- [x] **Medium: Remove production in-memory fallbacks**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./cmd/controlplane ./internal/api ./internal/store` — PASS; production wiring uses PostgreSQL repositories.
  - Commit: `1d25561`.
- [x] **Medium: Add transaction and timeout conventions**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go vet ./... && GOCACHE=/tmp/glyphflow-gocache go test -race ./...` — PASS.
  - Commit: `1d25561`.
- [x] **Medium: Keep only cron schedules**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/controlplane ./internal/store` — PASS; cron-only migration and contract pass.
  - Commit: `3f22cff` (`refactor(controlplane): Keep schedules cron-only`).
- [x] **Medium: Hide inapplicable form fields**
  - Test: `cd frontend && npm test && npm run lint` — PASS.
  - Commit: `81903ea` (`feat(frontend): Improve task and schedule forms`).
- [x] **Low: Consolidate route contract checks**
  - Test: `cd backend && GOCACHE=/tmp/glyphflow-gocache go test ./internal/api -run 'Route|OpenAPI|Docs'` — PASS.
  - Commit: `1d25561`.
- [x] **Low: Replace remaining raw task JSON fields with key/value rows**
  - Test: `cd frontend && npm test && npm run typecheck` — PASS.
  - Commit: `1d25561`.
- [x] **Low: Improve operator formatting**
  - Test: `cd frontend && npm run lint && npm run build` — PASS; native date/time controls, server filters, and pagination build cleanly.
  - Commit: `1d25561`.
- [x] **Low: Keep the current React stack**
  - Test: `cd frontend && npm test && npm run typecheck && npm run build` — PASS; no framework dependency was added.
  - Commit: `1d25561`.
