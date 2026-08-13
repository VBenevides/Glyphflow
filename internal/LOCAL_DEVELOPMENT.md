# Local services

Start PostgreSQL and NATS JetStream from the repository root:

```sh
docker compose up -d
```

The development connection values are:

```text
DATABASE_URL=postgres://glyphflow:glyphflow@localhost:5432/glyphflow?sslmode=disable
NATS_URL=nats://localhost:4222
```

Stop the services with `docker compose down`. Add `-v` only when the local
database and queue data can be discarded.
