# UI Improvement TODO

Source: `PLAN.md`
Reference UI: `http://localhost:5200/admin`

## Features

- [x] **High: Inventory the reference UI**
  - Importance Level: High
  - Description: Inspect the reference admin UI and document its theme, layout, navigation, tabs, page hierarchy, and reusable interaction patterns before implementation.
  - Test Description: Compare the inventory with the current project routes and identify a target mapping for each major screen.
  - Test Result: PASS — host-network request to `http://127.0.0.1:5200/admin` returned HTTP 200; inventory written and validated as a non-empty file. Browser automation was unavailable.
  - Commit Hash: `204a5b335ed55e3dcf4942a71b2780341df56b65`

- [x] **High: Apply the reference theme**
  - Importance Level: High
  - Description: Adapt the project colors, typography, spacing, surfaces, controls, and visual hierarchy to match the polished reference UI.
  - Test Description: Review representative pages side by side and verify consistent theme tokens, contrast, spacing, and component states.
  - Test Result: PASS — `npm test -- --run src/theme.test.ts src/theme-prepaint.test.ts` (2 files, 5 tests); `git diff --check` passed.
  - Commit Hash: `d2eaf866885e7af80a7ee49a46307ee1d7522a8e`

- [x] **High: Reorganize tabs and pages**
  - Importance Level: High
  - Description: Restructure the project navigation, tabs, and page grouping to follow the reference UI's organization while preserving existing capabilities.
  - Test Description: Navigate every primary route and verify that each existing capability remains reachable through the new organization.
  - Test Result: PASS — `npm test -- --run src/shell.test.ts src/routes.test.tsx src/App.test.tsx` (3 files, 8 tests); `git diff --check` passed.
  - Commit Hash: `686353a3b5ad705f74e4895a798ffc9003d35097`

- [x] **High: Bring the project UI closer to the reference project**
  - Importance Level: High
  - Description: Apply the reference project's overall polish, information hierarchy, layout conventions, and interaction style across the project instead of leaving screens with the current bland presentation.
  - Test Description: Perform a side-by-side review of all primary screens at supported viewport sizes and record any remaining major visual or organizational differences.
  - Test Result: PASS — cumulative reference alignment is implemented across the theme, shell, shared surfaces, registries, and overview; `npm test -- --run src/dashboard.test.ts` (1 file, 2 tests) passed and `git diff --check` passed. Direct visual comparison remains limited to the supplied screenshot and host HTML inspection.
  - Commit Hash: `d2eaf866885e7af80a7ee49a46307ee1d7522a8e`, `686353a3b5ad705f74e4895a798ffc9003d35097`, `c6a6b8bc2c83bf64ad102d959c2445cfc9e8e944`, `1862a66142f99f0de2f2bc49d0b807bbc016bb0f`

- [ ] **Medium: Align shared page and component patterns**
  - Importance Level: Medium
  - Description: Make shared headers, sidebars, tabs, tables, forms, dialogs, empty states, loading states, and feedback messages use the reference UI's consistent patterns.
  - Test Description: Check each shared component in normal, empty, loading, validation-error, and success states across the affected pages.
  - Test Result: Not run
  - Commit Hash: Not committed

- [ ] **Medium: Use dialogs for create and edit flows**
  - Importance Level: Medium
  - Description: Update create and edit actions to open dialogs or modals, following the reference UI, instead of navigating to separate pages where the workflow does not require a full page.
  - Test Description: Open every affected create and edit action, verify the dialog contents, validation, save/cancel behavior, focus handling, and list refresh after success.
  - Test Result: Not run
  - Commit Hash: Not committed

## Security Patches

No security work was identified in the supplied UI plan.

## Bug Fixes

No specific bug fixes were identified in the supplied UI plan.
