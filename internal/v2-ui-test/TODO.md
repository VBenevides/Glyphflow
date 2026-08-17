# v2 UI improvement TODO

Based on [`findings.md`](./findings.md).

## P1

- [ ] Add admin UI to create users, create/promote admins, and assign or revoke roles.
- [ ] Fix mobile task-editor layout at widths below 768px: closed drawer, full-width content, no horizontal clipping.
- [x] Show useful metadata on `/account/sessions` such as device/user agent and last-seen or expiry time.
  - Verification: `npm test -- src/account-pages.test.ts`
  - Result: PASS — 3 tests passed.
  - Implementation commit: `b197564675ee53ea8ec91498e34f480a39f42a0c`
- [ ] Add confirmation before deleting global variables, including reference/impact information.

## P2

- [ ] Align authentication checkboxes immediately beside their labels.
- [ ] Put `Default role` and its dropdown on one line; use a smaller consistent dropdown width with a mobile breakpoint.
- [ ] Rework `/runs` filters so `From` and `To` remain together at desktop widths.
- [ ] Reduce excessive empty-state height and padding on collection pages.

## Verification

- [ ] Run the full route/dialog audit at desktop and mobile widths.
- [ ] Confirm no console errors, clipped controls, accidental destructive actions, or broken primary buttons.
