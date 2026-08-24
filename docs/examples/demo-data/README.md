# Demonstration data

Use a separate local Compose project for documentation work. This creates a
new database from scratch and keeps the demo volumes separate from the default
development environment.

```bash
COMPOSE_PROJECT_NAME=glyphflow-docs docker compose down -v
COMPOSE_PROJECT_NAME=glyphflow-docs \
GLYPHFLOW_BOOTSTRAP_EMAIL=owner@northstar.test \
GLYPHFLOW_BOOTSTRAP_PASSWORD='Northstar-demo-password-123!' \
GLYPHFLOW_SYSTEM_ADMINS=owner@northstar.test \
./dev_run.sh
```

The command uses only fake local data. Do not point it at a production
database, real identity provider, real host, or real credential.

## Seed catalog

Create the following records through the local console as the flows require
them:

| Type | Fake value |
| --- | --- |
| Admin | Olivia Mercer — `owner@northstar.test` |
| Operator | Mateo Silva — `operator@northstar.test` |
| Runner pool | `Northstar Linux` |
| Runner | `northstar-runner-01` on `atlas-demo-01` |
| Runner capability | `platform=linux`, `architecture=amd64` |
| Task | `Customer Export Check` |
| Task | `Nightly Invoice Archive` |
| Task | `Warehouse Snapshot` |
| Task | `Invoice Archive Failure Drill` |
| Schedule | `Nightly Customer Export` |
| Schedule | `Hourly Warehouse Snapshot` |
| Resource | `warehouse-report-lock` |
| Global variable | `ARCHIVE_ROOT=/srv/demo/archive` |

Use fake commands such as `/bin/echo customer-export-complete` and
`/bin/sh -c 'exit 23'` only when the flow requires a failure. Keep command
arguments separate in the task editor.

## Reset and review

Run the reset command again before a new screenshot session. Review every
screen and image for real credentials, production identifiers, private keys,
tokens, and non-fake hostnames before publishing the documentation.
