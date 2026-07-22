# SPEC

## §G

G1: extend Autobase Community Console → safe day-2 management for existing HA PostgreSQL clusters.
G2: v1 → guarded cluster operations; phase 2 → cluster lifecycle; phase 3 → database administration.
G3: ∀ release → direct tested upgrade from unmodified Community `2.9.0`.

## §C

C1: baseline = Autobase Community `2.9.0`; upstream attribution + MIT license ! preserved; Enterprise code/assets ⊥ copy.
C2: audience = internal engineers + PostgreSQL operators.
C3: reuse Console UI → API → DB/operation log → Automation → managed cluster; second orchestration engine ⊥.
C4: Patroni + DCS authoritative for topology/leadership; pgBackRest authoritative for backup repository state.
C5: browser-direct host access, SSH credentials, arbitrary shell, unrestricted Ansible ⊥.
C6: import/discovery read-only against managed cluster; writes limited to Console DB inventory.
C7: ∀ mutation → current + desired state, preflight, exact plan, affected nodes, proportional confirmation, cluster lock, durable audit, final verification.
C8: omitted input → no configuration change.
C9: secrets, passwords, private keys, tokens, backup credentials ⊥ operation params/logs/API responses.
C10: operation failure → cluster recoverable + current state + safe next action.
C11: v1 excludes forced/ambiguous failover, restore/PITR, node scaling, config management, updates/upgrades, database/role/extension/PgBouncer administration.
C12: v1 excludes SQL editor, billing/subscriptions/support plans, full cloud-provider parity.
C13: future release compatibility declared/tested as one `{ui,api,console_db,automation}` version set.
C14: migrations forward + ordered; Console persistent volume reset ⊥.
C15: implementation extends existing Swagger, Go service/storage/watchers, React routes, Goose migrations, Automation playbooks/roles.

## §I

doc: `MANAGEMENT_VISION.md` → product authority for scope, phases, safety, success.
ui.health: `/clusters/:clusterId/overview` → topology + DCS + routing + backup + operation summary + guarded action entry.
ui.ops: `/operations` + `/operations/:operationId/log` → queue/state/filter/detail/log/failure/verification.
api.health: `GET /clusters/{id}/health` → `{observed_at,topology,dcs,routing,backup,operation,recoverability}`.
api.preflight: `POST /clusters/{id}/preflights` `{type,target?,params?}` → `{id,observed,desired,checks,blockers,plan,affected_nodes,confirmation}`.
api.run: `POST /clusters/{id}/operations` `{preflight_id,confirmation}` → `{operation_id,status}`.
api.ops: `GET /operations` + `GET /operations/{id}` + `GET /operations/{id}/log` → durable operation/audit state.
op.v1: `type ∈ {switchover,reload,rolling_restart,replica_reinit,backup_full,backup_diff}`.
op.state: `queued → running → succeeded|failed|cancelled`.
db: `console/db/migrations` → preserve existing data; extend operation/preflight/audit/backup-evidence persistence.
automation: API runner → existing inventory + supported Autobase playbooks/roles + Patroni/pgBackRest operations.
authority: Patroni/DCS → live topology; routing target checks → traffic state; pgBackRest → backup/WAL/lock state.
release: stock `2.9.0` DB/config fixture → migrate → verify data + secrets metadata + zero managed-cluster mutation.
verify: Go unit/integration + UI unit/e2e + migration fixture + operation safety contract + `git diff --check`.

## §V

