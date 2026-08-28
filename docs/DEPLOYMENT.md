# Deployment contract

A release contains the existing Glyphflow application image, NATS, and
PostgreSQL as separate OCI images. `release-manifest.json` records the exact
registry digest for each image. `deployment-images.tar.gz` contains all three
images for an offline deployment; verify `SHA256SUMS` before loading it.

## Full deployment

The bundle contains `compose.yaml` and `compose.production.yaml`. Supply the
protected secret and TLS files required by the production overlay, then run:

```bash
docker load -i glyphflow-<version>-deployment-images.tar.gz
docker compose --env-file images.env -f compose.yaml -f compose.production.yaml up -d
```

Use one dedicated Compose project, PostgreSQL database, and NATS JetStream
authority per client. Keep PostgreSQL and NATS private. The production overlay
uses the service names `postgres` and `nats` and does not publish their ports.

## Partial deployment

Run PostgreSQL, NATS, and the control plane as separate containers on one
operator-created network. Attach the PostgreSQL and NATS containers to the
network with stable DNS names (`postgres` and `nats`), or put their reachable
TLS hostnames in the protected `DATABASE_URL_FILE` and `NATS_URL_FILE` values.
Use a dedicated database and NATS JetStream authority for every client.

Create the network and start the released control-plane image with:

```bash
docker network create glyphflow-client
GLYPHFLOW_NETWORK=glyphflow-client \
COMPOSE_PROJECT_NAME=glyphflow-client \
docker compose -f compose.partial.yaml up -d
```

Set the variables named by `compose.partial.yaml` in the deployment secret
manager or shell environment. The database URL must use `sslmode=verify-full`;
the profile sets `PGSSLROOTCERT=/run/secrets/postgres-ca` for the mounted CA.
Set `DATABASE_STORAGE_CAPACITY_BYTES` to a conservative byte budget for the
application database. The control plane compares PostgreSQL's database size
with this value for run admission, retention cleanup, and storage health; it
does not use the control-plane volume as a proxy for database capacity.
The NATS URL must use
`tls://` and the mounted client certificate, key, and CA. The service exposes
the control-plane HTTP listener on port 8080 and reports `/api/v1/healthz` and
`/api/v1/readyz`.

The partial profile runs `/usr/local/bin/glyphflow-controlplane` directly and
sets `GLYPHFLOW_DISABLE_NGINX=true`. This is the container mode for platforms
with private ingress, such as ACA: route the ingress to port 8080, set an
HTTPS `WEB_ORIGIN`, and keep the platform's TLS termination outside the image.

Workers must use an HTTPS `GLYPHFLOW_CONTROL_PLANE_URL` and a `tls://`
`GLYPHFLOW_NATS_ENDPOINT` (or equivalent enrollment values). Plain HTTP/NATS
endpoints are accepted only in development with
`ALLOW_INSECURE_TRANSPORT=true`. Do not share the
database, JetStream authority, signing key, or worker subject policy between
clients.
