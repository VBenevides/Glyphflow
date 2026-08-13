# Operations baseline

The control plane is the only PostgreSQL client. Workers use outbound HTTPS
enrollment and JetStream connections, store accepted orders and event outbox
records locally, and never receive database credentials.

Required production checks are executable from the repository:

```sh
GOCACHE=/tmp/glyphflow-go-cache go test ./backend/...
docker compose config -q
```

Backups must include PostgreSQL public state and exclude plaintext private
keys. Recovery targets are recorded per deployment; the default operational
target is RTO 30 minutes and RPO 5 minutes.
