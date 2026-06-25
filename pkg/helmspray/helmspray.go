// Package helmspray orchestrates the deployment of an umbrella chart's
// sub-charts in ascending weight order, creating one Helm release per sub-chart
// and gating workload readiness between tiers.
package helmspray

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ThalesGroup/helm-spray/v5/internal/dependencies"
	"github.com/ThalesGroup/helm-spray/v5/internal/log"
	"github.com/ThalesGroup/helm-spray/v5/internal/values"
	"github.com/ThalesGroup/helm-spray/v5/pkg/helm"
	"github.com/ThalesGroup/helm-spray/v5/pkg/util"
	loader "helm.sh/helm/v4/pkg/chart/v2/loader"
	cliValues "helm.sh/helm/v4/pkg/cli/values"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	// statusDeployed is the helm release status indicating a successful upgrade.
	statusDeployed = "deployed"
	// minPollInterval and maxPollInterval bound the readiness back-off polling.
	minPollInterval = 1 * time.Second
	maxPollInterval = 5 * time.Second
)

// documentSeparator matches a YAML document boundary: a line consisting solely
// of "---". Splitting on the bare substring "---" would corrupt resources whose
// content legitimately contains that sequence.
var documentSeparator = regexp.MustCompile("(?m)^---$")

// Spray holds the configuration of a spray run and the clients used to perform
// it. The helm and readiness clients default to CLI-backed implementations and
// can be overridden (e.g. in tests).
type Spray struct {
	ChartName                   string
	ChartVersion                string
	Targets                     []string
	Excludes                    []string
	Namespace                   string
	CreateNamespace             bool
	PrefixReleases              string
	PrefixReleasesWithNamespace bool
	ResetValues                 bool
	ReuseValues                 bool
	ValuesOpts                  cliValues.Options
	Force                       bool
	Prune                       bool
	Timeout                     int
	DryRun                      bool
	Verbose                     bool
	Debug                       bool

	helmClient HelmClient
	readiness  ReadinessChecker
}

// workloads collects the names of the workloads created by the releases of a
// single weight tier, grouped by kind, so readiness can be gated per tier.
type workloads struct {
	deployments  []string
	statefulSets []string
	daemonSets   []string
	jobs         []string
}

// Spray installs or upgrades the sub-charts of the umbrella chart in ascending
// weight order, one release per sub-chart, waiting for each tier to become ready
// before processing the next.
func (s *Spray) Spray(ctx context.Context) error {
	if s.helmClient == nil {
		s.helmClient = execHelmClient{}
	}
	if s.readiness == nil {
		s.readiness = execReadinessChecker{}
	}

	if s.Debug {
		log.Info(1, "starting spray with flags: %+v", s)
	}
	startTime := time.Now()

	updatedChartValuesAsString, deps, releasePrefix, umbrellaName, err := s.resolve()
	if err != nil {
		return err
	}
	if len(updatedChartValuesAsString) > 0 {
		// Write the processed default values to a temporary file and prepend it
		// to the value files passed to helm.
		tempDir, err := os.MkdirTemp("", "spray-")
		if err != nil {
			return fmt.Errorf("creating temporary directory to write updated default values file for umbrella chart: %w", err)
		}
		defer removeTempDir(tempDir)
		tempFile, err := os.CreateTemp(tempDir, "updatedDefaultValues-*.yaml")
		if err != nil {
			return fmt.Errorf("creating temporary file to write updated default values file for umbrella chart: %w", err)
		}
		defer removeTempFile(tempFile.Name())
		if _, err = tempFile.Write([]byte(updatedChartValuesAsString)); err != nil {
			return fmt.Errorf("writing updated default values file for umbrella chart into temporary file: %w", err)
		}
		if err = tempFile.Close(); err != nil {
			return fmt.Errorf("closing temporary file to write updated default values file for umbrella chart: %w", err)
		}
		s.ValuesOpts.ValueFiles = append([]string{tempFile.Name()}, s.ValuesOpts.ValueFiles...)
	}

	if len(releasePrefix) > 0 {
		log.Info(1, "deploying solution chart \"%s\" in namespace \"%s\", with release prefix \"%s\"", s.ChartName, s.Namespace, releasePrefix)
	} else {
		log.Info(1, "deploying solution chart \"%s\" in namespace \"%s\"", s.ChartName, s.Namespace)
	}

	releases, err := s.helmClient.List(ctx, s.Namespace, s.Debug)
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}
	if s.Verbose {
		logRelease(releases, deps)
	}

	// Pre-compute the "<name>.enabled=false" set once. Each upgrade re-enables
	// only its own sub-chart by appending a later (higher-priority) --set, so the
	// per-release set construction is O(1) rather than O(n) per release.
	disabled := make([]string, 0, len(deps))
	for _, d := range deps {
		disabled = append(disabled, d.UsedName+".enabled=false")
	}
	disableAllSet := strings.Join(disabled, ",")

	// Process sub-charts by ascending weight; gate each tier before the next.
	// Only the weights that actually occur are visited, computed once up front.
	for _, weight := range sortedWeights(deps) {
		tier, shouldWait, err := s.upgrade(ctx, releases, deps, weight, disableAllSet)
		if err != nil {
			return err
		}
		if shouldWait && !s.DryRun {
			if err = s.wait(ctx, tier); err != nil {
				return err
			}
		}
	}

	if s.Prune {
		if err := s.prune(ctx, deps, umbrellaName, releasePrefix); err != nil {
			return err
		}
	}

	log.Info(1, "upgrade of solution chart \"%s\" completed in %s", s.ChartName, util.Duration(time.Since(startTime)))
	return nil
}

