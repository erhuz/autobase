# SPEC

## §G

G1: extend Autobase Community Console → safe day-2 management for existing HA PostgreSQL clusters.
G2: v1 → guarded cluster operations + query-performance observability; phase 2 → cluster lifecycle; phase 3 → database administration.
G3: ∀ release → direct tested upgrade from unmodified Community `2.9.0`.

## §C

C1: baseline = Autobase Community `2.9.0`; upstream attribution + MIT license ! preserved; Enterprise code/assets ⊥ copy.
C2: audience = internal engineers + PostgreSQL operators.
C3: reuse Console UI → API → DB/operation log → Automation → managed cluster; API → PostgreSQL read-only telemetry ?; second orchestration engine ⊥.
C4: Patroni + DCS authoritative for topology/leadership; pgBackRest authoritative for backup repository state.
C5: browser-direct host access, SSH credentials, arbitrary shell, unrestricted Ansible ⊥.
C6: import/discovery read-only against managed cluster; writes limited to Console DB inventory.
C7: ∀ mutation → current + desired state, preflight, exact plan, affected nodes, proportional confirmation, cluster lock, durable audit, final verification.
C8: omitted input → no configuration change.
C9: secrets, passwords, private keys, tokens, backup credentials ⊥ operation params/logs/API responses.
C10: operation failure → cluster recoverable + current state + safe next action.
C11: v1 excludes forced/ambiguous failover, restore/PITR, node scaling, generic config management, updates/upgrades, database/role/extension/PgBouncer administration; platform-owned `pg_stat_monitor` lifecycle = sole telemetry exception.
C12: v1 excludes SQL editor, billing/subscriptions/support plans, full cloud-provider parity.
C13: future release compatibility declared/tested as one `{ui,api,console_db,automation}` version set.
C14: migrations forward + ordered; Console persistent volume reset ⊥.
C15: implementation extends existing Swagger, Go service/storage/watchers, React routes, Goose migrations, Automation playbooks/roles.

## §I

doc: `MANAGEMENT_VISION.md` → product authority for scope, phases, safety, success.
ui.health: `/clusters/:clusterId/overview` → topology + DCS + routing + backup + operation summary + guarded action entry.
ui.ops: `/operations` + `/operations/:operationId/log` → queue/state/filter/detail/log/failure/verification.
ui.query: `/clusters/:clusterId/query-performance` → state/coverage/filter/KPI/trend/top-query/detail/enable/disable.
api.health: `GET /clusters/{id}/health` → `{observed_at,topology,dcs,routing,backup,operation,recoverability}`.
api.query: `GET /clusters/{id}/query-performance` + `GET /clusters/{id}/query-performance/{fingerprintId}` → `{status,coverage,summary,series,queries|fingerprint,filters?,histogram?}`.
api.preflight: `POST /clusters/{id}/preflights` `{type,target?,params?}` → `{id,observed,desired,checks,blockers,plan,affected_nodes,confirmation}`.
api.run: `POST /clusters/{id}/operations` `{preflight_id,confirmation}` → `{operation_id,status}`.
api.ops: `GET /operations` + `GET /operations/{id}` + `GET /operations/{id}/log` → durable operation/audit state.
op.v1: `type ∈ {switchover,reload,rolling_restart,replica_reinit,backup_full,backup_diff,query_analytics_enable,query_analytics_disable}`.
op.state: `queued → running → succeeded|failed|cancelled`.
db: `console/db/migrations` → preserve existing data; extend operation/preflight/audit/backup-evidence persistence.
db.query: Console DB → analytics source + fingerprint + complete bucket + retained sample + 7d retention.
automation: API runner → existing inventory + supported Autobase playbooks/roles + Patroni/pgBackRest operations.
automation.query: signed package + secure PGSM config + scoped read-only role + serial HA rollout.
authority: Patroni/DCS → live topology; routing target checks → traffic state; pgBackRest → backup/WAL/lock state.
release: stock `2.9.0` DB/config fixture → migrate → verify data + secrets metadata + zero managed-cluster mutation.
verify: Go unit/integration + UI unit/e2e + migration fixture + operation safety contract + `git diff --check`.

