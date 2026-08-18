# Reference Admin UI Inventory

Source: `http://localhost:5200/admin` (AI Factory admin console)

## Observed structure

- A fixed left admin sidebar groups pages under Admin, System, and Monitoring.
- The sidebar includes compact icon-led links, expandable groups, a brand/action area, a return-to-user-panel action, local settings, and sign-out.
- The main area uses a page heading with a short description, followed by metric cards, grouped content sections, and dense registries or activity lists.
- Registry pages use a search/filter row above rounded, bordered tables with compact headings and pill-style statuses or permissions.

## Observed visual system

- Light mode uses a pale lavender canvas and slightly lighter lavender sidebar/surfaces.
- Cards and tables use thin lavender borders, rounded corners, restrained shadows, and compact spacing.
- Headings use dark navy text; supporting text uses muted gray-purple text.
- Primary actions and active navigation use saturated purple; metric icons use small colored outline treatments.
- The layout is responsive and keeps dense data readable through compact rows and horizontal overflow where needed.

## Current-project mapping

| Reference pattern | Glyphflow target |
| --- | --- |
| Expandable admin navigation | `frontend/src/shell.tsx` and `frontend/src/routes.tsx` |
| Lavender theme, surfaces, cards, tables | `frontend/src/index.css` and `frontend/src/theme.ts` |
| Shared dialog, button, input, table, metric, and state patterns | `frontend/src/components.tsx` |
| Registry and CRUD workflows | task, schedule, runner-pool, and run pages |

Direct browser automation was not available to the coding environment; the inventory is based on host-network HTML inspection and the supplied reference screenshot.
