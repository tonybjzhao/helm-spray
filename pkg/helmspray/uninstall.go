package helmspray

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ThalesGroup/helm-spray/v4/internal/dependencies"
	"github.com/ThalesGroup/helm-spray/v4/internal/log"
)

// Uninstall removes the releases helm-spray created for the umbrella chart's
// sub-charts. Releases are torn down in descending weight order — the reverse of
// deployment — so that higher tiers (which may depend on lower ones) are removed
// first. The --target/--exclude selection narrows the set. Releases that are not
// currently deployed are skipped, making uninstall idempotent. With DryRun set,
// helm reports what it would remove without deleting anything.
func (s *Spray) Uninstall(ctx context.Context) error {
	if s.helmClient == nil {
		s.helmClient = execHelmClient{}
	}

	_, deps, _, _, err := s.resolve()
	if err != nil {
		return err
	}

	releases, err := s.helmClient.List(ctx, s.Namespace, s.Debug)
	if err != nil {
		return fmt.Errorf("listing releases: %w", err)
	}

	// Tear down in descending weight order.
	order := make([]dependencies.Dependency, 0, len(deps))
	for _, d := range deps {
		if d.Targeted && d.AllowedByTags {
			order = append(order, d)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].Weight > order[j].Weight })

	removed := 0
	for _, d := range order {
		if _, ok := releases[d.CorrespondingReleaseName]; !ok {
			continue
		}
		log.Info(1, "uninstalling release \"%s\" (weight %d)...", d.CorrespondingReleaseName, d.Weight)
		if err := s.helmClient.Uninstall(ctx, s.Namespace, d.CorrespondingReleaseName, s.DryRun, s.Debug); err != nil {
			return fmt.Errorf("uninstalling release %q: %w", d.CorrespondingReleaseName, err)
		}
		removed++
	}

	if removed == 0 {
		log.Info(1, "no matching releases to uninstall for chart \"%s\" in namespace \"%s\"", s.ChartName, s.Namespace)
	} else {
		log.Info(1, "uninstalled %d release(s) for chart \"%s\"", removed, s.ChartName)
	}
	return nil
}

// prune removes releases that this umbrella previously created but that are no
// longer part of it — for example a sub-chart deleted from the umbrella since the
// last spray. A release is considered spray-owned when its chart matches the
// umbrella chart; a spray-owned release whose name is absent from the current
// dependency set is an orphan and is uninstalled. When a release prefix is in
// effect, only releases bearing that prefix are eligible, so independent
// solutions sharing a namespace do not interfere with one another.
func (s *Spray) prune(ctx context.Context, deps []dependencies.Dependency, umbrellaName, releasePrefix string) error {
	releases, err := s.helmClient.List(ctx, s.Namespace, s.Debug)
	if err != nil {
		return fmt.Errorf("listing releases for prune: %w", err)
	}

	desired := make(map[string]bool, len(deps))
	for _, d := range deps {
		desired[d.CorrespondingReleaseName] = true
	}

	chartPrefix := umbrellaName + "-"
	orphans := make([]string, 0)
	for name, r := range releases {
		if desired[name] {
			continue
		}
		// Only consider releases produced from this umbrella chart.
		if !strings.HasPrefix(r.Chart, chartPrefix) {
			continue
		}
		// Respect the release-name prefix so unrelated solutions are left alone.
		if releasePrefix != "" && !strings.HasPrefix(name, releasePrefix) {
			continue
		}
		orphans = append(orphans, name)
	}
	sort.Strings(orphans)

	for _, name := range orphans {
		log.Info(1, "pruning orphaned release \"%s\" (no longer part of chart \"%s\")...", name, s.ChartName)
		if err := s.helmClient.Uninstall(ctx, s.Namespace, name, s.DryRun, s.Debug); err != nil {
			return fmt.Errorf("pruning release %q: %w", name, err)
		}
	}
	if len(orphans) > 0 {
		log.Info(1, "pruned %d orphaned release(s)", len(orphans))
	}
	return nil
}
