# Management v1 release

This runbook covers the Console control plane. It does not authorize changes to
managed PostgreSQL clusters. Use one release tag for the UI, API, Console DB,
and Automation components described in `MANAGEMENT_VERSION_SET.json`.

## Release gate

Release only a commit whose **Management v1** GitHub Actions workflow passed.
The gate verifies the stock Community `2.9.0` migration, guarded operation and
query-analytics contracts, backup contracts, API and storage behavior, the UI
build, and the management browser journey.

## Back up

Before upgrading:

1. Stop starting new Console operations and wait for queued or running
   operations to finish.
2. Record the release tag, Compose files, environment configuration, and
   externally stored `PG_CONSOLE_ENCRYPTIONKEY`. The key is required to decrypt
   existing secret records and must not be written into the database dump.
3. Back up the Console database:

   ```sh
   docker compose exec -T autobase-console-db \
     pg_dump -U postgres -d postgres --format=custom > console-before-upgrade.dump
   pg_restore --list console-before-upgrade.dump
   ```

4. Record each managed cluster's Patroni topology and Console health snapshot.
   Confirm pgBackRest repository reachability, a current completed backup, WAL
   continuity, one scheduler owner, no conflicting lock, and restore-test
   evidence. Database availability alone is not proof of recoverability.

## Upgrade directly from Community 2.9.0

1. Select one tested release tag. Pin `console_ui`, `console_api`,
   `console_db`, and the API's `PG_CONSOLE_DOCKER_IMAGE` Automation image to
   that same tag.
2. Preserve the existing Console database volume, environment configuration,
   authorization token, and encryption key.
3. Pull and start the selected version set:

   ```sh
   docker compose pull
   docker compose up -d
   ```

The API applies ordered forward migrations when it starts. Do not reset the
Console database volume and do not run an intermediate fork release. Import and
startup discovery remain read-only against managed clusters.

## Verify

1. Require all Console containers to become healthy and
   `GET /api/v1/version` to report the selected release.
2. Confirm projects, environments, settings, clusters, servers, inventory,
   connection metadata, encrypted secrets, and prior operation history remain
   present.
3. Confirm the migration head matches `MANAGEMENT_VERSION_SET.json`.
4. Confirm no operation was created by the upgrade and no managed-cluster
   package, configuration, service, credential, PostgreSQL, Patroni, DCS,
   routing, backup, or monitoring state changed.
5. Open cluster overview, operation detail, and query performance. Verify
   topology, routing, backup freshness and restore evidence, redacted audit
   data, query coverage, and any rollout-required state.

## Roll back

Migrations are forward-only. Do not start an older API against an upgraded
Console database.

1. Stop the upgraded Console.
2. Restore `console-before-upgrade.dump` into an empty Console database volume.
3. Restore the preserved environment configuration and encryption key.
4. Pin all four components to the previous tested release tag and start the
   Console.
5. Repeat the verification steps and compare the recorded managed-cluster
   topology and health snapshot.

Rollback restores only the Console control plane. It must not restore over,
restart, or reconfigure a running managed PostgreSQL cluster.
