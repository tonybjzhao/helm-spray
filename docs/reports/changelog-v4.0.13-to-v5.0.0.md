# helm-spray — Changes from v4.0.13 to v5.0.0

A curated summary of the net changes between the previous release (**v4.0.13**)
and **v5.0.0**, grouped by theme. This complements the release notes in
[CHANGELOG.md](../../CHANGELOG.md); it lists changes relative to v4.0.13 only.

> **At a glance:** Helm v4 with verified Helm v3 compatibility · `kubectl` no
> longer required · automated tests from zero · a configurable web UI with live
> status · `uninstall`/`--prune` · a signed, SBOM-backed supply chain.

---

## Breaking changes

- **Go module path** is now `github.com/ThalesGroup/helm-spray/v5` (was
  `github.com/gemalto/helm-spray/v4`). Required for the v5 release under Go's
  semantic import versioning; update import paths accordingly.
- **Helm v4 SDK.** The plugin is built against `helm.sh/helm/v4`. It remains
  compatible with both Helm v3 and Helm v4 hosts at runtime (see below).
- **Tag semantics now match Helm.** A dependency tag is *enabled by default*: a
  tagged sub-chart is deployed unless **every** one of its tags is explicitly set
  to `false`. Previously a tag had to be explicitly set `true`, which meant an
  umbrella whose dependencies all carried tags deployed nothing unless every tag
  was passed on the command line.

## Compatibility

- **Helm v3 and v4 both supported.** helm-spray drives the `HELM_BIN` exported by
  the host helm, detects its major version, and emits version-appropriate flags
  (`--force` on v3, `--force-replace` on v4). Verified by deploying the same
  multi-service umbrella under Helm v3.16.4 and v4.2.2 with identical results.
- **`kubectl` is no longer required.** Workload readiness is checked through the
  embedded Kubernetes client using the ambient kubeconfig (and helm's
  `--kube-context`/`HELM_KUBE*` settings). `helm` is now the only external tool
  helm-spray needs.

## New features

- **`--output json`** prints the resolved, weight-ordered deployment plan without
  contacting the cluster — useful for previews and CI gating. Diagnostics go to
  stderr so stdout carries only the JSON.
- **Embedded web UI** (`helm spray ui`): a single-binary web application to
  configure a spray and visualise the umbrella chart as ordered weight tiers.
  - Launch it pre-configured for a chart — `helm spray ui ./chart -n ns` opens
    with the form filled in and **live release status** shown immediately.
  - Read-only live status colours each release as it reaches its helm status; a
    version indicator shows the helm host version; light/dark theme.
- **`helm spray uninstall [CHART]`** removes a solution's releases in reverse
  weight order (honouring `--target`/`--exclude`/prefixes; idempotent).
- **`--prune`** uninstalls releases for sub-charts that are no longer part of the
  umbrella, scoped to releases owned by the same umbrella.
- **`--namespace/-n`** flag (the baseline had none; namespace came only from the
  environment).
- **`--timeout`** accepts a Helm-style duration (`5m`, `300s`) as well as a bare
  number of seconds.

## Bug fixes

- A missing sub-chart `weight` now defaults to 0 instead of aborting the run.
- `OnDelete` StatefulSets no longer hang the readiness wait (their revisions never
  converge under that strategy).
- A sub-chart consisting only of helm hooks (empty rendered manifest) now deploys
  instead of stalling.
- Rendered manifests are split on the YAML document boundary (`^---$`) rather than
  the bare substring `---`, so resources whose content contains `---` are no
  longer mis-split.
- Jobs readiness honours `.spec.completions` (parallel/indexed Jobs) and fails
  fast on a failed Job rather than waiting out the timeout.
- Tag values are accepted as strings (e.g. from a YAML values file), not only Go
  booleans.
- `--prefix-releases` is validated against its documented character set;
  `--reset-values` together with `--reuse-values` is rejected; a `go vet`-failing
  format string was corrected.
- Release binaries are statically linked (`CGO_ENABLED=0`), resolving GLIBC
  load failures on some hosts.

## Security

- Chart fetching is pure Go (no shell), eliminating a command-injection vector,
  and no longer writes into the working directory.
- `--set`/`--set-string`/`--set-file` values are redacted from debug logs.
- The web UI is read-only (never mutates the cluster), binds to loopback with a
  warning otherwise, caps request bodies, HTML-escapes all input, and rejects
  remote chart/value-file references (preventing server-side request forgery).
- Supply chain: published `SHA256SUMS`, keyless **cosign** signatures, an **SPDX
  SBOM**, and build-provenance attestation; the install script verifies the
  download against the published checksum.
- `gosec` and `govulncheck` run in CI and report no findings.

## Performance

- Readiness queries each workload kind once through a typed client `List` with a
  per-call timeout (previously one `kubectl` subprocess per kind), and matches
  names through a map.
- Readiness polling uses capped exponential back-off (1s → 5s).
- The per-release `.enabled=false` set is precomputed once; the weight loop visits
  only the distinct weights present; sub-chart appVersions are indexed in a map.
- `context.Context` cancellation terminates in-flight helm processes on
  SIGINT/SIGTERM.

## Quality, testing & CI

- **From 0 to 76 automated tests** (~75% statement coverage), enabled by
  `HelmClient`/`ReadinessChecker` interface seams and a fake Kubernetes clientset.
- A build-tagged integration test runs a real weight-ordered rollout against a
  live cluster, wired into CI via a kind job.
- CI gates: build, `gofmt`, `go vet`, **golangci-lint**, `gosec`, `govulncheck`,
  unit tests (race), and the e2e kind job.
- Hardened release workflow; `dependabot`, `CODEOWNERS`.

## Documentation & governance

- Rewritten `README.md` (compatibility, prerequisites, quickstart, flags,
  how-it-works), plus `SECURITY.md`, `CODE_OF_CONDUCT.md`, expanded
  `CONTRIBUTING.md`, and issue/PR templates.
- Package-level and exported-symbol godoc throughout; explanatory comments on the
  non-obvious logic (tag defaults, OnDelete readiness, the enable/disable
  mechanism).