// resolve loads the umbrella chart, merges its values, computes the
// per-sub-chart dependency metadata (weights, targeting, tags), and validates
// the targets/excludes. It performs no cluster operations and is shared by
// Spray and Plan. It returns the processed default-values document (when the
// "#! .Files.Get" includes produced one), the dependencies, and the release
// name prefix.
func (s *Spray) resolve() (updatedChartValues string, deps []dependencies.Dependency, releasePrefix string, umbrellaName string, err error) {
	chart, err := loader.Load(s.ChartName)
	if err != nil {
		return "", nil, "", "", fmt.Errorf("loading chart \"%s\": %w", s.ChartName, err)
	}
	umbrellaName = chart.Name()
	mergedValues, updatedChartValues, err := values.Merge(chart, s.ReuseValues, &s.ValuesOpts, s.Verbose)
	if err != nil {
		return "", nil, "", "", fmt.Errorf("merging values: %w", err)
	}
	if s.PrefixReleasesWithNamespace && len(s.Namespace) > 0 {
		releasePrefix = s.Namespace + "-"
	} else if len(s.PrefixReleases) > 0 {
		releasePrefix = s.PrefixReleases + "-"
	}
	deps, err = dependencies.Get(chart, &mergedValues, s.Targets, s.Excludes, releasePrefix, s.Verbose)
	if err != nil {
		return "", nil, "", "", fmt.Errorf("analyzing dependencies: %w", err)
	}
	if err = checkTargetsAndExcludes(deps, s.Targets, s.Excludes); err != nil {
		return "", nil, "", "", fmt.Errorf("checking targets and excludes: %w", err)
	}
	return updatedChartValues, deps, releasePrefix, umbrellaName, nil
}

