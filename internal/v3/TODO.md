# Audit TODO

Generated from the 2026-08-18 project audit at baseline `d269f1c`.

## Features

- [ ] Complete one canonical API contract
  - Importance Level: High
  - Description: Reconcile the legacy `501 Not Implemented` route placeholders, route registry, OpenAPI document, and current frontend/admin handlers.
  - Test Description: Add live contract tests for every documented method and verify the frontend-used routes against the mounted handlers.
  - Test Result: Baseline tests pass; fix not tested.
  - Commit Hash: Not committed

- [ ] Add an end-to-end release acceptance test
  - Importance Level: High
  - Description: Start PostgreSQL, NATS, control plane, frontend proxy, and a worker, then verify login, enrollment, task execution, events, output, and restart recovery.
  - Test Description: Run the acceptance workflow in CI or an isolated Compose environment from built artifacts.
  - Test Result: Not run.
  - Commit Hash: Not committed

- [ ] Expose operational metrics
  - Importance Level: Medium
  - Description: Expose the existing internal runtime counters through a documented, protected or network-restricted metrics endpoint.
  - Test Description: Verify metric names, access control, sensitive-label exclusion, and endpoint availability in the release smoke test.
  - Test Result: Not run.
  - Commit Hash: Not committed

- [ ] Decide whether non-Windows TUI tray support is required
  - Importance Level: Low
  - Description: The TUI tray integration works on Windows; Linux and macOS currently use the terminal without a tray implementation.
  - Test Description: Exercise the selected platform integrations or document Windows-only tray support as the supported boundary.
  - Test Result: Not run.
  - Commit Hash: Not committed

## Security Patches

- [ ] Fail closed for deployment CORS and transport settings
  - Importance Level: Medium
  - Description: Require explicit origins, HTTPS, NATS TLS, and bootstrap credentials outside development; reject wildcard CORS when credentialed requests are enabled.
  - Test Description: Add configuration tests for production defaults, wildcard rejection, and explicit development-only overrides.
  - Test Result: Baseline tests pass; fix not tested.
  - Commit Hash: Not committed

- [ ] Apply security headers in the Docker Nginx image
  - Importance Level: Medium
  - Description: Mirror the headers from `frontend/_headers` in `build/nginx.conf`, with compatible policies for API and documentation paths.
  - Test Description: Build the image and assert CSP, referrer, frame, MIME, and permissions headers on static, API, and docs responses.
  - Test Result: Not run.
  - Commit Hash: Not committed

- [ ] Prevent OIDC DNS rebinding during outbound requests
  - Importance Level: Medium
  - Description: Pin or revalidate resolved addresses at connection time and preserve private-address and redirect protections.
  - Test Description: Add HTTP client tests that change DNS results between validation and connection and verify private targets are rejected.
  - Test Result: Baseline tests pass; fix not tested.
  - Commit Hash: Not committed

- [ ] Make the security verification script self-contained
  - Importance Level: Low
  - Description: Check `govulncheck`, npm audit, and SBOM tooling before execution, provide actionable errors, and use current release artifact paths.
  - Test Description: Run the script with each tool missing and installed, then verify all checks execute and failures are reported distinctly.
  - Test Result: Baseline run blocked at missing `govulncheck`.
  - Commit Hash: Not committed

## Bug Fixes

- [ ] Wire the production OIDC client-secret resolver
  - Importance Level: High
  - Description: Configure `OIDCService.SetSecretResolver` during control-plane startup or reject secret references until a deployment-backed resolver is available.
  - Test Description: Configure a confidential OIDC provider and verify authorization-code exchange succeeds after a restart.
  - Test Result: Not run.
  - Commit Hash: Not committed

- [ ] Update shared auth state after saving a display name
  - Importance Level: Medium
  - Description: Apply the updated `/api/v1/me` response to the auth context so the sidebar changes without a full application reload.
  - Test Description: Save a display name and assert both the account page and sidebar render the new value.
  - Test Result: Baseline tests pass; fix not tested.
  - Commit Hash: Not committed

- [ ] Embed `VERSION` in release-check binaries
  - Importance Level: Medium
  - Description: Pass the root version through linker flags in `scripts/release-check.sh` and verify binaries work outside the repository checkout.
  - Test Description: Build the release artifacts, run their version output from a temporary working directory, and compare it with `VERSION`.
  - Test Result: Baseline tests pass; fix not tested.
  - Commit Hash: Not committed

- [ ] Keep API documentation aligned with runtime handlers
  - Importance Level: Medium
  - Description: Remove independent route definitions or add complete live checks for methods, status codes, and response schemas.
  - Test Description: Iterate over the OpenAPI operations and compare each result with the mounted handler contract.
  - Test Result: Baseline tests pass; fix not tested.
  - Commit Hash: Not committed
