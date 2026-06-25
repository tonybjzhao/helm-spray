# Release Notes

## Version 5.0.0 - 06/25/2026

Modernisation of helm-spray as a Helm v4 plugin.

* Migrated the Go SDK from Helm v3 to Helm v4 and refreshed all dependencies,
  clearing two code-reachable advisories (`govulncheck` reports none).
* Detect the host helm version and emit version-appropriate flags (`--force` on
  v3, `--force-replace` on v4); drive the `HELM_BIN` provided by the host.
* Fixed defects: a missing sub-chart weight now defaults to 0 instead of
  aborting the run; tag matching accepts string values from value files;
  rendered manifests are split on YAML document boundaries; `--prefix-releases`
  is validated; `--reset-values` together with `--reuse-values` is rejected.
* **Behaviour change:** tag handling now matches Helm's own semantics — a tag is
  enabled by default, so a tagged sub-chart is sprayed unless every one of its
  tags is explicitly set to `false`. Previously a tag had to be explicitly set
  `true`, which inverted Helm's default and meant an idiomatic tagged umbrella
  deployed nothing unless every tag was passed on the command line.
* Hardened chart fetching (pure Go, no shell, no current-directory writes) and
  redacted secret `--set`/`--set-string`/`--set-file` values from debug logs.
* Reworked readiness gating: typed checks for Deployments, StatefulSets,
  DaemonSets and Jobs (Jobs honour `.spec.completions` and fail fast on a failed
  Job), with capped exponential back-off polling.
* Introduced client interfaces and `context.Context` propagation (SIGINT/SIGTERM
  cancels in-flight helm/kubectl processes), enabling automated unit and
  integration tests from zero prior coverage.
* Added `--output json` to print the weight-ordered deployment plan without
  contacting the cluster.
* `--timeout` now accepts a Helm-style duration (`5m`, `300s`) as well as a bare
  number of seconds, so the value can be written the same way as for `helm`.
* Added a `helm spray ui` embedded web interface to configure and visualise a
  deployment, with a read-only live-status view that colours the plan as each
  release reports its helm status, a helm-host version indicator, and a
  light/dark theme. Deploying stays on the CLI; the UI never mutates the cluster.
* Added a `helm spray uninstall [CHART]` command that removes the releases
  created for an umbrella chart's sub-charts in reverse weight order, and a
  `--prune` flag that, after deploying, uninstalls releases for sub-charts that
  are no longer part of the umbrella chart.
* Hardened the release supply chain: per-release `SHA256SUMS`, keyless cosign
  signatures, an SPDX SBOM, and build-provenance attestation; the install script
  now verifies the download against the published checksum.
* Rewrote the README; added SECURITY and Code of Conduct documents, issue/PR
  templates, and a CI workflow (build, gofmt, vet, test, golangci-lint, gosec,
  govulncheck, plus an end-to-end job that runs the integration suite against a
  kind cluster). Added a curated `.golangci.yml` and an `.editorconfig`.

