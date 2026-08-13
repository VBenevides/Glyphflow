# Security baseline

- PostgreSQL is reachable only from control-plane hosts.
- Workers have no inbound listener and no database configuration.
- Queue connections use mutual TLS and per-runner subjects.
- Orders and events use Ed25519 signatures with separate domains.
- Enrollment tokens are random, hashed at rest, expire, and are single-use.
- Private keys originate on the worker and are never placed in installers.
- Logs redact passwords, tokens, secrets, authorization values, and private keys.
- Commands use argument arrays, allowed working-directory roots, restricted
  service identities, bounded output, and resource ceilings.

Incident response: revoke the runner key and certificate, disable its queue
subjects, preserve audit records, rotate affected credentials, and restore only
from a verified backup.