## §V

V1: `MANAGEMENT_VISION.md` ∃ @ repo root; T1 docs-only output preserved.
V2: doc fully generic → environment names, IPs, credentials, secret values ⊥.
V3: passive import → package/config/service/credential/PostgreSQL/Patroni/DCS/routing/backup/monitoring mutation ⊥.
V4: ∀ mutating operation → preflight + desired/current diff + explicit confirmation + cluster lock + durable audit.
V5: leader/replica roles refreshed immediately before switchover, restart, reinit, node removal, restore.
V6: v1 covers health/topology, DCS/routing/backup state, query-performance analytics, planned switchover, reload, rolling restart, guarded replica reinit, manual full/diff backup, single scheduler ownership, operation progress/log/failure visibility.
V7: phase 2 owns scaling, generic config, updates/upgrades, emergency failover, restore/PITR; phase 3 owns database/role/generic extension/PgBouncer administration; platform-owned PGSM lifecycle remains v1.
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
V33: v1 query analytics = platform-owned `pg_stat_monitor`; SQL editor + generic extension/config administration remain excluded.
V34: supported new Console cluster → analytics default on; import/Console upgrade → read-only detect + `rollout_required`; managed-cluster mutation ⊥ until confirmed operation.
V35: compatibility ∈ release-tested package matrix; initial = PostgreSQL `14..18` + `pg_stat_monitor` `2.3.2`; unsigned/source build ⊥.
V36: existing enable|disable → standard preflight/confirm/lock/audit + ≥3 healthy members.
V37: rollout `serial=1`; replicas first + verify each → controlled switchover → former leader last; failed gate → stop.
V38: preload merge preserves other libraries; existing `pg_stat_statements` precedes `pg_stat_monitor`.
V39: normalized SQL + PGSM query id + application tracking on; utility/planning/comment/plan capture off; `track_io_timing` unchanged.
V40: collector role = login + `pg_read_all_stats` + read-only + timeout + scoped HBA; superuser ⊥; credential encrypted + redacted.
V41: collector reads every healthy PostgreSQL node + `bucket_done=true` only; self-query excluded; ingest idempotent by node boot/bucket/fingerprint.
V42: Console DB retention = 7d; exact bucket totals + top100 total-time ∪ top100 max-latency samples; hourly indexed cleanup.
V43: API/UI expose state, coverage/gaps, node/database/role/application filters, totals, trends, top queries, detail histogram.
V44: literals, client IP, comments, plans, error text, credentials, raw collector SQL ⊥ persistence/log/API.
V45: tests cover package matrix, privacy drift, serial rollout failure, switchover, duplicate ingest, retention, coverage gaps, stock `2.9.0` migration.
V46: Console Go verification runs `-mod=readonly`; committed `go.mod` + `go.sum` resolve together.
V47: checkout-safe Go gate targets committed packages; full service gate runs after Swagger generation.
V48: query-performance filters have programmatic labels + keyboard-operable native controls.
V49: query-performance refresh/filter change advances window end; stale window refetch ⊥.
V50: operation `finished` timestamp ∃ iff status ∈ `succeeded,failed,cancelled`.

## §T