// upgrade installs or upgrades every targeted, tag-allowed sub-chart at the given
// weight and returns the workloads they created (for readiness gating) and
// whether anything was upgraded.
func (s *Spray) upgrade(ctx context.Context, releases map[string]helm.Release, deps []dependencies.Dependency, currentWeight int, disableAllSet string) (workloads, bool, error) {
	var tier workloads
	shouldWait := false
	firstInWeight := true

	for _, dependency := range deps {
		if !dependency.Targeted || !dependency.AllowedByTags || dependency.Weight != currentWeight {
			continue
		}
		if firstInWeight {
			log.Info(1, "processing sub-charts of weight %d", dependency.Weight)
			firstInWeight = false
		}

		if release, ok := releases[dependency.CorrespondingReleaseName]; ok {
			oldRevision, _ := strconv.Atoi(release.Revision)
			log.Info(2, "upgrading release \"%s\": going from revision %d (status %s) to %d (appVersion %s)...", dependency.CorrespondingReleaseName, oldRevision, release.Status, oldRevision+1, dependency.AppVersion)
		} else {
			log.Info(2, "upgrading release \"%s\": deploying first revision (appVersion %s)...", dependency.CorrespondingReleaseName, dependency.AppVersion)
		}

		shouldWait = true

		// Enable only the current sub-chart: all charts are disabled, then this
		// one is re-enabled by a later (higher-priority) --set.
		valuesSet := make([]string, 0, len(s.ValuesOpts.Values)+2)
		valuesSet = append(valuesSet, s.ValuesOpts.Values...)
		if disableAllSet != "" {
			valuesSet = append(valuesSet, disableAllSet)
		}
		valuesSet = append(valuesSet, dependency.UsedName+".enabled=true")

		upgradedRelease, err := s.helmClient.Upgrade(ctx, helm.UpgradeRequest{
			Namespace:       s.Namespace,
			CreateNamespace: s.CreateNamespace,
			ReleaseName:     dependency.CorrespondingReleaseName,
			ChartPath:       s.ChartName,
			ResetValues:     s.ResetValues,
			ReuseValues:     s.ReuseValues,
			ValueFiles:      s.ValuesOpts.ValueFiles,
			Values:          valuesSet,
			StringValues:    s.ValuesOpts.StringValues,
			FileValues:      s.ValuesOpts.FileValues,
			Force:           s.Force,
			Timeout:         s.Timeout,
			DryRun:          s.DryRun,
			Debug:           s.Debug,
		})
		if err != nil {
			return workloads{}, false, fmt.Errorf("calling helm upgrade: %w", err)
		}

		log.Info(3, "release: \"%s\" upgraded", dependency.CorrespondingReleaseName)
		if s.Verbose {
			log.Info(3, "helm status: %s", upgradedRelease.Info["status"])
		}
		if !s.DryRun && upgradedRelease.Info["status"] != statusDeployed {
			return workloads{}, false, errors.New("status returned by helm differs from \"deployed\", spray interrupted")
		}

		s.collectWorkloads(&tier, upgradedRelease.Manifest)
	}

	return tier, shouldWait, nil
}

// collectWorkloads decodes the rendered manifest and records the names of the
// workloads whose readiness gates tier progression.
func (s *Spray) collectWorkloads(tier *workloads, manifest string) {
	var ignoredParts []string
	for _, doc := range documentSeparator.Split(manifest, -1) {
		obj, _, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(doc), nil, nil)
		if err != nil {
			if s.Verbose && len(strings.TrimSpace(doc)) > 0 {
				ignoredParts = append(ignoredParts, doc)
			}
			continue
		}
		switch o := obj.(type) {
		case *appsv1.Deployment:
			tier.deployments = append(tier.deployments, o.Name)
		case *appsv1.StatefulSet:
			tier.statefulSets = append(tier.statefulSets, o.Name)
		case *appsv1.DaemonSet:
			tier.daemonSets = append(tier.daemonSets, o.Name)
		case *batchv1.Job:
			tier.jobs = append(tier.jobs, o.Name)
		}
	}
	if s.Verbose && len(ignoredParts) > 0 {
		log.Info(3, "warning: ignored part(s) of helm upgrade output")
		if s.Debug {
			log.Info(3, "warning: ignored '%v'", ignoredParts)
		}
	}
}

