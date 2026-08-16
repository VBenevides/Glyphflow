# Glyphflow v2 review TODO

Items are ordered by severity within each category. Each item maps to `REPORT.md`.

## Security

- [ ] **Medium — SEC-01: Upgrade `golang.org/x/text` to v0.39.0 or later**
  - Run `govulncheck ./...` and the complete backend suite.
- [ ] **Medium — SEC-02: Signal audit persistence failures**
  - Use the existing structured logger and metrics. Add one failure-path test.
- [ ] **Medium — SEC-03: Reduce the system `user` role to least privilege**
  - Move task, resource, and runner management into an explicit operator role.
- [ ] **Low — SEC-04: Add Go and npm vulnerability scans to the security gate**
  - Reuse `govulncheck` and `npm audit --omit=dev`.

## Bugs

- [ ] **High — BUG-01: Restore the mobile menu and drawer scrim**
  - Remove the duplicate CSS hide rule. Test 320 and 390 pixels.
- [ ] **High — BUG-02: Allow parent-cascade deletion of schedule versions**
  - Add a migration that rejects version updates, not parent-driven deletes.
- [ ] **Medium — BUG-03: Restore CSRF state after logout**
  - Make logout followed by login work without a reload.
- [ ] **Medium — BUG-04: Align run-log rendering with `logs.read`**
  - Grant the permission or hide the panels. Stop repeated 403 reconnects.
- [ ] **Medium — BUG-05: Contain task editor tables**
  - Reuse `.gf-table-wrap` for selectors, environment values, and secret references.
- [ ] **Medium — BUG-06: Give schedule controls accessible names**
  - Use explicit labels and valid combobox relationships.
- [ ] **Medium — BUG-07: Isolate PostgreSQL test data**
  - Fix cleanup order, assert cleanup errors, and use a fresh database.
- [ ] **Low — BUG-08: Repair the README roadmap link**
  - Link to `internal/v2-review/TODO.md`.

## New Features

- [ ] **Medium — FEATURE-01: Show the placement blocker for waiting runs**
  - Reuse the dispatcher reason on run details.

## Enhancements

- [ ] **High — ENH-01: Add a focused browser acceptance suite**
  - Cover authentication, role routes, mobile navigation, task editing, run logs, and accessible names.
- [ ] **Medium — ENH-02: Gate releases on a migrated PostgreSQL suite**
  - Run it after unit tests in one clean database.
- [ ] **Medium — ENH-03: Complete the existing desktop tray matrix**
  - Test supported Linux and Windows environments from `internal/v2/TODO-2.md`.
- [ ] **Low — ENH-04: Remove deprecated Vite transform options**
  - Update the existing configuration during the next dependency update.
