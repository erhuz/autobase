# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Internal engineers and PostgreSQL operators managing existing HA clusters during routine operations and incident triage.

## Product Purpose

Extend Autobase Community Console with safe day-2 cluster management. Success means operators can quickly distinguish database availability from recoverability, find the affected node or subsystem, and reach the next safe action.

## Positioning

Unifies live Patroni/DCS topology, pgBackRest recovery evidence, and guarded operation audit in the existing Autobase Console workflow.

## Operating Context

Browser-based Console for existing PostgreSQL HA clusters using Patroni, a DCS, routing targets, pgBackRest, Automation playbooks, and durable operation logs.

## Capabilities and Constraints

Cluster health, query-performance observability, connection access, and guarded operations use existing Console APIs. Discovery remains passive; cluster mutations require preflight, confirmation, locking, audit, and verification. Browser-direct host access and arbitrary execution are excluded.

## Brand Commitments

Preserve Autobase Community Console identity, terminology, MUI component language, and restrained operational color semantics.

## Evidence on Hand

`SPEC.md`, `MANAGEMENT_VISION.md`, generated API clients, health/query UI tests, and management Playwright coverage are authoritative. Customer claims and synthetic operational evidence must not be invented.

## Product Principles

- Triage health before configuration.
- Label availability and recoverability separately.
- Show evidence and safe next actions, not optimistic summaries.
- Keep infrequent configuration behind clear route boundaries.
- Reuse established Console patterns before adding abstractions.

## Accessibility & Inclusion

Controls are keyboard-native and programmatically labelled; operational state never relies on color alone.
