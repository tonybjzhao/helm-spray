# helm-spray — Deep Review Worklist (47 verified findings)

> Authoritative remediation checklist for Phase 4. Findings were produced by a
> 6-dimension review, each adversarially verified. 6 high, 15 medium, 26 low.
> Not committed; drives the modernization. Date 2026-06-25.

## HIGH (6) — Helm v4 migration + testability foundation
- [ ] **go.mod**: replace `helm.sh/helm/v3 v3.18.5` → `helm.sh/helm/v4`; `go mod tidy` re-resolves k8s.io/* and oras (also clears x/net CVEs). Keep module path `.../v4` (that is helm-spray's own version).
- [ ] **SDK imports** (helmspray.go:12-13, values.go:6-9, dependencies.go:7-8): `helm.sh/helm/v3/...` → `v4`; reconcile moved packages.
- [ ] **loader.Load** (helmspray.go:57): verify signature + `chart.Chart` fields (Raw, Files, Values, Metadata.Dependencies, Dependencies()).
- [ ] **chartutil** (values.go:32-45): verify CoalesceValues/ReadValues/Values type/ValuesfileName; add fixture test asserting merged values.
- [ ] **--force flag** (helm.go:110-112): v4 CLI flag/semantics differ; branch by detected helm version.
- [ ] **Interface seam** (helm.go, kubectl.go, helmspray.go): introduce HelmClient + KubeReadiness interfaces held by Spray; inject fakes — unblocks all unit testing.

## MEDIUM (15)
- [ ] correctness: missing sub-chart weight → hard error instead of default 0 (dependencies.go:82-85) — handle ErrNoValue.
- [ ] correctness: non-constant format string to log.Info (dependencies.go:133) — pass format+args separately.
- [ ] security: fetched chart copied into CWD / path traversal surface (helm.go:174) — keep in temp dir, return sanitized abs path.
- [ ] security: secrets in --set echoed in debug logs + full values dumped on merge error (helm.go:122, values.go:40) — redact.
- [ ] perf: AreJobsReady spawns one kubectl per job per poll (kubectl.go:45-67) — single templated call.
- [ ] perf: fixed 5s poll + serial readiness checks (helmspray.go:256,262-304) — backoff + concurrent checks + configurable.
- [ ] helm-v4: no version detection (helm.go:49) — probe `helm version` once, cache, branch.
- [ ] helm-v4: hardcoded 'helm'/'kubectl' (helm.go:124, kubectl.go) — use HELM_BIN/KUBECTL env.
- [ ] maintainability: no context.Context (helmspray/helm/kubectl) — thread cmd.Context(), exec.CommandContext.
- [ ] maintainability: bespoke log level→prefix mapping (log.go:12-30) — slog or injectable writer + named constants.
- [ ] maintainability: Fetch shells out to sh/cmd for ls+cp (helm.go:166-188) — pure-Go os.ReadDir + io.Copy.
- [ ] maintainability: Spray struct mixes config + mutable state (helmspray.go:25-45...) — upgrade() returns workload set; wait() takes it.
- [ ] domain: tag matching only accepts bool true, ignores string "true" (dependencies.go:73) — strconv.ParseBool + warn.
- [ ] domain: only Deploy/STS/Job gate readiness; DaemonSet/etc ignored (helmspray.go:210) — extend + document.
- [ ] domain: AreJobsReady ignores completions>1 and failed jobs (kubectl.go:45) — compare to spec.completions; fail fast on failed.

## LOW (26)
- [ ] correctness: double-dash in release-prefix log (helmspray.go:103).
- [ ] correctness: WithNumberedLines forwards preformatted + params (log.go:51).
- [ ] correctness: manifest split on bare "---" (helmspray.go:211) — split on `(?m)^---$`.
- [ ] correctness: misreported error when picked sub-value not string (values.go:154-155).
- [ ] correctness: dead GetDeployments/StatefulSets/Jobs/getWorkloads + empty-split bug (kubectl.go:25-35) — remove or guard.
- [ ] security: fetch relocate via sh -c / cmd /C G204 (helm.go:167-188) — pure-Go (same as above).
- [ ] security: kubectl go-template built by string concat from names (kubectl.go:119-142) — escape/validate or fixed argv.
- [ ] perf: O(n^2) rebuild of `.enabled` --set per upgrade (helmspray.go:168-178) — precompute base.
- [ ] perf: AppVersion resolution O(n*m) (dependencies.go:110-115) — prebuild name→AppVersion map.
- [ ] perf: per-poll go-template rebuilt + extra debug kubectl per poll (kubectl.go:80-142) — compute once per tier.
- [ ] perf: manifest deserialize full objects per release (helmspray.go:210-228) — lightweight TypeMeta/ObjectMeta; ignoredParts only if verbose.
- [ ] helm-v4: `helm fetch`/OCI behaviour + single-file assumption (helm.go:147) — filter to .tgz; verify v4.
- [ ] helm-v4 + maintainability: io/ioutil deprecated (helm.go:20, helmspray.go) — os.MkdirTemp/os.CreateTemp.
- [ ] maintainability: leaf exec errors not wrapped with %w (helm.go, kubectl.go) — add context.
- [ ] maintainability: magic numbers/strings (helmspray.go:256, root.go:162,181, kubectl.go:59) — named constants.
- [ ] maintainability: duplicated chart-fetch logic (root.go:115-142) — extract helper.
- [ ] maintainability: duplicated readiness blocks in wait() (helmspray.go:262-304) — table-driven.
- [ ] maintainability: comparison to boolean literals (multiple) — idiomatic.
- [ ] maintainability: missing/placeholder godoc on exported symbols (multiple).
- [ ] domain: doubled hyphen prefix log (helmspray.go:103) — dup of above.
- [ ] domain: --prefix-releases not validated vs documented charset (root.go:152) — validate.
- [ ] domain: per-bucket name-based readiness can collide across releases (helmspray.go:123) — scope by instance label / document.
- [ ] domain: per-release upgrade overrides user .enabled + replays all --set (helmspray.go:168) — document/warn.
- [ ] domain: reset-values + reuse-values both passable; precedence delegated (helm.go:104, values.go:27) — enforce precedence.
- [ ] domain: checkTargetsAndExcludes runs after dependencies.Get/log/list (helmspray.go:117) — move earlier, fail fast.
- [ ] (coverage) add full automated tests once seams exist — Phase 4d.
