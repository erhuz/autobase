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
C16: automation service credentials = separate-purpose + cluster-bound + attach-only; per-operation override + live PostgreSQL/Patroni password rotation ⊥.
C17: DCS health reuses authenticated Patroni observation; direct etcd client + TLS-key distribution ⊥.

## §I

doc: `MANAGEMENT_VISION.md` → product authority for scope, phases, safety, success.
ui.health: `/clusters/:clusterId/overview` → triage-first availability/recoverability + topology + DCS + routing + backup + operation summary + guarded action entry.
ui.ops: `/operations` + `/operations/:operationId/log` → queue/state/filter/detail/log/failure/verification.
ui.query: `/clusters/:clusterId/query-performance` → state/coverage/filter/KPI/trend/top-query/detail/enable/disable.
ui.access: `/clusters/:clusterId/access` → connection values + management credential + 3 automation credential bindings.
ui.secrets: Settings → encrypted password secret create; password input masked; values ⊥ read.
api.health: `GET /clusters/{id}/health` → `{observed_at,topology,dcs,routing,backup,operation,recoverability}`.
api.query: `GET /clusters/{id}/query-performance` + `GET /clusters/{id}/query-performance/{fingerprintId}` → `{status,coverage,summary,series,queries|fingerprint,filters?,histogram?}`.
api.preflight: `POST /clusters/{id}/preflights` `{type,target?,params?}` → `{id,observed,desired,checks,blockers,plan,affected_nodes,confirmation}`.
api.run: `POST /clusters/{id}/operations` `{preflight_id,confirmation}` → `{operation_id,status}`.
api.credential: `PUT /clusters/{id}/credential` `{secret_id}` → `ClusterInfo`; secret value ⊥ response.
api.automation_credentials: `PUT /clusters/{id}/automation-credentials` `{postgres_superuser_secret_id:int|null,postgres_replication_secret_id:int|null,patroni_restapi_secret_id:int|null}` → `ClusterInfo`; full atomic replace; values ⊥ response.
api.ops: `GET /operations` + `GET /operations/{id}` + `GET /operations/{id}/log` → durable operation/audit state.
op.v1: `type ∈ {switchover,reload,rolling_restart,replica_reinit,backup_full,backup_diff,backup_scheduler_reconcile,query_analytics_enable,query_analytics_disable}`.
op.credentials: `query_analytics_enable|query_analytics_disable` → superuser+REST; `node_add|config_update|postgresql_upgrade|restore|pitr` → superuser+replication+REST; `backup_full|backup_diff|backup_scheduler_reconcile` → ∅; remaining guarded ops → superuser; undeclared op → block.
op.state: `queued → running → succeeded|failed|cancelled`.
db: `console/db/migrations` → preserve existing data; extend operation/preflight/audit/backup-evidence persistence.
db.query: Console DB → analytics source + fingerprint + complete bucket + retained sample + 7d retention.
automation: API runner → existing inventory + supported Autobase playbooks/roles + Patroni/pgBackRest operations.
automation.credentials: cluster bindings → launch-only `patroni_superuser_*|patroni_replication_*|patroni_restapi_*`; persisted operation/preflight/audit/log values ⊥.
automation.query: signed package + secure PGSM config + scoped read-only role + serial HA rollout.
authority: Patroni/DCS → live topology; routing target checks → traffic state; pgBackRest → backup/WAL/lock state.
release: stock `2.9.0` DB/config fixture → migrate → verify data + secrets metadata + zero managed-cluster mutation.
image: main/tag → public `ghcr.io/erhuz/{automation,console_ui,console_api}`; Console DB = official Autobase `2.9.0` digest.
release.manifest: tag artifact → `{release,base,source_commit,migration_head,platform,ui,api,console_db,automation}` refs + digests.
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
V51: ∀ supported operation type → preflight persistence + schema constraints accept it; integration test exercises ≥1 non-analytics type against real schema.
V52: I.image publish auth = GitHub Actions `GITHUB_TOKEN` + `packages:write`; anonymous pull succeeds; Docker Hub publish ⊥; current refs use I.image; stock `2.9.0` fixture unchanged.
V53: stock `2.9.0` upgrade gate = production Console DB PostgreSQL 16 + TimescaleDB columnstore hypertable + compressed legacy operations; raw history/timestamps preserved; external status canonical; migration succeeds.
V54: ∀ Automation-backed mutation → attached same-project `ssh_key|password` secret !; absent/invalid credential blocks preflight + execution.
V55: release built from tagged source commit; workflow source edits/commits ⊥; §I.release.manifest pins base, head, platform, official DB + published UI/API/Automation digests.
V56: cluster detail → route-backed `{overview,query-performance,access}`; overview keeps availability ≠ recoverability + health/node triage; tab-only data loads active route; controls keyboard-native.
V57: viewport ≤600px → sidebar width = 60px + cluster tabs/content reachable; horizontal content clipping ⊥.
V58: query analytics enable|disable preserves effective preload + HBA when Patroni DCS omits either; collector rules scoped + reversible.
V59: ∀ guarded op → credential-purpose set declared; required same-project password secrets attached + decryptable @ preflight, IDs+`updated_at` bound, values launch-only; running target auth probed before first mutation; isolated bootstrap validates before mutation + probes every target after service start; failure → next mutation/verification ⊥.
V60: DCS `reachable=true` iff every current healthy member has fresh Patroni-observed `dcs_last_seen`; configured type/members alone → `not_observed`; stale/missing/member disagreement → degraded; direct etcd client/TLS-key distribution ⊥.
V61: Automation launch name empty → Docker assigns uniqueness; lifecycle uses container ID; terminal observer cleanup reaches Docker after success|error with CID present|absent; concurrent launch + cleanup tested; app name generator/retry registry ⊥.
V62: backup observer read-only; duplicate scheduler owners visible + backup mutation blocked; guarded `backup_scheduler_reconcile` reuses pgBackRest role → cron only on `pgbackrest_scheduler_host`; arbitrary cron/playbook vars ⊥.
V63: `restore_tested_at` updated only after successful isolated restore/PITR final verification; configured/operator timestamp alone ⊥; absent evidence → recoverability degraded.
V64: DB/Docker diagnostic logging accepts typed-nil args + absent `CtxCidKey`; panic ⊥; operation flow unchanged; Docker bodies/secrets ⊥ logs.
V65: query analytics enable|disable → preflight-bound primary routes ! nonempty; launch passes `operation_primary_routing_targets`; Automation validates before config/service mutation; each serial restart stage verifies ∀ route writable; absent|changed input → stop pre-mutation.

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
T11|x|add pgBackRest health, scheduler ownership, manual full/diff backup, restore evidence|V17,V21,V22,V26,V27,V32,I.op.v1,I.authority,I.automation
T12|x|run v1 safety/query/e2e + stock-upgrade gates; publish backup/upgrade/verify/rollback docs + version set|V10,V12,V13,V14,V15,V32,V35,V37,V39,V40,V41,V42,V43,V44,V45,I.release,I.verify
T13|x|phase 2 add/remove nodes + supported `config_pgcluster` management|V5,V7,V29,I.automation
T14|x|phase 2 rolling updates/upgrades + emergency-failover policy|V5,V7,V29,I.automation
T15|x|phase 2 isolated restore + PITR workflow|V5,V7,V30,I.automation
T16|x|phase 3 database, owner, user, role, grant management|V7,V31,I.api.preflight,I.api.run,I.automation
T17|x|phase 3 supported extension + PgBouncer pool/limit management|V7,V31,I.api.preflight,I.api.run,I.automation
T18|x|add PGSM package/config/default-on bootstrap + contract tests|V33,V34,V35,V38,V39,V40,V44,V45,I.automation.query,I.verify
T19|x|add analytics Console DB schema + migration/storage tests|V12,V13,V14,V40,V41,V42,V44,V45,V46,V47,I.db.query,I.release,I.verify
T20|x|add all-node PGSM collector + query-performance APIs|V34,V39,V40,V41,V42,V43,V44,V45,I.api.query,I.db.query,I.verify
T21|x|add query-performance UI + status/filter/trend/detail tests|V34,V42,V43,V44,V45,V48,V49,I.ui.query,I.api.query,I.verify
T22|x|add guarded PGSM enable/disable preflight + serial HA operation|V4,V5,V19,V20,V21,V22,V24,V34,V36,V37,V38,V39,V40,V44,V45,V50,I.api.preflight,I.api.run,I.api.ops,I.op.v1,I.automation.query,I.verify
T23|x|widen `operation_preflights.operation_type` constraint to all supported types + non-analytics preflight integration test|V4,V32,V51,I.db,I.api.preflight
T24|x|guard operations-list `finished` to terminal states|V50,I.api.ops
T25|x|cut image publishing + current pulls to public GHCR|V11,V15,V52,I.image,I.release,I.verify
T26|x|repair stock `2.9.0` TimescaleDB migration + legacy operation compatibility|V12,V13,V14,V20,V21,V32,V45,V50,V53,I.db,I.release,I.verify
T27|x|add imported-cluster credential attach + shared management blocker|V3,V4,V9,V18,V22,V32,V44,V54,I.api.credential,I.api.preflight,I.api.run
T28|x|cut reproducible `2.9.0-management.1` release manifest + official DB retention|V11,V12,V14,V15,V46,V47,V52,V55,I.image,I.release,I.release.manifest,I.verify
T29|x|redesign cluster detail into triage-first tabs + route-scoped data|V11,V16,V17,V26,V43,V48,V56,V57,I.ui.health,I.ui.query,I.ui.access,I.api.health,I.api.query,I.api.credential,I.verify
T30|x|cut reproducible `2.9.0-management.2` release + direct `2.9.0` upgrade gate|V11,V12,V14,V15,V46,V47,V52,V55,I.image,I.release,I.release.manifest,I.verify
T31|x|cut `2.9.0-management.3` PGSM imported-cluster hotfix release|V11,V12,V14,V15,V46,V47,V52,V55,V58,I.image,I.release,I.release.manifest,I.verify
T32|x|bind 3 cluster automation password secrets + Access UI + purpose map + pre-mutation auth probes + half-applied-op recovery runbook|V4,V12,V13,V19,V22,V32,V44,V54,V59,I.ui.access,I.ui.secrets,I.api.automation_credentials,I.op.credentials,I.automation.credentials,I.verify
T33|x|cut reproducible `2.9.0-management.4` credential-safety release|V11,V12,V14,V15,V46,V47,V52,V55,V59,I.image,I.release,I.release.manifest,I.verify
T34|x|use Docker-assigned Automation names; make DB/Docker logging panic-safe; verify concurrent launch + terminal observer cleanup|V11,V44,V47,V61,V64,I.automation,I.verify
T35|x|derive DCS reachability from existing Patroni watcher evidence + health/UI contract|V16,V23,V32,V60,I.api.health,I.authority,I.verify
T36|x|add guarded pgBackRest scheduler reconcile via existing role + duplicate-owner contract|V4,V17,V21,V22,V26,V27,V32,V54,V62,I.api.preflight,I.api.run,I.op.v1,I.authority,I.automation,I.verify
T37|x|bind restore evidence to verified isolated restore/PITR completion|V17,V26,V30,V32,V63,I.api.health,I.authority,I.automation,I.verify
T38|x|bind query-analytics primary routes → desired/Automation; add pre-mutation guards + regressions|V22,V24,V32,V36,V37,V65,I.automation.query,I.authority,I.verify

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
B50|2026-07-23|UI API generator targeted absent obsolete directory + returned success without output|V47
B51|2026-07-23|backup observer startup omitted controller import; full generated-source service gate failed|V47
B52|2026-07-23|ansible-lint 26.4 made intentional play-level become + linear run-once patterns fatal; CI failed|V45
B53|2026-07-23|CI installed Python 3.14 but bootstrap silently selected stale 3.12 pin|V45
B54|2026-07-24|focused lifecycle Go gate ran before Swagger generation|V47
B55|2026-07-24|Ansible lint gate ran before required `.venv` bootstrap|V45
B56|2026-07-24|local Playwright web server invoked unavailable `yarn` shim|V45
B57|2026-07-24|local Playwright gate ran before Chromium bundle install|V45
B58|2026-07-24|restore syntax gate ran before declared Automation collections installed|V45
B59|2026-07-24|new lifecycle Go source missed canonical formatting; service gate stopped before tests|V47
B60|2026-07-24|`operation_preflights.operation_type` check constraint fixed at query-analytics types; ∀ other guarded preflight insert fails|V4,V51
B61|2026-07-24|`v_operations` mapped `updated_at` to `finished` unguarded; running rows reported terminal timestamp in list API|V50
B62|2026-07-24|plain PostgreSQL 17 migration gate missed TimescaleDB columnstore; guarded migration disabled hypertable trigger + rewrote compressed history|V53
B63|2026-07-24|TimescaleDB health SQL lost nested shell quotes; production migration job stayed unhealthy|V53
B64|2026-07-27|health test required standalone leader text after detail compaction|V16
B65|2026-07-27|restricted sandbox blocked Chromium sandbox host before E2E launch|V45
B66|2026-07-27|permanent 220px sidebar clipped cluster content @ phone viewport|V57
B67|2026-07-27|health timestamp interpolation escaped locale date slashes into visible entities|V16
B68|2026-07-27|copy control kept unused hook-state destructure; lint failed|V56
B69|2026-07-27|full UI lint blocked by 42 pre-existing out-of-scope errors|V45
B70|2026-07-27|parallel UI gates killed `tsc --noEmit` with exit 143 before diagnostics|V45
B71|2026-07-27|serial whole-tree `tsc --noEmit` hung >4m without diagnostics|V45
B72|2026-07-27|bounded T29 typecheck timed out under default + 8GiB heaps without diagnostics|V45
B73|2026-07-27|cluster shell used ES2021 `replaceAll` + numeric router params under ES2020/string route contract|V56
B74|2026-07-27|final UI gate guessed absent `npm test` script + stale router paths instead of live package/status|V45
B75|2026-07-27|focused E2E server used default auth token while fixture injected management token; app correctly rejected fixture|V45
B76|2026-07-27|restricted sandbox denied corrected Vite localhost bind until elevated|V45
B77|2026-07-27|new node-state chip repeated ES2021 `replaceAll` under ES2020 compiler target|V56
B78|2026-07-27|copy button disabled empty values visually but passed optional value to clipboard hook|V56
B79|2026-07-27|query analytics required DCS `pg_hba`; Autobase/imported clusters keep effective rules in local `pg_hba.conf`|V58
B80|2026-07-28|maintenance trusted stale Patroni superuser config; Query Analytics changed cluster before TCP auth probe|V59
B81|2026-07-28|T32 focused gates retained pre-credential payload + implicit UI/YAML runner assumptions|V47,V59
B82|2026-07-28|isolated restore target may have no service to authenticate before destructive bootstrap|V59
B83|2026-07-28|DCS card used configured inventory only; healthy live etcd remained `configured_not_observed`|V60
B84|2026-07-28|finite app-generated Docker names collided; backup observer stopped before evidence upsert|V61
B85|2026-07-28|pgBackRest scheduler cron remained on every member; concurrent jobs raced repository locks|V62
B86|2026-07-28|typed-nil backup timestamps panicked SQL trace; CID-less deferred Docker cleanup panicked before DELETE → exited containers accumulated|V61,V64
B87|2026-07-28|Automation gate used forbidden destructive temp cleanup; syntax checks never started|V45
B88|2026-07-29|query-analytics desired/launch omitted `operation_primary_routing_targets` required by shared restart verifier; operation failed after first replica restart|V65
