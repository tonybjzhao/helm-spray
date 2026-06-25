package helmspray

import (
	"context"
	"strings"
	"testing"

	"github.com/gemalto/helm-spray/v4/pkg/helm"
)

// fakeHelm records the upgrades it is asked to perform and returns canned
// manifests, so the orchestration can be exercised without a real helm binary.
type fakeHelm struct {
	upgrades  []helm.UpgradeRequest
	manifests map[string]string
}

func (f *fakeHelm) List(_ context.Context, _ string, _ bool) (map[string]helm.Release, error) {
	return map[string]helm.Release{}, nil
}

func (f *fakeHelm) Upgrade(_ context.Context, req helm.UpgradeRequest) (helm.UpgradedRelease, error) {
	f.upgrades = append(f.upgrades, req)
	return helm.UpgradedRelease{
		Info:     map[string]interface{}{"status": statusDeployed},
		Manifest: f.manifests[req.ReleaseName],
	}, nil
}

// fakeReadiness records the deployment readiness queries and always reports ready.
type fakeReadiness struct {
	deploymentQueries [][]string
}

func (f *fakeReadiness) DeploymentsReady(_ context.Context, names []string, _ string, _ bool) (bool, error) {
	f.deploymentQueries = append(f.deploymentQueries, names)
	return true, nil
}
func (f *fakeReadiness) StatefulSetsReady(_ context.Context, _ []string, _ string, _ bool) (bool, error) {
	return true, nil
}
func (f *fakeReadiness) DaemonSetsReady(_ context.Context, _ []string, _ string, _ bool) (bool, error) {
	return true, nil
}
func (f *fakeReadiness) JobsReady(_ context.Context, _ []string, _ string, _ bool) (bool, error) {
	return true, nil
}

func TestSprayOrchestration(t *testing.T) {
	fh := &fakeHelm{manifests: map[string]string{
		"alpha": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: alpha-dep\n",
	}}
	fr := &fakeReadiness{}
	s := &Spray{
		ChartName:  "testdata/umbrella",
		Namespace:  "test-ns",
		Timeout:    1,
		helmClient: fh,
		readiness:  fr,
	}

	if err := s.Spray(context.Background()); err != nil {
		t.Fatalf("Spray returned error: %v", err)
	}

	// All three sub-charts are upgraded, alpha (weight 0) before beta/gamma
	// (weight 1).
	if len(fh.upgrades) != 3 {
		t.Fatalf("expected 3 upgrades, got %d: %+v", len(fh.upgrades), fh.upgrades)
	}
	if fh.upgrades[0].ReleaseName != "alpha" {
		t.Errorf("weight 0 chart must be upgraded first; got %q", fh.upgrades[0].ReleaseName)
	}

	// Each upgrade enables only its own sub-chart and disables the others.
	for _, u := range fh.upgrades {
		joined := strings.Join(u.Values, ",")
		if !strings.Contains(joined, u.ReleaseName+".enabled=true") {
			t.Errorf("upgrade %q should enable itself; values=%v", u.ReleaseName, u.Values)
		}
		if !strings.Contains(joined, ".enabled=false") {
			t.Errorf("upgrade %q should disable the other sub-charts; values=%v", u.ReleaseName, u.Values)
		}
		if u.Namespace != "test-ns" {
			t.Errorf("upgrade %q namespace = %q, want test-ns", u.ReleaseName, u.Namespace)
		}
	}

	// The Deployment rendered for alpha must have been gated for readiness.
	gated := false
	for _, q := range fr.deploymentQueries {
		for _, n := range q {
			if n == "alpha-dep" {
				gated = true
			}
		}
	}
	if !gated {
		t.Errorf("expected readiness gating for alpha-dep; queries=%v", fr.deploymentQueries)
	}
}

func TestSprayInvalidTargetFailsFast(t *testing.T) {
	s := &Spray{
		ChartName:  "testdata/umbrella",
		Namespace:  "test-ns",
		Timeout:    1,
		Targets:    []string{"does-not-exist"},
		helmClient: &fakeHelm{manifests: map[string]string{}},
		readiness:  &fakeReadiness{},
	}
	err := s.Spray(context.Background())
	if err == nil {
		t.Fatal("expected an error for an invalid --target")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should name the invalid target; got %v", err)
	}
}

func TestPlanGroupsByWeight(t *testing.T) {
	s := &Spray{ChartName: "testdata/umbrella", Namespace: "ns"}
	plan, err := s.Plan()
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(plan.Tiers) != 2 {
		t.Fatalf("expected 2 weight tiers, got %d: %+v", len(plan.Tiers), plan.Tiers)
	}
	if plan.Tiers[0].Weight != 0 || plan.Tiers[1].Weight != 1 {
		t.Errorf("tiers not in ascending weight order: %d then %d", plan.Tiers[0].Weight, plan.Tiers[1].Weight)
	}
	if len(plan.Tiers[0].Releases) != 1 || plan.Tiers[0].Releases[0].SubChart != "alpha" {
		t.Errorf("weight-0 tier should contain only alpha, got %+v", plan.Tiers[0].Releases)
	}
	if len(plan.Tiers[1].Releases) != 2 {
		t.Errorf("weight-1 tier should contain beta and gamma, got %+v", plan.Tiers[1].Releases)
	}
}