id|status|task|cites
T1|x|add root management vision + migration baseline; verify formatting, references, privacy|V1,V2,V11,I.doc
T2|x|add stock `2.9.0` DB/config fixture + direct migration preservation test incl analytics schema|V12,V13,V14,V15,V32,V45,I.release,I.verify
T3|x|extend Console DB operation/preflight/audit + query source/fingerprint/bucket/sample model + DB locks/retention|V19,V20,V21,V32,V40,V41,V42,V45,I.db,I.db.query,I.op.state
T4|x|add unified health + all-node complete-bucket collectors + health/query GET APIs|V16,V17,V26,V41,V42,V43,V44,V45,I.api.health,I.api.query,I.authority,I.db.query
T5|x|make import passive; report health/query capability drift + gate management|V3,V18,V34,V35,V39,V40,V44,V45,I.api.health,I.api.query,I.authority
T6|x|add shared preflight/confirm/launch + operation detail + query enable/disable + redaction|V4,V8,V19,V20,V21,V22,V28,V32,V34,V36,V40,V44,V45,I.api.preflight,I.api.run,I.api.ops,I.op.v1,I.automation.query
T7|x|build cluster health + operation-center + query-performance UI on existing routes|V16,V17,V19,V20,V26,V34,V42,V43,V44,V45,I.ui.health,I.ui.ops,I.ui.query,I.api.health,I.api.ops,I.api.query
T8|x|add guarded planned switchover vertical slice|V5,V21,V22,V23,V32,I.op.v1,I.automation
T9|x|add reload + guarded rolling-restart + PGSM package/config/bootstrap/enable/disable vertical slices|V5,V21,V22,V24,V32,V35,V36,V37,V38,V39,V40,V44,V45,I.op.v1,I.automation,I.automation.query
T10|x|add guarded replica-reinit vertical slice|V5,V21,V22,V25,V32,I.op.v1,I.automation
T11|.|add pgBackRest health, scheduler ownership, manual full/diff backup, restore evidence|V17,V21,V22,V26,V27,V32,I.op.v1,I.authority,I.automation
T12|.|run v1 safety/query/e2e + stock-upgrade gates; publish backup/upgrade/verify/rollback docs + version set|V10,V12,V13,V14,V15,V32,V35,V37,V39,V40,V41,V42,V43,V44,V45,I.release,I.verify
T13|.|phase 2 add/remove nodes + supported `config_pgcluster` management|V5,V7,V29,I.automation
T14|.|phase 2 rolling updates/upgrades + emergency-failover policy|V5,V7,V29,I.automation
T15|.|phase 2 isolated restore + PITR workflow|V5,V7,V30,I.automation
T16|.|phase 3 database, owner, user, role, grant management|V7,V31,I.api.preflight,I.api.run,I.automation
T17|.|phase 3 supported extension + PgBouncer pool/limit management|V7,V31,I.api.preflight,I.api.run,I.automation
T18|x|add PGSM package/config/default-on bootstrap + contract tests|V33,V34,V35,V38,V39,V40,V44,V45,I.automation.query,I.verify
T19|x|add analytics Console DB schema + migration/storage tests|V12,V13,V14,V40,V41,V42,V44,V45,V46,V47,I.db.query,I.release,I.verify
T20|x|add all-node PGSM collector + query-performance APIs|V34,V39,V40,V41,V42,V43,V44,V45,I.api.query,I.db.query,I.verify
T21|x|add query-performance UI + status/filter/trend/detail tests|V34,V42,V43,V44,V45,V48,V49,I.ui.query,I.api.query,I.verify
T22|x|add guarded PGSM enable/disable preflight + serial HA operation|V4,V5,V19,V20,V21,V22,V24,V34,V36,V37,V38,V39,V40,V44,V45,V50,I.api.preflight,I.api.run,I.api.ops,I.op.v1,I.automation.query,I.verify

## §B

