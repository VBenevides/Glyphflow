# v2 UI improvement TODO

Based on [`findings.md`](./findings.md).

## P1

- [ ] Add admin UI to create users, create/promote admins, and assign or revoke roles.
- [x] Fix mobile task-editor layout at widths below 768px: closed drawer, full-width content, no horizontal clipping.
  - Verification: `npm test -- src/layout.test.ts`
  - Result: PASS — 3 layout contract tests passed; desktop/mobile geometry checks passed during implementation.
  - Implementation commit: `5efa0b8bb8237b41d01e730df607c93682136755`
- [x] Show useful metadata on `/account/sessions` such as device/user agent and last-seen or expiry time.
  - Verification: `npm test -- src/account-pages.test.ts`
  - Result: PASS — 3 tests passed.
  - Implementation commit: `b197564675ee53ea8ec91498e34f480a39f42a0c`
- [x] Add confirmation before deleting global variables, including reference/impact information.
  - Verification: `npm test -- src/global-variables-page.test.ts`
  - Result: PASS — 2 tests passed.
  - Implementation commit: `807e52dde420e2aa97bdfe9554ff3e5b1d88cc06`

## P2

- [x] Align authentication checkboxes immediately beside their labels.
  - Verification: `npm test -- src/layout.test.ts`
  - Result: PASS — 3 layout contract tests passed.
  - Implementation commit: `5efa0b8bb8237b41d01e730df607c93682136755`
- [x] Put `Default role` and its dropdown on one line; use a smaller consistent dropdown width with a mobile breakpoint.
  - Verification: `npm test -- src/layout.test.ts`
  - Result: PASS — 3 layout contract tests passed.
  - Implementation commit: `5efa0b8bb8237b41d01e730df607c93682136755`
- [x] Rework `/runs` filters so `From` and `To` remain together at desktop widths.
  - Verification: `npm test -- src/layout.test.ts`
  - Result: PASS — 3 layout contract tests passed.
  - Implementation commit: `5efa0b8bb8237b41d01e730df607c93682136755`
- [x] Reduce excessive empty-state height and padding on collection pages.
  - Verification: `npm test -- src/layout.test.ts`
  - Result: PASS — 3 layout contract tests passed.
  - Implementation commit: `5efa0b8bb8237b41d01e730df607c93682136755`

## Verification

- [ ] Run the full route/dialog audit at desktop and mobile widths.
- [ ] Confirm no console errors, clipped controls, accidental destructive actions, or broken primary buttons.
