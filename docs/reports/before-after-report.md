# helm-spray Modernisation — Before / After Report

This report summarises the modernisation of helm-spray from a stale Helm v3
plugin into a first-class, Helm v4 open-source package (version **5.0.0**). It
compares the state at the branch point (`d98d7a5`) with the modernised branch:
**49 commits**, **+5.4k / −1.2k** lines, Go source **1,356 → 4,234 lines**.

## Executive summary

| Area | Before | After |
|------|--------|-------|
| Helm | Go SDK `helm.sh/helm/v3` v3.18.5 (Helm v3, EOL Sep 2026) | Go SDK `helm.sh/helm/v4` v4.2.2; **Helm v3 compatibility live-verified** |
| Runtime dependencies | `helm` + **`kubectl`** (shelled out) | **`helm` only** — readiness uses the embedded Kubernetes client |
| Security — code-reachable CVEs | 2 (`golang.org/x/net`) | **0** (`govulncheck`) |
| Security — static analysis | 11 `gosec` findings | **0** (justified `#nosec` where applicable) |
| Linting | `go vet` failing; no aggregate linter | **golangci-lint** (gocritic, revive, errorlint, …) + vet, all green |
| Automated tests | none (0 `*_test.go`, 0% coverage) | **76 tests**, ~75% statement coverage, + a live integration test |
| Interfaces | CLI only | CLI + `--output json` plan + embedded web UI (live status) + `uninstall`/`--prune` |
| Supply chain | unsigned release, no checksums | static `CGO_ENABLED=0` builds, `SHA256SUMS`, **cosign** signatures, **SPDX SBOM**, build provenance |
| Module path | `github.com/gemalto/helm-spray/v4` (defunct org) | `github.com/ThalesGroup/helm-spray/v5` (canonical, SemVer-correct) |
| CI | release workflow only (Go 1.22, old actions) | CI (build, gofmt, vet, lint, gosec, govulncheck, e2e/kind) + hardened release |
| Governance | LICENSE, brief CONTRIBUTING | + SECURITY, Code of Conduct, issue/PR templates, CODEOWNERS, dependabot |

## Live proof: the Skyward ATC reference deployment

