# Automation credential recovery

Use this after an operation stopped partway through because a PostgreSQL or Patroni credential was rejected.

1. Confirm the operation is terminal in Console and no other operation holds the cluster lock. Do not rerun it yet.
2. Check cluster health, current leader, replica streaming, pending restarts, and temporary Patroni tags. Restore tags only after the node is healthy and its original role is known.
3. On every PostgreSQL node, compare hashes of the active Patroni PostgreSQL authentication configuration and `.pgpass` without printing either file. If the hashes differ, stop and perform a coordinated credential rotation outside Console.
4. If the hashes match and application traffic does not use the `postgres` role, reset that role on the current leader through local peer authentication. Keep the replacement value out of shell history and logs.
5. Verify the replacement over TCP on every PostgreSQL node. Separately verify the replication role with a replication connection and Patroni REST credentials against `/config`.
6. In **Settings → Secrets**, create or update separate password secrets for PostgreSQL superuser, PostgreSQL replication, and Patroni REST API. In the cluster **Access** page, attach all three together.
7. Run a fresh preflight. Retry only when all required credential checks pass.
8. After completion, verify topology, replica lag, pending restarts, Query Analytics state, and backup evidence.

Console attaches existing values; it does not rotate live service credentials. Restore/PITR validates all three bindings before an isolated bootstrap, then authenticates every restored target before final verification.
