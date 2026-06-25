# Helm Spray

![helm-spray](https://thalesgroup.github.io/helm-spray/logo/helm-spray_150x150.png)

A Helm plugin that installs or upgrades the sub-charts of an umbrella chart
**one tier at a time, in a deterministic order**, creating one Helm release per
sub-chart so each can later be upgraded individually.

It behaves like `helm upgrade --install`, except that it deploys each sub-chart
according to a **weight** (an integer `>= 0`) declared per sub-chart: all
sub-charts of weight 0 are processed first, then weight 1, and so on. A tier is
fully deployed and **ready** before the next tier starts.

## Why

Real solutions are layered: a datastore must be ready before the services that
use it, which must be ready before the operator-facing edge. Expressing that
ordering with a single umbrella `helm upgrade` is not possible — Helm applies a
chart's resources without cross-sub-chart ordering or readiness gating between
them. Helm Spray adds that ordering, the per-tier readiness wait, and an
individually-addressable release per sub-chart.

## Compatibility

| helm-spray | Helm |
|------------|------|
| v4.x       | Helm v4 (primary) and Helm v3 |
| v3.x       | Helm v2 |

helm-spray v4 links the **Helm v4 SDK** for chart loading and value merging, and
drives whichever `helm` binary invoked it (via `HELM_BIN`). It detects the host
helm version and emits version-appropriate flags, so it works against both Helm
v4 and Helm v3 hosts during the v3 end-of-life transition.

## Install

```console
$ helm plugin install https://github.com/ThalesGroup/helm-spray
$ helm plugin list
NAME    VERSION  DESCRIPTION
spray   4.x      Helm plugin for upgrading sub-charts from umbrella chart with dependency orders
```

`helm plugin install` requires `git`. Helm Spray uses `kubectl` to check
workload readiness — see the [kubectl install guide](https://kubernetes.io/docs/tasks/tools/).

### From source

```console
$ make dist_darwin   # or dist_linux / dist_windows
$ helm plugin install .
```

## Quickstart

```console
# Preview the weight-ordered plan without touching the cluster:
$ helm spray --output json ./my-umbrella-chart

# Deploy the whole solution:
$ helm spray ./my-umbrella-chart

# Re-deploy a single sub-chart:
$ helm spray --target my-service ./my-umbrella-chart

# Explore and visualise a chart in the browser:
$ helm spray ui
```

Helm Spray is always invoked on the **umbrella chart**, whether deploying the
whole solution or an individual sub-chart (with `--target`).

## How it works

Each sub-chart is deployed as its own Helm release named `<chart name or alias>`
(optionally prefixed). The umbrella chart's name and version are recorded as the
chart for every release:

```
NAME             REVISION  UPDATED                   STATUS    CHART         APP VERSION  NAMESPACE
micro-service-1  12        Wed Jan 30 17:19:15 2019  deployed  solution-0.1  0.1          default
micro-service-2  21        Wed Jan 30 17:18:55 2019  deployed  solution-0.1  0.1          default
ms3              7         Wed Jan 30 17:18:45 2019  deployed  solution-0.1  0.1          default
```

Sub-charts that share a weight are deployed together; weight `n+1` starts only
once weight `n` is fully ready. Readiness is gated on the Deployments,
StatefulSets, DaemonSets and Jobs created by the tier (Jobs must reach their
required completions; a failed Job aborts the run).

### The umbrella chart

List the sub-charts in the umbrella's `Chart.yaml` `dependencies`. Each may have
an `alias`, and its `condition` must be `<chart name or alias>.enabled` (Helm
Spray uses the condition internally to deploy one sub-chart at a time):

```yaml
# Chart.yaml
dependencies:
  - name: micro-service-1
    version: ~1.2
    repository: http://chart-museum/charts
    condition: micro-service-1.enabled
  - name: micro-service-2
    version: ~2.3
    repository: http://chart-museum/charts
    condition: micro-service-2.enabled
  - name: micro-service-3
    alias: ms3
    version: ~1.1
    repository: http://chart-museum/charts
    condition: ms3.enabled
```

Set each sub-chart's weight via `<chart name or alias>.weight`, ideally in the
umbrella's default `values.yaml` (weights rarely change):

```yaml
# values.yaml
micro-service-1:
  weight: 0
micro-service-2:
  weight: 1
ms3:
  weight: 2
```

A sub-chart with no `weight` defaults to `0`. If an alias is set, use the
**alias** with `--target`.

### Values

The umbrella gathers many micro-services into one solution, so values can be set
at several levels: in each micro-service's own `values.yaml` (developer
defaults), in the umbrella's `values.yaml` (solution topology), and at deploy
time via `--values/-f`, `--set`, `--set-string`, and `--set-file`
(deployment-specific values such as URLs and credentials).

Because Helm allows only one `values.yaml` in the umbrella, Helm Spray lets you
split solution-level values across several files and include them with a
directive in the umbrella `values.yaml`:

```yaml
micro-service-1:
  weight: 0
#! {{ .Files.Get ms1.yaml }}

micro-service-2:
  weight: 1
#! {{ pick (.Files.Get ms2.yaml) foo | indent 2 }}
```

- `#! {{ .Files.Get <file> }}` includes a whole YAML file.
- `#! {{ pick (.Files.Get <file>) path.to.element }}` includes a sub-element
  (a table or a leaf value; lists are not supported).
- Append `| indent N` to indent the included content by `N` spaces.

The `#!` prefix is required: the `values.yaml` is parsed both with and without
the included content, so the directive must be a comment that keeps the file
valid YAML when the include is not yet applied.

### Tags

Helm Spray honours Helm **tags** declared on dependencies. A tagged sub-chart is
deployed only when one of its tags is enabled; an untagged sub-chart is always
deployed:

```yaml
dependencies:
  - name: micro-service-1
    condition: micro-service-1.enabled
    tags: [common, front-end]
  - name: micro-service-2
    condition: micro-service-2.enabled
    tags: [common, back-end]
```

```console
$ helm spray --set tags.front-end=true ./my-umbrella-chart
```

Tags must be provided via `--values/-f`, `--set`, `--set-string`, or
`--set-file` (values from the cluster, e.g. with `--reuse-values`, are not
considered). A tag value may be a boolean or a string (`"true"`, `"1"`, ...).

> Helm Spray reserves Helm *conditions* for its own use (`<name>.enabled`); other
> conditions are ignored.

## Plan preview

`--output json` resolves the umbrella and prints the weight-ordered deployment
plan — the sub-charts grouped into tiers with their weight, targeting and tag
status — then exits **without contacting the cluster**. This is handy for review
and for gating in CI:

```console
$ helm spray --output json ./my-umbrella-chart
{
  "chart": "./my-umbrella-chart",
  "namespace": "default",
  "tiers": [
    { "weight": 0, "releases": [ { "release": "micro-service-1", "subChart": "micro-service-1", "weight": 0, "targeted": true, "allowedByTags": true } ] },
    { "weight": 1, "releases": [ { "release": "micro-service-2", "subChart": "micro-service-2", "weight": 1, "targeted": true, "allowedByTags": true } ] }
  ]
}
```

## Web UI

`helm spray ui` starts a local web application (served from the single plugin
binary) for configuring a spray and visualising the umbrella chart as a
weight-ordered deployment:

```console
$ helm spray ui --address 127.0.0.1:8080
[spray] helm-spray UI listening on http://127.0.0.1:8080
```

Open the address, enter an umbrella chart and options, and the UI renders the
ordered weight tiers and the per-release targeting/tag status.

## Flags

```
      --create-namespace                 create the namespace if it does not exist
      --debug                            enable helm debug output (implies --verbose)
      --dry-run                          simulate a spray
  -x, --exclude strings                  sub-chart(s) to exclude (repeatable)
      --force                            force resource updates through delete/recreate if needed
  -n, --namespace string                 namespace to spray into (default "default")
  -o, --output string                    print the weight-ordered plan and exit (format: json)
      --prefix-releases string           prefix releases with "<prefix>-" (chars: a-z A-Z 0-9 -)
      --prefix-releases-with-namespace   prefix releases with "<namespace>-"
      --reset-values                     reset values to the chart defaults on upgrade
      --reuse-values                     reuse the last release's values (ignored with --reset-values)
      --set strings                      set values (key1=val1,key2=val2)
      --set-file strings                 set values from files (key=path)
      --set-string strings               set STRING values (key1=val1,key2=val2)
  -t, --target strings                   sub-chart(s) to target (repeatable; default: all)
      --timeout int                      seconds to wait for readiness per tier (default 300)
  -f, --values strings                   values file(s) or URL(s) (repeatable)
  -v, --verbose                          enable verbose output
      --version string                   exact chart version to install (default: latest)
```

## Development

```console
$ go build ./...
$ go test ./...                                   # unit tests
$ go test -tags integration ./pkg/helmspray/...   # live cluster integration test (needs helm + kubectl + a cluster)
$ make dist                                        # cross-platform release archives
```

A complete, runnable reference deployment — a fictional air-traffic-control
platform with nine weight-ordered services — is available as a companion project
and makes a good end-to-end exercise for Helm Spray.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). Please also
read the [Code of Conduct](CODE_OF_CONDUCT.md). To report a security issue, see
[SECURITY.md](SECURITY.md).

## License

Apache-2.0 — see [LICENSE](LICENSE).
