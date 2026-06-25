# helm-spray — "Before" Baseline (Phase 3)

> Living document. Captured at the start of the modernization program so the final
> before/after report (Goal 5) is measurable. Branch:
> `AU_Thales-Against-The-Machine_2026June_ClaudeCode_SL`. Date: 2026-06-25.
> NOT yet committed.

## Toolchain (dev environment, Goal 1)
| Tool | Version |
|------|---------|
| Go | 1.26.4 (darwin/arm64) |
| Helm | v4.2.2 |
| kubectl | v1.36.2 |
| Container runtime | Pending (Docker Desktop cask needs sudo/GUI; runtime choice under review) |
| Local K8s | Pending runtime decision |

## Build & static analysis
- `go build ./...` — **passes**.
- `go vet` / `go test` — **FAILS vet**: `internal/dependencies/dependencies.go:133:16: non-constant format string in call to log.Info` (Go 1.24+ printf analyzer). Real latent defect — log.Info is a Printf-like sink fed a dynamic string. → Phase 4 bug item.
- Source size: **1356 Go LOC** across 9 files (excl. vendor).

## Security — `govulncheck` (code-reachable)
2 vulnerabilities the code actually calls, both transitive via `golang.org/x/net@v0.42.0` (pulled by the Helm SDK), reachable through `internal/values/values.go:51` (`values.Merge` → HTTP getter):
| ID | Issue | Fixed in |
|----|-------|----------|
| GO-2026-5026 | idna ASCII Punycode label rejection failure | x/net v0.55.0 |
| GO-2026-4918 | HTTP/2 transport infinite loop on bad SETTINGS_MAX_FRAME_SIZE | x/net v0.53.0 |

Plus 8 vulns in imported (uncalled) packages and 2 in required modules. → resolved by dependency upgrade in Phase 4.

## Security — `gosec` (static)
- **11 issues** across 9 files / 1356 lines, 0 `#nosec`.
- Includes G104 (unhandled errors), e.g. `internal/log/log.go:59,61` `os.Stderr.WriteString` returns ignored; G204-class subprocess construction in helm/kubectl wrappers (to review). → full report + triage in Phase 4 / formal security review.

## Test coverage
- **0.0% across every package.** No `*_test.go` files exist. → Goal 4: full automated coverage.

## Dependencies (outdated direct)
| Module | Current | Latest |
|--------|---------|--------|
| github.com/spf13/cobra | v1.9.1 | v1.10.2 |
| helm.sh/helm/v3 | v3.18.5 | v3.21.2 (→ migrating to helm.sh/helm/v4) |
| k8s.io/api | v0.33.3 | v0.36.2 |
| k8s.io/client-go | v0.33.3 | v0.36.2 |

Module path still `github.com/gemalto/helm-spray/v4` (legacy Gemalto org; decide on rename in Phase 4).

## Capabilities (current, for before/after matrix)
CLI-only; weight-based sequential subchart deploy; per-subchart releases; `--target`/`--exclude`; tags; value-file include directives; release prefixing; readiness wait (Deployment/StatefulSet/Job via kubectl); dry-run. **Absent:** GUI/visualization, dependency DAG, diff/preview, drift/GitOps, secrets, intra-stage parallelism, rollback orchestration, automated tests.
