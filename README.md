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
| v5.x       | Helm v4 (primary) and Helm v3 |
| v4.x       | Helm v3 |
| v3.x       | Helm v2 |

helm-spray v5 links the **Helm v4 SDK** for chart loading and value merging, and
drives whichever `helm` binary invoked it (via `HELM_BIN`). It detects the host
helm version and emits version-appropriate flags, so it works against both Helm
v4 and Helm v3 hosts during the v3 end-of-life transition.

## Install

```console
$ helm plugin install https://github.com/ThalesGroup/helm-spray
$ helm plugin list
NAME    VERSION  DESCRIPTION
spray   5.x      Helm plugin for upgrading sub-charts from umbrella chart with dependency orders
```

`helm plugin install` requires `git`. Helm Spray checks workload readiness by
talking to the Kubernetes API directly with your existing kubeconfig, so **no
`kubectl` binary is required** — `helm` is the only external tool it needs.

### From source

Build a binary for your platform and run it directly:

```console
$ make build                       # builds ./bin/helm-spray for the host
$ ./bin/helm-spray --output json ./my-umbrella-chart
```

`make dist` cross-compiles release archives into `_dist/`. Note that
`helm plugin install <url>` runs an install hook that downloads the released
binary matching `plugin.yaml`'s version — it does not build from source.

## Quickstart

```console
# Preview the weight-ordered plan without touching the cluster:
$ helm spray --output json ./my-umbrella-chart

# Deploy the whole solution:
$ helm spray ./my-umbrella-chart

# Re-deploy a single sub-chart:
$ helm spray --target my-service ./my-umbrella-chart

# Remove every release the solution created (reverse weight order):
$ helm spray uninstall ./my-umbrella-chart

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

Helm Spray honours Helm **tags** declared on dependencies, with the same default
as Helm itself: **a tag is enabled unless you explicitly disable it**. A tagged
sub-chart is therefore deployed unless *every* one of its tags is set to `false`;
an untagged sub-chart is always deployed:

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
# Deploy everything (tags default to enabled):
$ helm spray ./my-umbrella-chart

# Skip the front-end group:
$ helm spray --set tags.front-end=false ./my-umbrella-chart
```

Tag overrides are read from `--values/-f`, `--set`, `--set-string`, or
`--set-file` (values from the cluster, e.g. with `--reuse-values`, are not
considered). A tag value may be a boolean or a string (`"false"`, `"0"`, ...).

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
ordered weight tiers and the per-release targeting/tag status. The header shows
the helm-spray version and the **helm host version** it would drive, and a
light/dark toggle is available.

Click **Watch live status** to poll the cluster (read-only) and watch the plan
colour in as each release reports its helm status — grey for not-yet-deployed,
amber while pending, green when deployed, red on failure. The UI never mutates
the cluster: deploying, uninstalling, and pruning are performed by the
`helm spray` CLI, which keeps a single, well-tested execution path and avoids
exposing a cluster-mutating endpoint on an unauthenticated local server.

## Uninstall and prune

`helm spray uninstall` removes the releases a spray created, in **descending
weight order** (the reverse of deployment), so higher tiers are torn down before
the lower tiers they depend on:

```console
# Remove the whole solution:
$ helm spray uninstall ./my-umbrella-chart

# Remove a single sub-chart's release:
$ helm spray uninstall --target my-service ./my-umbrella-chart

# Preview what would be removed:
$ helm spray uninstall --dry-run ./my-umbrella-chart
```

Pass the same umbrella chart and `--prefix-releases`/
`--prefix-releases-with-namespace` you deployed with, so helm-spray can compute
the release names. Releases that are not currently deployed are skipped, so
uninstall is idempotent.

When a sub-chart is **removed** from an umbrella, its old release lingers in the
cluster. Add `--prune` to a deploy to reconcile that: after the spray completes,
helm-spray uninstalls any release that was created from this umbrella chart but
is no longer one of its sub-charts.

```console
$ helm spray --prune ./my-umbrella-chart
```

Prune only ever touches releases produced from the same umbrella chart, and
honours `--prefix-releases`, so independent solutions sharing a namespace never
interfere with one another.

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
      --prune                            after deploying, uninstall releases for sub-charts no longer in the umbrella
      --reset-values                     reset values to the chart defaults on upgrade
      --reuse-values                     reuse the last release's values (cannot be combined with --reset-values)
      --set strings                      set values (key1=val1,key2=val2)
      --set-file strings                 set values from files (key=path)
      --set-string strings               set STRING values (key1=val1,key2=val2)
  -t, --target strings                   sub-chart(s) to target (repeatable; default: all)
      --timeout string                   wait per tier, as seconds or a duration: 300 or 5m (default "300")
  -f, --values strings                   values file(s) or URL(s) (repeatable)
  -v, --verbose                          enable verbose output
      --version string                   exact chart version to install (default: latest)
```

## Development

```console
$ go build ./...
$ go test ./...                                   # unit tests
$ go test -tags integration ./pkg/helmspray/...   # live cluster integration test (needs helm + a cluster)
$ make dist                                        # cross-platform release archives
```

A good end-to-end exercise is to build a small umbrella chart of weighted
micro-services — for example a layered system with a datastore and message bus at
weight 0, data services at weight 1, domain logic at weight 2, and an
operator-facing edge at weight 3 — and deploy it with `helm spray`. The companion
[Skyward ATC](https://github.com/samlister-thales/SkywardATC) project is a
ready-made example of exactly this.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). Please also
read the [Code of Conduct](CODE_OF_CONDUCT.md). To report a security issue, see
[SECURITY.md](SECURITY.md).

## License

Apache-2.0 — see [LICENSE](LICENSE).
