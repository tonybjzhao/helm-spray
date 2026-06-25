# Upstream issue triage (v5.0.0)

Status of the open issues on `ThalesGroup/helm-spray` against the 5.0.0
modernisation. Each entry is written so it can be posted directly as an issue
response. Categories: **Fixed**, **Behaviour change**, **By design**,
**Documented**, **Not reproducible**.

## Fixed

- **Command injection via chart reference** — chart fetching is now pure Go
  (`helm pull` invoked through `os/exec` with an argument vector, never a shell),
  so a chart name can no longer inject shell commands. It also no longer writes
  into the current directory.

- **Static binary / GLIBC crash on some hosts** — releases are now built with
  `CGO_ENABLED=0 -trimpath`, producing a static binary with no libc dependency,
  for darwin/linux (amd64+arm64) and windows/amd64.

- **Inconsistent readiness waiting between Helm v3 and v4** — readiness gating
  was rewritten to query each workload kind once as typed JSON and evaluate
  Deployments, StatefulSets, DaemonSets and Jobs from the decoded objects
  (observedGeneration, updated/ready replicas, revision convergence; Jobs honour
  `.spec.completions` and fail fast). This is independent of the Helm version.

- **StatefulSet with `updateStrategy: OnDelete` never finishes waiting** — the
  OnDelete strategy does not roll pods automatically, so the current/update
  revisions never converge after a spec change. Readiness for OnDelete
  StatefulSets is now based on ready replicas alone, so the wait completes.

- **Sub-chart consisting only of hooks** — a sub-chart whose only resources are
  Helm hooks renders an empty `.manifest` (Helm reports hooks separately). The
  readiness step now treats a tier with no workloads as immediately ready, so the
  release deploys successfully. Covered by a regression test.

## Behaviour change

- **Sub-charts gated by tags were not deployed by default** — helm-spray
  previously required a tag to be set *true* before a tagged sub-chart was
  sprayed, inverting Helm's own default (a tag is enabled unless set false). On
  an idiomatic umbrella whose dependencies all carry tags, a plain `helm spray`
  deployed nothing. As of 5.0.0, tag handling matches Helm: a tagged sub-chart is
  sprayed unless **every** one of its tags is explicitly `false`. This was found
  by planning a real multi-service umbrella and is the kind of divergence that
  made tag-based grouping surprising.

## By design (with rationale)

- **Restore the normal meaning of `enabled`** — helm-spray deploys one Helm
  release per sub-chart by toggling `<name>.enabled` per release; the `enabled`
  condition is the mechanism that makes per-release isolation work. Restoring its
  vanilla Helm meaning would require a different orchestration model (e.g. an
  explicit dependency graph). 5.0.0 makes the contract explicit: helm-spray warns
  when a dependency does not declare `condition: <name>.enabled`, so a
  misconfigured umbrella is diagnosable rather than silently mis-ordered.

- **Sub-charts of sub-charts (nested dependencies)** — helm-spray orchestrates
  the **top-level** dependencies of the umbrella chart, one release each. Nested
  sub-charts are deployed as part of their parent sub-chart's release (standard
  Helm), not as independently weighted releases. Weight-ordering nested charts
  would change the release model.

## Documented

- **`weight` rejected by a `values.schema.json`** — `weight` and `enabled` are
  helm-spray control values. If an umbrella ships a `values.schema.json`, it must
  permit them (e.g. allow additional properties on each sub-chart, or declare
  `weight`/`enabled`). helm-spray cannot relax a user-authored schema.

- **`requirements.yaml` vs `charts/`** — Helm itself unified `requirements.yaml`
  into the `dependencies:` block of `Chart.yaml` (apiVersion v2). helm-spray
  follows Helm and reads dependencies from `Chart.yaml`; vendored sub-charts live
  under `charts/`.

- **AppVersion shown for the umbrella, not the sub-chart** — every release
  helm-spray creates installs the umbrella chart (with a single sub-chart
  enabled), so `helm list` reports the umbrella's chart/appVersion for each
  release; that column is owned by Helm. helm-spray now surfaces each sub-chart's
  own `appVersion` in its plan output (`--output json`) and the web UI.

## Not reproducible

- **Keycloak/Kong integration test failure** — this is specific to a particular
  third-party chart set and environment rather than a defect in helm-spray's
  orchestration; it is not reproducible from the project itself. Happy to look at
  a minimal umbrella that reproduces an ordering or readiness problem.

## New in 5.0.0 that relates to long-standing requests

- **Uninstall / prune** — `helm spray uninstall [CHART]` removes a solution's
  releases in reverse weight order; `helm spray --prune` reconciles sub-charts
  removed from the umbrella by uninstalling their orphaned releases.
- **Rollback** — atomic rollback on partial failure is tracked as a follow-up
  capability rather than shipped in 5.0.0.
