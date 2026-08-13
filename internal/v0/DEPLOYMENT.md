# Deployment baseline

Development uses `compose.yaml`. A single-node production deployment runs one
control-plane process, PostgreSQL, and JetStream. Availability deployment adds
control-plane replicas behind the same API endpoint, a three-node JetStream
cluster, and PostgreSQL failover/backup tooling. Workers remain outbound-only.

Before rollout, verify certificates, clocks, firewall rules, queue permissions,
database backups, SBOM/signatures, and the recovery runbook.