## Version 4.0.13 - 11/27/2024
* Bump to helm v3.16.3, k8s.io/api v0.31.3, go 1.22, and k8s.io/client-go v0.31.3
* Fixed [`#92`](https://github.com/ThalesGroup/helm-spray/issues/92) (barmaths)
* Fixed [`#86`](https://github.com/ThalesGroup/helm-spray/issues/86) (dongbeiqing91)

## Version 4.0.12 - 02/03/2024
* Updated build dependencies

## Version 4.0.12 - 07/07/2023
* Updated build dependencies

## Version 4.0.12 - 12/15/2022
* Updated build dependencies

## Version 4.0.11 - 06/29/2022
* Add OCI support to fetch umbrealla chart [`#76`](https://github.com/ThalesGroup/helm-spray/issues/76) (cvila84)
* Updated build dependencies

## Version 4.0.10 - 08/24/2021
* Reducing excessing verbose logs on helm upgrade ignored parts
* Added error management when readiness kubectl template cannot be executed

## Version 4.0.9 - 07/12/2021
* Fixed [`#73`](https://github.com/ThalesGroup/helm-spray/issues/73) (cvila84)

## Version 4.0.8 - 06/24/2021
* Exposed helm install/update --create-namespace flag on spray. Since 4.0.6, --create-namespace is automatically passed to helm install/update commands but because it is trying to create namespace even if it already exists, it can generate errors when user rights on cluster do not include namespace creation  

## Version 4.0.7 - 02/03/2021
* Fixed [`#71`](https://github.com/ThalesGroup/helm-spray/issues/71) (Elassyo)

## Version 4.0.6 - 01/08/2021
* Fixed [`#69`](https://github.com/ThalesGroup/helm-spray/issues/69) (Elassyo)

## Version 4.0.5 - 10/26/2020
* Fixed [`#64`](https://github.com/ThalesGroup/helm-spray/issues/64) (AYDEV-FR)

## Version 4.0.4 - 10/16/2020
* Fixed [`#66`](https://github.com/ThalesGroup/helm-spray/issues/66)

## Version 4.0.3 - 10/13/2020
* Add Darwin support [`#65`](https://github.com/ThalesGroup/helm-spray/pull/65) (Bazze)

## Version 4.0.2 - 09/21/2020
* Expose spray logic in a library module for external usage

## Version 4.0.1 - 06/18/2020
* Fixed [`#55`](https://github.com/ThalesGroup/helm-spray/issues/55)

## Version 4.0.0 - 06/11/2020
* Bump to helm v3 (this version is NOT compatible with helm v2)

## Version 3.4.6 - 03/20/2020
* Plugin installation via helm plugin install is now possible

## Version 3.4.5 - 10/08/2019
* Bugfix issues #39, #40 and #41 [`#42`](https://github.com/gemalto/helm-spray/pull/42) (Patrice Amiel)

## Version 3.4.4 - 09/04/2019
* Support other Deployment/StatefulSet versions for the 'waiting for' phase [`#30`](https://github.com/gemalto/helm-spray/pull/30) (Patrice Amiel) 
* Support of Helm Tags [`#35`](https://github.com/gemalto/helm-spray/pull/35) (Patrice Amiel) 

## Version 3.4.3 - 07/15/2019
* Support of new flags (--exclude, --set-file, --set-string) + bugfix #! .File.Get clause [`#28`](https://github.com/gemalto/helm-spray/pull/28) (Patrice Amiel) 

## Version 3.4.2 - 05/23/2019
* Bugfix regexp for '.File.Get' for windows [`3a2a527`](https://github.com/gemalto/helm-spray/commit/3a2a5279f078391e7d8b421d7e3aa69f425ebcac) (Patrice Amiel)

## Version 3.4.1 - 05/23/2019
* Bump to go 1.11 [`ea90f7a`](https://github.com/gemalto/helm-spray/commit/ea90f7a686065dec9a9308bce4ebc3ac03a8dd4a) (Christophe Vila)

## Version 3.4.0 - 05/22/2019
* Support of "Multiple values files in the umbrella chart" [`#20`](https://github.com/gemalto/helm-spray/pull/20) [`#21`](https://github.com/gemalto/helm-spray/pull/21) (Patrice Amiel)

## Version 3.3.0 - 03/25/2019
* Enhance "wait liveness and readiness" and create capability to prefix releases names [`#16`](https://github.com/gemalto/helm-spray/pull/16) (Patrice Amiel)

## Version 3.2.1 - 02/14/2019
* Bugfixes on "waiting for Liveness and Readiness" step [`#14`](https://github.com/gemalto/helm-spray/pull/14) (Patrice Amiel)

## Version 3.2.0 - 02/01/2019
* Fix plugin.yaml executable name according to OS [`#5`](https://github.com/gemalto/helm-spray/pull/5) (Christophe Vila)
* Support of alias and of the '--force' option [`#3`](https://github.com/gemalto/helm-spray/pull/3) (Patrice Amiel)

## Version 3.1.0 - 01/27/2019
* Adding support of several -f clauses
* Adding debug option 
* Supporting HELM_DEBUG envar to get the debug mode as helm is not forwarding the --debug option

## Version 3.0.0 - 11/10/2018
* First delivery on Github.