V1: `MANAGEMENT_VISION.md` ∃ @ repo root; T1 docs-only output preserved.
V2: doc fully generic → environment names, IPs, credentials, secret values ⊥.
V3: passive import → package/config/service/credential/PostgreSQL/Patroni/DCS/routing/backup/monitoring mutation ⊥.
V4: ∀ mutating operation → preflight + desired/current diff + explicit confirmation + cluster lock + durable audit.
V5: leader/replica roles refreshed immediately before switchover, restart, reinit, node removal, restore.
V6: v1 covers health/topology, DCS/routing/backup state, planned switchover, reload, rolling restart, guarded replica reinit, manual full/diff backup, single scheduler ownership, operation progress/log/failure visibility.
V7: phase 2 owns scaling, config, updates/upgrades, emergency failover, restore/PITR; phase 3 owns database/role/extension/PgBouncer administration.
V8: architecture reuses existing Console + Automation boundaries; browser-direct host access ⊥.
V9: non-goals explicitly include SQL editor, billing, cloud-provider parity, arbitrary execution, Enterprise-code replication.
V10: v1 acceptance → zero-mutation import + serialized ops + complete audit + preserved HA + current and restore-proven backups.
V11: Markdown clean; referenced local paths ∃; new build/test dependency ⊥ unless existing stack cannot cover requirement.
V12: ∀ release → stock `2.9.0` DB/config fixture upgrades directly; intermediate fork release ⊥.
V13: upgrade preserves/migrates clusters, servers, projects, environments, settings, operation history, encrypted secret records, inventory, config; external key preservation documented.
V14: Console control-plane upgrade → managed-cluster mutation ⊥; backup, upgrade, verification, rollback docs !.
V15: release metadata declares tested `{ui,api,console_db,automation}` set.
V16: health snapshot includes leader, replicas, state, timeline, lag, pending restart, Patroni/DCS reachability/membership, routing targets, pgBackRest repository/backup age/WAL/locks, active/latest/unresolved operation.
V17: database availability ≠ recoverability; stale backup, WAL gap, or missing restore evidence → recoverability degraded despite healthy Patroni.
V18: discovery drift reported before management enabled; unresolved safety-critical drift → mutation blocked.
V19: operation record includes state, timestamps, actor, sanitized inputs, preflight/checks, plan, affected nodes, automation output, outcome, final verification, safe next action.
V20: operation states ∈ `queued,running,succeeded,failed,cancelled`; terminal state immutable except appended audit correction.
V21: ∀ cluster ≤1 queued/running mutation; DB-enforced lock acquired before launch + released on terminal state/recovery.
V22: preflight bound to cluster + action + target + observed state; topology-sensitive execution rechecks guards after confirmation; stale/changed state → stop.
V23: planned switchover requires healthy leader + selected replica, candidate lag within policy, DCS reachable, no conflict; show routing impact; verify new leader + replicas + routing.
V24: reload ≠ restart; rolling restart handles one replica @ time, preserves healthy failover candidate, controlled switchover before leader restart, verifies topology/routing after each stage.
V25: replica reinit requires current replica + another healthy member + explicit local-data-loss confirmation + clone source/method; current leader target → reject; completion → streaming + lag verified.
V26: backup view reports repository reachability, latest full/diff, retention, WAL continuity, locks, scheduler owner, freshness policy, restore-test evidence; repository `ok` alone ≠ recoverable.
V27: manual full/diff backup requires one cluster-aware scheduler/initiator, progress + final pgBackRest verification; duplicate cluster-wide initiator → reject.
V28: operations idempotent where Automation permits; completed progress preserved; failed guard stops next stage; arbitrary command input ⊥.
V29: phase 2 node removal/scaling/update/upgrade/emergency failover preserves required membership + failover capacity and verifies DCS/routing/backup after change.
V30: restore/PITR targets isolated recovery workflow; running source-cluster overwrite ⊥.
V31: phase 3 database/owner/user/role/grant/extension/PgBouncer changes reuse authorization, preflight, confirmation, locking, audit, verification contract.
V32: automated tests cover operation lock race, stale preflight, role change, secret redaction, failure transition, HA guard, backup verification, stock `2.9.0` migration.

## §T

id|status|task|cites
T1|x|add root management vision + migration baseline; verify formatting, references, privacy|V1,V2,V11,I.doc
T2|.|add stock `2.9.0` DB/config fixture + direct migration preservation test|V12,V13,V14,V15,V32,I.release,I.verify
T3|.|extend Console DB operation/preflight/audit model + DB-enforced per-cluster lock|V19,V20,V21,V32,I.db,I.op.state
T4|.|add unified health collector + `GET /clusters/{id}/health`|V16,V17,V26,I.api.health,I.authority
T5|.|make import passive; report drift + gate management|V3,V18,V32,I.api.health,I.authority
T6|.|add shared preflight/confirm/launch service + operation detail API + redaction|V4,V8,V19,V20,V21,V22,V28,V32,I.api.preflight,I.api.run,I.api.ops,I.automation
T7|.|build cluster health + operation-center UI on existing routes|V16,V17,V19,V20,V26,I.ui.health,I.ui.ops,I.api.health,I.api.ops
T8|.|add guarded planned switchover vertical slice|V5,V21,V22,V23,V32,I.op.v1,I.automation
T9|.|add reload + guarded rolling-restart vertical slices|V5,V21,V22,V24,V32,I.op.v1,I.automation
T10|.|add guarded replica-reinit vertical slice|V5,V21,V22,V25,V32,I.op.v1,I.automation
T11|.|add pgBackRest health, scheduler ownership, manual full/diff backup, restore evidence|V17,V21,V22,V26,V27,V32,I.op.v1,I.authority,I.automation
T12|.|run v1 safety/e2e + stock-upgrade gates; publish backup/upgrade/verify/rollback docs + version set|V10,V12,V13,V14,V15,V32,I.release,I.verify
T13|.|phase 2 add/remove nodes + supported `config_pgcluster` management|V5,V7,V29,I.automation
T14|.|phase 2 rolling updates/upgrades + emergency-failover policy|V5,V7,V29,I.automation
T15|.|phase 2 isolated restore + PITR workflow|V5,V7,V30,I.automation
T16|.|phase 3 database, owner, user, role, grant management|V7,V31,I.api.preflight,I.api.run,I.automation
T17|.|phase 3 supported extension + PgBouncer pool/limit management|V7,V31,I.api.preflight,I.api.run,I.automation

## §B

id|date|cause|fix
