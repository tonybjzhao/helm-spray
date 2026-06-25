# helm-spray — Capability Roadmap

This roadmap captures capabilities that would strengthen helm-spray as a multi-chart
deployment orchestrator, derived from an analysis of the broader ecosystem of
Kubernetes/Helm delivery tooling. It is organised by priority and mapped against the
current implementation.

> Status legend — **missing**: not implemented; **partial**: a limited form exists;
> **present**: already covered.

## Themes

1. **Ordering model** — evolve from flat integer-weight buckets to a true dependency
   graph, preserving weight semantics as a compatibility layer.
2. **Speed** — bounded concurrency for independent subcharts within an ordering tier.
3. **Safety before apply** — an aggregate, whole-solution plan/diff plus dependency
   validation so a coordinated change can be reviewed before any mutation.
4. **Failure handling** — atomic/rollback orchestration and resume-from-failure so a
   partial deploy never leaves the solution half-applied.
5. **Readiness fidelity** — broader, more accurate per-release health gating beyond
   three workload kinds.
6. **Lifecycle integrity** — prune releases for removed subcharts; orchestration-level
   hooks between tiers.
7. **Configuration scale** — secrets handling, multi-environment layering, programmable
   configuration.
8. **Robustness & observability** — reduce reliance on external CLI binaries, add
   machine-readable output, and visualize topology.

## Prioritised backlog

| # | Capability | Status | Priority | Impact | Effort |
|---|------------|--------|----------|--------|--------|
| 1 | Dependency-graph (DAG) ordering with explicit per-subchart dependencies (weights map onto graph levels for back-compat) | missing | P0 | high | high |
| 2 | Aggregate plan/diff preview of the whole solution before applying (with CI exit code) | missing | P0 | high | medium |
| 3 | Atomic/rollback orchestration on partial failure (per-release + group-level) | missing | P0 | high | high |
| 4 | Bounded intra-tier parallelism for independent subcharts | missing | P1 | high | medium |
| 5 | Higher-fidelity, per-release readiness/health gating (DaemonSets, PVCs, CRDs, conditions) | partial | P1 | high | high |
| 6 | Prune releases for subcharts removed from the umbrella (lifecycle reconciliation) | missing | P1 | medium | medium |
| 7 | Resume-from-failure / idempotent re-run | partial | P1 | medium | medium |
| 8 | Encrypted secrets handling for values (decrypt at deploy, mask in output) | missing | P1 | medium | medium |
| 9 | Orchestration-level lifecycle hooks between tiers | missing | P2 | medium | medium |
| 10 | Multi-environment value layering (base + per-env overrides, documented precedence) | partial | P2 | medium | medium |
| 11 | Embed deployment engine (Helm SDK + client-go) instead of shelling out to CLIs | missing | P2 | medium | high |
| 12 | Drift detection against live cluster (on-demand, reuses plan/diff) | missing | P2 | medium | medium |
| 13 | Programmable/templated orchestration configuration | missing | P3 | medium | high |
| 14 | Dependency/topology and ordering visualization | missing | P3 | low | low |
| 15 | Structured machine-readable run output (JSON: releases, order, edges, action, status) | missing | P3 | low | low |

## Sequencing rationale

- **#11 (embed the SDK)** is treated as a foundational enabler: it removes CLI
  version-coupling and brittle stdout parsing, unlocks accurate readiness (#5),
  structured output (#15), and the planned GUI (which needs an in-process API and the
  computed deployment graph #14).
- **#1 (DAG)** is the highest-value functional differentiator and the substrate that
  parallelism (#4) and partial-deploy correctness (#3, #7) build on.
- **#2 (plan/diff)** and **#15 (structured output)** are prerequisites for trustworthy
  automation and gate the rest.

See [00-baseline-before.md](../reports/00-baseline-before.md) for the measured starting
point.
