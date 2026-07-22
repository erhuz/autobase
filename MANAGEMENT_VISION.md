# Autobase Community Management Vision

## Purpose

Extend Autobase Community Console from a deployment and inventory interface into a safe day-2 management console for existing highly available PostgreSQL clusters.

The console should let internal engineers and PostgreSQL operators perform routine, well-defined operations without assembling commands over SSH. It should expose the behavior already available through Autobase Automation and Patroni while adding validation, serialization, confirmation, progress reporting, and auditability.

This project extends the MIT-licensed Community code. It preserves upstream attribution and does not copy or reproduce proprietary Enterprise code.

## Current Gap

Community Console can create or register clusters, display topology and connection information, refresh observed state, export inventory, and show deployment logs. Most ongoing management still requires Autobase CLI, Patroni commands, or direct SSH access.

That split creates avoidable operational risk:

- operators must reconstruct inventory, credentials, and command arguments;
- current state and intended change are not reviewed together;
- concurrent operations are not centrally prevented;
- preflight results, confirmations, output, and outcomes lack one durable audit trail;
- destructive commands remain too close to routine read-only inspection.

The goal is not arbitrary remote administration. The goal is a small set of guarded, repeatable PostgreSQL lifecycle operations.

## Architecture

```text
Operator
   |
   v
Console UI --> Console API --> Console DB and operation log
                    |
                    v
             Autobase Automation
                    |
                    v
        PostgreSQL, Patroni, DCS, routing, backups
```

Responsibilities remain separated:

- **Console UI** presents observed state, desired action, preflight results, confirmation, progress, and outcome.
- **Console API** authorizes requests, validates state, acquires a per-cluster operation lock, launches automation, and records audit events.
- **Console DB** stores inventory, operation state, sanitized parameters, logs, and outcomes.
- **Autobase Automation** remains the orchestration layer and reuses existing playbooks and roles.
- **Patroni and the DCS** remain authoritative for PostgreSQL topology and leadership.

The browser must never connect directly to database hosts or receive SSH credentials. Host access belongs to the API-controlled automation runner.

## Safety Principles

### Passive import

Import and discovery must be read-only against the managed cluster. Registration may store discovered inventory in Console DB, but it must not deploy packages, rewrite configuration, restart services, change credentials, or alter PostgreSQL, Patroni, DCS, routing, backup, or monitoring state.

Discovery should report differences between observed state and Console assumptions before management is enabled.

### Explicit change contract

Every mutating operation must provide:

1. current observed state;
2. requested target state;
3. preflight checks and blockers;
4. exact planned operation and affected nodes;
5. explicit confirmation proportional to risk;
6. one per-cluster operation lock;
7. durable audit and operation logs.

No operation should silently turn an omitted form value into a configuration change.

### Topology-aware guards

Leader and replica roles must be refreshed immediately before topology-sensitive work. Switchover, restart, replica reinitialization, node removal, and restore must stop when their safety conditions no longer hold.

At minimum, guards must prevent:

- reinitializing or removing the current leader as if it were a replica;
- reducing healthy membership below the operation's required redundancy;
- starting a second cluster operation while one is active;
- performing rolling work without a healthy failover candidate;
- restoring into a running cluster without a dedicated recovery workflow.

### Recoverable execution

Operations should be idempotent where Autobase permits it, preserve completed progress, stop on failed safety checks, and present the operator with the failure, current cluster state, and safe next action. Arbitrary shell execution is outside the product surface.

## V1: Safe Cluster Operations

### Health and topology

Provide one cluster view covering:

- leader, replicas, state, timeline, replication lag, and pending restart;
- Patroni and DCS reachability and membership;
- routing/load-balancer health and target roles;
- pgBackRest repository status, backup age, WAL archive state, and active locks;
- current operation, latest completed operation, and unresolved failure.

Health must distinguish an available database from a recoverable database. A healthy Patroni topology must not hide stale or unverified backups.

### Planned switchover

Allow a planned Patroni switchover only when the leader and selected candidate are healthy, the candidate is sufficiently caught up, DCS is reachable, and no conflicting operation exists. Show expected routing impact and verify the new leader and replicas after completion.

Forced or ambiguous failover is deferred until stronger emergency-operation policy exists.

### Reload and rolling restart

Expose configuration reload separately from restart. Rolling restart must process replicas individually, preserve a healthy failover candidate, perform a controlled switchover when required, and verify topology and routing after each stage.

### Guarded replica reinitialization

Permit reinitialization only for a confirmed replica. Require another healthy cluster member, display that local replica data will be replaced, identify the clone source and method, and verify streaming state and lag after completion.

### Backup management

Provide pgBackRest-aware management:

- display repository reachability, latest full/differential backup, retention, WAL continuity, and lock state;
- identify scheduler ownership and prevent duplicate cluster-wide backup initiators;
- run manual full or differential backups with progress and final verification;
- evaluate backup freshness against configured policy;
- record restore-test evidence instead of treating repository status alone as proof of recoverability.

Restore and point-in-time recovery remain later-phase workflows because they require explicit target selection, isolation, and destructive-state controls.

### Operation center

Every operation must expose queued, running, succeeded, failed, or cancelled state; timestamps; actor; sanitized inputs; preflight results; affected nodes; automation output; and final verification. Logs must redact passwords, private keys, tokens, and backup credentials.

## Later Roadmap

### Phase 2: Cluster lifecycle

- add and remove nodes with membership and redundancy checks;
- manage supported PostgreSQL and Patroni configuration through `config_pgcluster`;
- run rolling package updates and PostgreSQL upgrades;
- provide emergency failover policy and guarded execution;
- restore to an isolated target and support controlled point-in-time recovery;
- verify DCS, routing, and backup state after lifecycle changes.

### Phase 3: Database administration

- create and manage databases, owners, users, roles, and grants;
- install and configure supported extensions;
- manage PgBouncer pools and connection limits;
- expose changes through the same preflight, confirmation, locking, logging, and audit model.

## V1 Non-Goals

- embedded SQL editor;
- billing, subscriptions, or support-plan integration;
- feature parity across every cloud provider;
- arbitrary shell or unrestricted Ansible execution;
- replacement of Autobase Automation, Patroni, pgBackRest, or the DCS;
- reproduction of proprietary Enterprise implementation or assets;
- database, role, extension, or PgBouncer administration before safe cluster operations are established.

## Success Criteria

V1 succeeds when:

- an existing cluster can be imported with zero managed-cluster mutation;
- observed state and intended change are visible before confirmation;
- only one mutating operation can run per cluster;
- every operation has a complete, secret-safe audit trail;
- planned switchover, reload, rolling restart, and replica reinitialization preserve required HA or stop before violating it;
- manual backups finish with verified pgBackRest results and scheduling has one cluster-aware owner;
- backup freshness is evaluated against policy and recoverability includes recorded restore evidence;
- routine supported operations no longer require operators to assemble direct SSH commands;
- failed operations leave the cluster recoverable and provide a clear safe next action.
