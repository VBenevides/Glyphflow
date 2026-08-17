# Glyphflow v2 UI audit

Date: 2026-08-17  
Runtime: `./dev_run.sh`, Chromium headless, 1440×1000 unless noted

## Coverage

- 27 routes: dashboard, task/schedule/run flows, runners/pools/enrollment, resources, audit, global variables, all administration pages, all account pages, and invalid record IDs.
- Dialogs/actions checked: appearance, audit details, authentication settings confirmation, custom-role deletion, session revocation, run cancellation, custom-role creation, and global-variable deletion.
- Visual review included label/control alignment, inline versus stacked fields, control widths, table/action alignment, spacing, overflow, and responsive behavior on every captured page.
- Test setup: bootstrap admin `admin@example_domain.com`; created a test user and custom role through the API; created another custom role through the UI. No passwords are recorded here.
- Runtime result: 0 browser console errors. Invalid IDs resolve to the Not found state.

Evidence is in [`runtime-report.json`](./runtime-report.json) and the screenshots in this directory.

## Findings

### P1 — Admins cannot create users, create admins, or assign roles

Page: `/admin/users`, `/admin/users/:id`, `/admin/roles`

The Users page only offers `Disable` and `Details`. Its empty-state copy says to “Create or provision a user” but has no create action. User details are read-only and provide no role assignment or promotion flow. A custom role can be created, but there is no way to assign it to a user. The audit had to use the API to create the test user and role.

Impact: normal onboarding and access administration cannot be completed in the product.

Recommendation: add `Create user`, role selection/assignment, and an explicit admin-role guard. If provisioning is intentionally external, change the empty-state copy to explain the external workflow.

### P1 — Mobile task editor is clipped and horizontally scrolls

Page: `/tasks/new` at 320px wide. See [`task-new-320.png`](./task-new-320.png).

The sidebar and task form compete for the viewport. Only a narrow slice of the form is visible, the page has a horizontal scrollbar, and labels/controls are clipped. This prevents practical task creation on a phone-sized viewport.

Recommendation: ensure the mobile drawer is closed by default, keep the main content at `width: 100%`, and remove child horizontal overflow at widths below 768px.

### P1 — Account sessions do not show useful session metadata

Page: `/account/sessions`. See [`homeaccount_sessions.png`](./homeaccount_sessions.png).

Every non-current session renders as `— · Session`, while the admin user-session view shows timestamps and expiry dates for the same type of records. Users cannot identify which session they are revoking.

Recommendation: render user agent/device plus last-seen, created, or expiry time; show an explicit “Unknown” when the API does not provide a field.

### P1 — Global-variable deletion has no confirmation

Page: `/global-variables`

The Delete button calls the removal action directly. The audit click deleted the existing `PYTHON_PATH_LINUX` variable immediately; the audit had to restore it through the API. This is especially risky because variables can be referenced by tasks and schedules.

Recommendation: use the existing destructive confirmation dialog and include the reference count/affected tasks in the warning.

### P2 — Authentication form controls are misaligned and oversized

Page: `/admin/auth`. See [`homeadmin_auth.png`](./homeadmin_auth.png).

The `Enable password login` and `Allow password registration` labels are left aligned, but their checkboxes are stretched across the form row and appear far to the right. `Default role` is stacked above its select instead of sharing a row with it, and the select expands to nearly the full form width.

Recommendation: make checkbox inputs intrinsic-sized and align each checkbox immediately beside its label using an inline-flex row. Put `Default role` and its select on one line, with a smaller consistent select width (and a deliberate narrow-screen breakpoint).

### P2 — Run filters wrap awkwardly

Page: `/runs`. See [`homeruns.png`](./homeruns.png).

The `To` date field drops to a second row while the filter card retains a large empty area. The resulting filter layout looks accidental and makes the date range harder to scan.

Recommendation: use a responsive grid with deliberate breakpoints, keeping `From` and `To` together.

### P2 — Empty states use more vertical space than necessary

Pages: `/resources`, `/admin/sso`, and other empty collections. See [`homeresources.png`](./homeresources.png) and [`homeadmin_sso.png`](./homeadmin_sso.png).

The empty-state panel is a large fixed-looking block for two short lines, pushing the useful page controls farther from the header.

Recommendation: reduce empty-state minimum height/padding on collection pages while retaining clear hierarchy and the primary create action.

## Verified working

- Global-variable autocomplete filters and shows the value in the option.
- Hover/focus on a complete environment-variable reference shows its value.
- Command argument labels appear only for populated lines.
- Final Command resolves `$ENV:PYTHON_PATH_LINUX` and renders the runner-style quoted string: `"/usr/sbin/python" "asd"`.
- Appearance and confirmation dialogs opened and closed successfully.
- Invalid record IDs eventually show a correlated Not found state.
