package helmspray

import (
	"context"
	"fmt"
	"sort"

	"github.com/ThalesGroup/helm-spray/v5/internal/dependencies"
)

// PlanRelease describes a single release in the deployment plan.
type PlanRelease struct {
	Release       string `json:"release"`
	SubChart      string `json:"subChart"`
	Alias         string `json:"alias,omitempty"`
	Weight        int    `json:"weight"`
	Targeted      bool   `json:"targeted"`
	AllowedByTags bool   `json:"allowedByTags"`
	AppVersion    string `json:"appVersion,omitempty"`
}

// PlanTier groups the releases that share a weight and are deployed together,
// after the previous tier has become ready.
type PlanTier struct {
	Weight   int           `json:"weight"`
	Releases []PlanRelease `json:"releases"`
}

// Plan is the ordered, cluster-independent deployment plan for an umbrella
// chart: the sub-charts grouped into weight tiers in the order they would be
// processed. It is consumed by the "--output json" preview and the GUI.
type Plan struct {
	Chart     string     `json:"chart"`
	Namespace string     `json:"namespace"`
	Tiers     []PlanTier `json:"tiers"`
}

// Plan resolves the umbrella chart and returns the ordered deployment plan
// without contacting the cluster.
func (s *Spray) Plan() (*Plan, error) {
	_, deps, _, _, err := s.resolve()
	if err != nil {
		return nil, err
	}

	byWeight := make(map[int][]PlanRelease)
	for _, d := range deps {
		byWeight[d.Weight] = append(byWeight[d.Weight], planRelease(d))
	}
	weights := make([]int, 0, len(byWeight))
	for w := range byWeight {
		weights = append(weights, w)
	}
	sort.Ints(weights)

	plan := &Plan{Chart: s.ChartName, Namespace: s.Namespace}
	for _, w := range weights {
		plan.Tiers = append(plan.Tiers, PlanTier{Weight: w, Releases: byWeight[w]})
	}
	return plan, nil
}

// ReleaseStatus is the live state of a single release, as surfaced read-only to
// the web UI so it can colour the plan while a deployment is in progress.
type ReleaseStatus struct {
	Status   string `json:"status"`
	Revision string `json:"revision"`
}

// LiveStatus returns the deployment plan augmented with the live helm status of
// each release in the namespace. It performs a read-only "helm list" and never
// mutates the cluster, so it is safe for the web UI to poll.
func (s *Spray) LiveStatus(ctx context.Context) (*Plan, map[string]ReleaseStatus, error) {
	if s.helmClient == nil {
		s.helmClient = execHelmClient{}
	}
	plan, err := s.Plan()
	if err != nil {
		return nil, nil, err
	}
	releases, err := s.helmClient.List(ctx, s.Namespace, s.Debug)
	if err != nil {
		return nil, nil, fmt.Errorf("listing releases: %w", err)
	}
	status := make(map[string]ReleaseStatus, len(releases))
	for name, r := range releases {
		status[name] = ReleaseStatus{Status: r.Status, Revision: r.Revision}
	}
	return plan, status, nil
}

func planRelease(d dependencies.Dependency) PlanRelease {
	return PlanRelease{
		Release:       d.CorrespondingReleaseName,
		SubChart:      d.Name,
		Alias:         d.Alias,
		Weight:        d.Weight,
		Targeted:      d.Targeted,
		AllowedByTags: d.AllowedByTags,
		AppVersion:    d.AppVersion,
	}
}
