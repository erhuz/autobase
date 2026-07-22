# SPEC

## §G

G1: add `MANAGEMENT_VISION.md` → purpose + phased roadmap for safe day-2 management in Autobase Community fork.

## §C

C1: docs-only; existing code, runtime, READMEs ⊥ change.
C2: audience = internal engineers + PostgreSQL operators.
C3: content generic; environment names, IPs, credentials, deployment-specific values ⊥.
C4: v1 focus = safe cluster operations; database administration deferred.
C5: reuse Console UI → API → DB/operation log → Automation → PostgreSQL/Patroni; new orchestration engine ⊥.
C6: browser-direct SSH ⊥; API-triggered automation only.
C7: Community MIT attribution preserved; Enterprise code replication ⊥.
C8: v1 SQL editor, billing, cloud-provider parity ⊥.

## §I

doc: `MANAGEMENT_VISION.md` → {purpose,current-gap,architecture,safety-principles,v1,roadmap,non-goals,success-criteria}
flow: `console/ui` → `console/service` → `console/db` + `automation` → managed PostgreSQL cluster
verify: `git diff --check` + local path/link checks + environment-data scan

## §V

V1: `MANAGEMENT_VISION.md` ∃ @ repo root; existing tracked files unchanged except `SPEC.md` task status.
V2: doc fully generic → environment names, IPs, credentials, secret values ⊥.
V3: passive import invariant → discovery/registration performs zero managed-cluster mutation.
V4: ∀ mutating operation → preflight + desired/current diff + explicit confirmation + cluster lock + durable audit.
V5: leader/replica guards precede switchover, restart, reinitialization, node removal, restore.
V6: v1 covers health/topology, DCS/routing/backup state, planned switchover, reload, rolling restart, guarded replica reinit, manual full/diff backup, single scheduler ownership, operation progress/log/failure visibility.
V7: later roadmap separates scaling, updates/upgrades, restore/PITR, database/role/extension/PgBouncer administration from v1.
V8: architecture reuses existing Console + Automation boundaries; browser-direct host access ⊥.
V9: non-goals explicitly include SQL editor, billing, cloud-provider parity, Enterprise-code replication.
V10: success criteria include zero-mutation import, serialized ops, complete audit, preserved HA, current + restore-proven backups.
V11: Markdown clean; referenced local paths ∃; no new build/test dependency.

## §T

id|status|task|cites
T1|x|add root management vision + roadmap; verify formatting, references, privacy|V1,V2,V3,V4,V5,V6,V7,V8,V9,V10,V11,I.doc,I.flow,I.verify

## §B

id|date|cause|fix