id|date|cause|fix
B1|2026-07-22|Console `go.mod` pgx version absent from `go.sum`; clean verification failed|V46
B2|2026-07-22|checkout-safe test targeted absent generated Swagger packages|V47
B3|2026-07-22|nil latency histogram encoded SQL `NULL` against non-null sample schema|V45
B4|2026-07-22|histogram normalization scoped to fingerprint loop; storage build failed|V45
B5|2026-07-22|query analytics read model assigned pointer results to value fields; storage build failed|V47
B6|2026-07-22|pinned go-swagger v0.32.3 dependency failed under Go 1.26 token internals|V47
B7|2026-07-22|query-performance path insertion captured cluster delete operation; Swagger path validation failed|V47
B8|2026-07-22|current generated Swagger source imported split swag helpers absent from committed module lock|V46,V47
B9|2026-07-22|unquoted PostgreSQL test DSN triggered zsh glob expansion before integration test|V47
B10|2026-07-22|sandbox denied localhost socket for disposable PostgreSQL integration test|V47
B11|2026-07-22|Docker TRACE logger emitted request/response bodies containing deployment credentials|V44
B12|2026-07-22|UI verification could not start because checkout dependencies were absent|V45
B13|2026-07-22|npm fallback rejected existing ESLint peer mismatch before UI dependency install|V45
B14|2026-07-22|whole-tree TypeScript check exceeded 120s under unsupported local Node 25 toolchain|V45
B15|2026-07-22|narrow UI typecheck reached pre-existing store reducer and overview prop errors outside query analytics|V45
B16|2026-07-22|rendered query-performance test used `.ts` extension despite JSX; transform failed|V45
B17|2026-07-22|query-performance component test assumed jest-dom matchers absent from Vitest setup|V45
B18|2026-07-22|query-performance filter labels were not programmatically associated with select controls|V48
B19|2026-07-22|full UI suite hit pre-existing jsdom `localStorage` failures in cluster transform tests|V45
B20|2026-07-22|cluster overview invoked RTK query hook through callback; changed-surface lint failed|V45
B21|2026-07-22|query-performance refresh reused initial time-window end and could return stale data|V49
B22|2026-07-23|sandbox blocked writes to default Go module cache before guarded-operation tests ran|V47
B23|2026-07-23|isolated Go cache retry required dependency downloads but sandbox denied DNS/network|V47
B24|2026-07-23|Ansible gate used read-only default local temp path before playbook parsing|V45
B25|2026-07-23|sandbox prevented Ansible local RPC worker startup before contract execution|V45
B26|2026-07-23|guarded-rollout contract embedded backslash-escaped quotes invalid in YAML scalar|V45
B27|2026-07-23|guarded-rollout contract expected a literal version line while playbook pins via default expression|V45
B28|2026-07-23|standalone Ansible syntax gate could not resolve repo-local FQCN collection layout|V45
B29|2026-07-23|guarded-operation UI test used global queries while prior renders persist in project Vitest setup|V45
B30|2026-07-23|Testing Library render queries remained bound to document body, so prior test DOM still duplicated controls|V45
B31|2026-07-23|shared switchover retry block ignored a second failure and could continue to leader restart|V24,V37
B32|2026-07-23|operations list filtered by finished time, hiding queued/running rows with no final completion|V19,V20
B33|2026-07-23|sandbox denied Docker socket access during disposable migration-container cleanup|V45
B34|2026-07-23|Ansible 2.20 required regex assertion condition to remain an explicit string scalar|V45
B35|2026-07-23|switchover role recheck status list was indented outside the URI module mapping|V45
B36|2026-07-23|operation detail mapped running `updated_at` to terminal `finished`|V50
B37|2026-07-23|verification used repo-root file paths from nested workdir; formatter never ran|V47
B38|2026-07-23|isolated full-service gate assumed absent `/home/erhuz/go/bin/swagger`; generation never ran|V47
B39|2026-07-23|isolated service copy omitted sibling `console/db`; migration contract fixture unresolved|V47
B40|2026-07-23|deployment password secret merged into persisted cluster extra vars|V44
B41|2026-07-23|focused Go gate launched from repo root outside service module|V47
B42|2026-07-23|sandbox blocked pinned Swagger command metadata lookup during validation|V47
B43|2026-07-23|cluster-health test used exact DCS value match inside labeled text|V45
B44|2026-07-23|operation-detail test mock omitted refresh control dispatch hook|V45
B45|2026-07-23|new operation views wrapped RTK hooks in callbacks; changed-surface lint failed|V45
B46|2026-07-23|backup health card omitted retention + freshness policy evidence|V26
B47|2026-07-23|migration + storage integration packages ran concurrently; storage queried before schema creation|V47
B48|2026-07-23|standalone replica-reinit syntax gate lacked repo collection layout; FQCN role resolution failed|V47
B49|2026-07-23|Ansible Galaxy install used read-only default cache despite temp dirs|V45
