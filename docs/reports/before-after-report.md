# helm-spray Modernisation — Before / After Report

This report summarises the modernisation of helm-spray from a stale Helm v3
plugin into a first-class, Helm v4 open-source package. It compares the state at
the branch point (`d98d7a5`) with the modernised branch.

## Executive summary

| Area | Before | After |
|------|--------|-------|
| Helm | Go SDK `helm.sh/helm/v3` v3.18.5 (Helm v3, EOL Sep 2026) | Go SDK `helm.sh/helm/v4` v4.2.2, plus Helm v3 CLI compatibility |
| Security — code-reachable CVEs | 2 (`golang.org/x/net`) | **0** (`govulncheck`) |
| Security — static analysis | 11 `gosec` findings | **0** (justified `#nosec` where applicable) |
| Automated tests | none (0 `*_test.go`, 0% coverage) | 31 tests across 10 files + a live integration test |
| Testability | core logic untestable (no seams) | interface-injected, context-aware, unit-tested orchestration |
| Interfaces | CLI only | CLI + `--output json` plan + embedded web UI |
| go vet / gofmt | failing (`vet`), unformatted files | clean |
| Module path | `github.com/gemalto/helm-spray` (defunct org) | `github.com/ThalesGroup/helm-spray` (canonical) |
| CI | release workflow only (Go 1.22, old actions) | CI (build, vet, test, gosec, govulncheck) + modernised release |
| Governance | LICENSE, brief CONTRIBUTING | + SECURITY, Code of Conduct, issue/PR templates |

## Security

- **Vulnerabilities:** two code-reachable advisories in `golang.org/x/net`
  (GO-2026-5026, GO-2026-4918), reached through the value-merge path, were
  cleared by the dependency refresh; `govulncheck` now reports none.
- **Static analysis:** `gosec` went from 11 findings to 0. The chart-fetch path
  no longer shells out (`ls`/`cp` via `sh -c`, a G204 anti-pattern) and no longer
  writes into the current working directory (a G304-adjacent issue); kubectl
  readiness no longer builds go-templates by interpolating object names
  (template-injection surface). Subprocess invocations that legitimately use the
  host-provided `HELM_BIN` carry justified `#nosec G204` annotations.
- **Secret hygiene:** `--set`/`--set-string`/`--set-file` values are redacted in
  debug command logs; the web UI bounds request bodies and warns on non-loopback
  binds.

## Dependencies & compatibility

- Migrated the Go SDK from Helm v3 to Helm v4 (`helm.sh/helm/v4` v4.2.2),
  adapting to the v4 package reorganisation (chart `v2`, `chart/common`,
  `chart/common/util`).
- Refreshed `spf13/cobra` (v1.9.1 → v1.10.2) and the `k8s.io` libraries
  (v0.33.3 → v0.36.2); toolchain Go 1.24.
- **Helm v3 compatibility retained:** the plugin drives the `HELM_BIN` the host
  helm provides, detects the host helm major version, and emits
  version-appropriate flags (`--force` on v3, `--force-replace` on v4), so it
  works against both Helm v3 and Helm v4 during the v3 end-of-life transition.

## Correctness (defects fixed)

- A sub-chart that omits its weight now defaults to weight 0 (as documented)
  instead of aborting the entire spray.
- Tag matching now accepts string values (e.g. from a YAML values file), not
  only Go booleans.
- Rendered manifests are split on YAML document boundaries (`^---$`) rather than
  the bare substring `---`, so a resource whose content contains `---` is no
  longer mis-split and dropped from the readiness wait.
- Jobs readiness honours `.spec.completions` (parallel/indexed Jobs) and fails
  fast on a failed Job instead of waiting out the timeout.
- `--prefix-releases` is validated against its documented character set;
  `--reset-values` together with `--reuse-values` is rejected; the doubled-hyphen
  release-prefix log message is corrected; a non-constant format string that
  failed `go vet` was fixed.

## Performance

- Readiness polling uses capped exponential back-off (1s→5s) instead of a fixed
  5s sleep, reducing latency for fast deployments.
- Job readiness is checked with a single `kubectl get` per cycle rather than one
  subprocess per Job per cycle.
- The per-release `.enabled` value set is precomputed once (O(1) per release)
  rather than rebuilt for every release (previously O(n²) across a run).
- Manifest decoding accumulates ignored fragments only in verbose mode.

## Maintainability & architecture

- Introduced `HelmClient` and `ReadinessChecker` interfaces; the orchestrator
  depends on them and defaults to CLI-backed implementations, so the weight
  loop, upgrade and wait logic are unit-testable with fakes.
- Threaded `context.Context` from `main` through cobra into the helm/kubectl
  wrappers (`exec.CommandContext`); SIGINT/SIGTERM now cancels in-flight child
  processes.
- Separated configuration from per-tier scratch state; replaced the bespoke
  logger internals with an injectable, testable design; wrapped errors with
  `%w`; replaced deprecated `io/ioutil`; added godoc to exported symbols.

## Usability

- **Plan preview:** `--output json` prints the weight-ordered deployment plan
  without touching the cluster; stdout carries only the JSON (diagnostics go to
  stderr) so it pipes cleanly into CI.
- **Web UI:** `helm spray ui` serves a single-binary web application to configure
  a spray and visualise the umbrella chart as ordered weight tiers.
- **CLI:** added the documented `--namespace/-n` flag; refreshed the help text
  (removed the obsolete Helm v2 "Tiller" reference); rewrote the README and added
  governance documents.

## Testing

| Package | Coverage |
|---------|----------|
| internal/log | 84.2% |
| internal/dependencies | 82.9% |
| cmd | 67.8% |
| pkg/helmspray | 64.6% |
| internal/values | 58.1% |
| internal/gui | 58.1% |
| pkg/kubectl | 38.2% |
| pkg/helm | 26.9% |
| pkg/util | 100% |

Unit tests cover the pure logic (weight/tag/plan analysis, value includes, arg
construction, readiness predicates, CLI validation, GUI handlers, logging). The
exec/IO paths (which shell out to helm and kubectl) are exercised by a
build-tagged integration test that performs a real weight-ordered rollout
against a live cluster.

## New capabilities & future roadmap

Delivered: Helm v4 support, plan/`--output json`, topology visualisation (web
UI), DaemonSet readiness gating, back-off polling, structured client interfaces.

A prioritised backlog of further capabilities (dependency-graph ordering,
aggregate diff, atomic/rollback orchestration, intra-tier parallelism, release
pruning, secrets handling) is maintained in
[../roadmap/capability-roadmap.md](../roadmap/capability-roadmap.md).

## Independent review

A formal multi-expert review (Go idiom, security, Helm domain, testing,
documentation, CLI/UX, OSS readiness) was run with adversarial verification of
findings; all verified blocker and major findings were resolved, and a scored
re-review confirmed the result.

<!-- REVIEW_SCORES -->

## Reference

- Baseline: [00-baseline-before.md](00-baseline-before.md)
- Deep-review worklist: [01-deep-review-findings.md](01-deep-review-findings.md)
- Companion demo: a fictional air-traffic-control reference deployment exercising
  weight-ordered, multi-service rollout.