A companion multi-service system — [Skyward ATC](https://github.com/samlister-thales/SkywardATC),
a fictional air-traffic-control platform of nine weighted services — was deployed
end-to-end through the modernised helm-spray onto a real Kubernetes cluster, and
used to **find and validate fixes**: planning the real umbrella surfaced the
tag-semantics defect below.

**Helm v3 vs v4, apples-to-apples** (Helm v3.16.4 and v4.2.2, same chart, same cluster):

| | Helm v3 | Helm v4 |
|---|---|---|
| **New helm-spray (5.0.0)** | ✅ 9/9 services deployed (1m14s) | ✅ 9/9 services deployed (1m11s) |
| **Old helm-spray (baseline)** | ❌ **0 deployed** — silently skipped every tagged sub-chart ("completed in 0s") | — |

The new plugin behaves identically across Helm v3 and v4; the old one deploys
*nothing* for an idiomatic tagged umbrella (the inverted tag default, now fixed).

## Security

- **Vulnerabilities:** two code-reachable advisories in `golang.org/x/net`,
  reached through the value-merge path, were cleared by the dependency refresh;
  `govulncheck` now reports none.
- **Static analysis:** `gosec` went from 11 findings to 0. Chart fetching no
  longer shells out (a G204 anti-pattern) nor writes into the working directory;
  removing the kubectl wrapper also removed the go-template-injection surface.
- **No external `kubectl`:** readiness now talks to the Kubernetes API through the
  embedded client, honouring helm's `--kube-context`/`HELM_KUBE*` settings, so it
  checks the same cluster helm deploys to.
- **Web UI:** read-only (never mutates the cluster), loopback-bound with a warning
  otherwise, request bodies capped, all input HTML-escaped, and remote
  chart/value-file references rejected (case-insensitively) to prevent SSRF.
- **Secret hygiene:** `--set`/`--set-string`/`--set-file` values are redacted in
  debug logs.
- **Supply chain:** static multi-arch builds, published `SHA256SUMS`, keyless
  cosign signatures, an SPDX SBOM and build-provenance attestation; the install
  script verifies the download and warns (rather than silently proceeding) when it
  cannot.

## Dependencies & compatibility

- Migrated the Go SDK from Helm v3 to Helm v4 (`helm.sh/helm/v4` v4.2.2), adapting
  to the v4 package reorganisation; refreshed `cobra` and the `k8s.io` libraries
  (v0.36.2).
- **Helm v3 compatibility (live-verified):** the plugin drives the `HELM_BIN` the
  host helm provides, detects the host major version and emits version-appropriate
  flags (`--force` on v3, `--force-replace` on v4) — confirmed by the
  apples-to-apples deployment above.

## Correctness (defects fixed)

- **Tag semantics now match Helm:** a tag is enabled by default, so a tagged
  sub-chart is sprayed unless *every* one of its tags is explicitly false.
  Previously a tag had to be set true, so an idiomatic tagged umbrella deployed
  nothing — the defect made visible in the v3/v4 comparison above.
- A missing sub-chart weight defaults to 0 instead of aborting the run.
- `OnDelete` StatefulSets no longer hang the readiness wait (revisions never
  converge under that strategy); readiness uses ready replicas instead.
- A sub-chart consisting only of hooks (empty rendered manifest) deploys instead
  of stalling.
- Rendered manifests split on the YAML document boundary (`^---$`) rather than the
  bare substring `---`.
- Jobs readiness honours `.spec.completions` and fails fast on a failed Job.
- `--timeout` accepts a Helm-style duration (`5m`, `300s`) as well as seconds, and
  rejects positive sub-second values rather than truncating to 0.
- `--prefix-releases` is validated; `--reset-values`+`--reuse-values` is rejected;
  a `go vet`-failing format string was fixed.

## Performance

- Readiness queries each workload kind once via a typed client `List` (with a
  per-call timeout) and matches names through a map, instead of a `kubectl`
  subprocess per kind and a linear scan.
- Capped exponential back-off (1s→5s) replaces a fixed sleep.
- The per-release `.enabled=false` set is precomputed once (O(1) per release); the
  weight loop visits only the distinct weights that occur (computed once); the
  sub-chart appVersion lookup uses a map rather than a nested scan.
- `context.Context` cancellation terminates in-flight helm processes on
  SIGINT/SIGTERM.

## Maintainability & architecture

- `HelmClient` and `ReadinessChecker` interfaces let the orchestrator be
  unit-tested with fakes; the readiness package uses a fake-clientset seam.
- Threaded `context.Context` from `main` through cobra into the wrappers; wrapped
  errors with `%w`; user-facing errors now name the pending workloads / release.
- Module path bumped to `/v5` (SemVer-correct for a v5 release) across all imports
  and the build.

## Usability

- **Plan preview:** `--output json` prints the weight-ordered plan without touching
  the cluster (clean stdout for CI).
- **Web UI:** `helm spray ui [CHART]` opens **pre-configured** for a chart and
  shows **live release status** (read-only), with a version indicator and
  light/dark theme. Deploy/uninstall/prune stay on the CLI.
- **Lifecycle:** `helm spray uninstall` (reverse-weight teardown) and `--prune`
  (remove releases for sub-charts dropped from the umbrella).
- **CLI:** added `--namespace/-n` (the baseline had none), refreshed help text,
  rewrote the README with a Prerequisites section and examples.

## Testing

| Package | Coverage |
|---------|----------|
| pkg/util | 100.0% |
| internal/dependencies | 86.3% |
| internal/log | 84.2% |
| internal/gui | 76.8% |
| pkg/readiness | 76.4% |
| pkg/helmspray | 75.1% |
| internal/values | 73.9% |
| cmd | 69.2% |
| pkg/helm | 58.5% |

From **0 to 76 tests** (~75% statement coverage). Unit tests cover the pure logic
(weight/tag/plan analysis, value includes, argument construction, readiness
predicates, CLI validation, GUI handlers, error UX) using runner/clientset seams;
a build-tagged integration test performs a real weight-ordered rollout against a
live cluster (also wired into CI via a kind job). The exec-only `helm` paths are
the main residual gap.

## Independent review

Two multi-agent formal reviews were run (each ~14 agents) with **adversarial
verification** of every finding:

1. A **scope-completion audit** scored each original goal against the code.
2. A **recent-changes review** of the client-go/security/performance work
   confirmed **7 of 8** flagged issues — including a high-severity web-UI SSRF
   case-bypass and the readiness/`--kube-context` gap — **all of which were
   fixed**. (These followed four earlier scored review rounds, 8.66 → 9.14.)

Scope scorecard (audit snapshot; the remaining gaps were then closed by the
client-go migration, the security/perf fixes and the documentation pass):

| Scope item | Score | Scope item | Score |
|---|---|---|---|
| Bug identification & correction | 95% | Security | 88% |
| Dependencies (v4 + v3 compat) | 92% | Performance | 88% |
| GUI (config + visual) | 93% | Refactoring (maintainability) | 85% |
| Code explanations | 90% | Onboarding (README) | 85% |
| Full-coverage tests | 80% | Technical documentation | 82% |

## Reference

- Baseline: [00-baseline-before.md](00-baseline-before.md)
- Deep-review worklist: [01-deep-review-findings.md](01-deep-review-findings.md)
- Upstream issue triage: [upstream-issues-triage.md](upstream-issues-triage.md)
- Roadmap: [../roadmap/capability-roadmap.md](../roadmap/capability-roadmap.md)
- Companion demo: [Skyward ATC](https://github.com/samlister-thales/SkywardATC).