// wait blocks until every workload in the tier is ready, the timeout elapses, or
// the context is cancelled. It polls with capped exponential back-off.
func (s *Spray) wait(ctx context.Context, tier workloads) error {
	log.Info(2, "waiting for liveness and readiness...")

	type check struct {
		kind  string
		names []string
		fn    func(context.Context, []string, string, bool) (bool, error)
	}
	checks := []check{
		{"deployments", tier.deployments, s.readiness.DeploymentsReady},
		{"statefulsets", tier.statefulSets, s.readiness.StatefulSetsReady},
		{"daemonsets", tier.daemonSets, s.readiness.DaemonSetsReady},
		{"jobs", tier.jobs, s.readiness.JobsReady},
	}
	done := make([]bool, len(checks))

	deadline := time.Now().Add(time.Duration(s.Timeout) * time.Second)
	interval := minPollInterval
	for {
		for i := range checks {
			if done[i] || len(checks[i].names) == 0 {
				done[i] = true
				continue
			}
			if s.Verbose {
				log.Info(3, "waiting for %s %v", checks[i].kind, checks[i].names)
			}
			ready, err := checks[i].fn(ctx, checks[i].names, s.Namespace, s.Debug)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("cannot check readiness of %s %v: %w", checks[i].kind, checks[i].names, err)
			}
			done[i] = ready
		}
		if allTrue(done) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for liveness and readiness")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		if interval < maxPollInterval {
			if interval *= 2; interval > maxPollInterval {
				interval = maxPollInterval
			}
		}
	}
}

func allTrue(b []bool) bool {
	for _, v := range b {
		if !v {
			return false
		}
	}
	return true
}

// sortedWeights returns the distinct sub-chart weights in ascending order, so the
// orchestrator visits exactly the tiers that exist (no scan over empty weights)
// and computes the set once rather than recomputing the maximum every iteration.
func sortedWeights(deps []dependencies.Dependency) []int {
	seen := make(map[int]bool, len(deps))
	weights := make([]int, 0, len(deps))
	for _, d := range deps {
		if !seen[d.Weight] {
			seen[d.Weight] = true
			weights = append(weights, d.Weight)
		}
	}
	sort.Ints(weights)
	return weights
}

func checkTargetsAndExcludes(deps []dependencies.Dependency, targets []string, excludes []string) error {
	// Check that the provided target(s) or exclude(s) correspond to valid
	// sub-chart names or aliases.
	known := make(map[string]bool, len(deps))
	for _, d := range deps {
		known[d.UsedName] = true
	}
	for _, t := range targets {
		if !known[t] {
			return fmt.Errorf("invalid targeted sub-chart name/alias \"%s\"", t)
		}
	}
	for _, x := range excludes {
		if !known[x] {
			return fmt.Errorf("invalid excluded sub-chart name/alias \"%s\"", x)
		}
	}
	return nil
}

func logRelease(releases map[string]helm.Release, deps []dependencies.Dependency) {
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 1, ' ', tabwriter.Debug)
	_, _ = fmt.Fprintln(w, "[spray]  \t subchart\t is alias of\t targeted\t weight\t| corresponding release\t revision\t status\t")
	_, _ = fmt.Fprintln(w, "[spray]  \t --------\t -----------\t --------\t ------\t| ---------------------\t --------\t ------\t")

	for _, dependency := range deps {
		currentRevision := "None"
		currentStatus := "Not deployed"
		if release, ok := releases[dependency.CorrespondingReleaseName]; ok {
			currentRevision = release.Revision
			currentStatus = release.Status
		}

		name := dependency.Name
		alias := "-"
		if dependency.Alias != "" {
			name = dependency.Alias
			alias = dependency.Name
		}

		targeted := fmt.Sprint(dependency.Targeted)
		if dependency.Targeted && dependency.HasTags && dependency.AllowedByTags {
			targeted = "true (tag match)"
		} else if dependency.Targeted && dependency.HasTags && !dependency.AllowedByTags {
			targeted = "false (no tag match)"
		}

		_, _ = fmt.Fprintf(w, "[spray]  \t %s\t %s\t %s\t %d\t| %s\t %s\t %s\t\n", name, alias, targeted, dependency.Weight, dependency.CorrespondingReleaseName, currentRevision, currentStatus)
	}
	_ = w.Flush()
}

func removeTempDir(tempDir string) {
	if err := os.RemoveAll(tempDir); err != nil {
		log.Error("Error: removing temporary directory: %s", err)
	}
}

func removeTempFile(tempFile string) {
	if err := os.Remove(tempFile); err != nil {
		log.Error("Error: removing temporary file: %s", err)
	}
}
