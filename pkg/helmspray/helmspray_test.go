package helmspray

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ThalesGroup/helm-spray/v5/pkg/helm"
)

// fakeHelm records the upgrades it is asked to perform and returns canned
// manifests, so the orchestration can be exercised without a real helm binary.
type fakeHelm struct {
	upgrades   []helm.UpgradeRequest
	manifests  map[string]string
	status     string                  // helm release status to report (defaults to "deployed")
	releases   map[string]helm.Release // releases reported by List (defaults to none)
	uninstalls []string                // release names passed to Uninstall, in order
}

func (f *fakeHelm) List(_ context.Context, _ string, _ bool) (map[string]helm.Release, error) {
	if f.releases != nil {
		return f.releases, nil
	}
	return map[string]helm.Release{}, nil
}

func (f *fakeHelm) Uninstall(_ context.Context, _, releaseName string, _, _ bool) error {
	f.uninstalls = append(f.uninstalls, releaseName)
	return nil
}

func (f *fakeHelm) Upgrade(_ context.Context, req helm.UpgradeRequest) (helm.UpgradedRelease, error) {
	f.upgrades = append(f.upgrades, req)
	status := f.status
	if status == "" {
		status = statusDeployed
	}
	return helm.UpgradedRelease{
		Info:     map[string]any{"status": status},
		Manifest: f.manifests[req.ReleaseName],
	}, nil
}

// fakeReadiness records the deployment readiness queries and always reports ready.
type fakeReadiness struct {
	deploymentQueries [][]string
	notReady          bool  // when true, workloads never become ready
	err               error // when set, readiness checks return this error
}

func (f *fakeReadiness) result() (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return !f.notReady, nil
}

func (f *fakeReadiness) DeploymentsReady(_ context.Context, names []string, _ string, _ bool) (bool, error) {
	f.deploymentQueries = append(f.deploymentQueries, names)
	return f.result()
}
func (f *fakeReadiness) StatefulSetsReady(_ context.Context, _ []string, _ string, _ bool) (bool, error) {
	return f.result()
}
func (f *fakeReadiness) DaemonSetsReady(_ context.Context, _ []string, _ string, _ bool) (bool, error) {
	return f.result()
}
func (f *fakeReadiness) JobsReady(_ context.Context, _ []string, _ string, _ bool) (bool, error) {
	return f.result()
}

// sprayWithAlphaDeployment targets only the weight-0 "alpha" sub-chart and gives
// it a Deployment manifest, so a single tier is upgraded and then gated.
func sprayWithAlphaDeployment(fh *fakeHelm, fr *fakeReadiness, timeout int) *Spray {
	if fh.manifests == nil {
		fh.manifests = map[string]string{}
	}
	fh.manifests["alpha"] = "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: alpha-dep\n"
	return &Spray{
		ChartName:  "testdata/umbrella",
		Namespace:  "ns",
		Targets:    []string{"alpha"},
		Timeout:    timeout,
		helmClient: fh,
		readiness:  fr,
	}
}

func TestSprayReadinessErrorPropagates(t *testing.T) {
	s := sprayWithAlphaDeployment(&fakeHelm{}, &fakeReadiness{err: errors.New("boom")}, 30)
	err := s.Spray(context.Background())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected the readiness error to propagate, got %v", err)
	}
}

func TestSprayReadinessTimeout(t *testing.T) {
	s := sprayWithAlphaDeployment(&fakeHelm{}, &fakeReadiness{notReady: true}, 0)
	err := s.Spray(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
}

func TestSprayContextCancellation(t *testing.T) {
	s := sprayWithAlphaDeployment(&fakeHelm{}, &fakeReadiness{notReady: true}, 600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Spray(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSprayInterruptsOnNonDeployedStatus(t *testing.T) {
	s := sprayWithAlphaDeployment(&fakeHelm{status: "failed"}, &fakeReadiness{}, 30)
	err := s.Spray(context.Background())
	if err == nil || !strings.Contains(err.Error(), "deployed") {
		t.Fatalf("expected the spray to be interrupted on a non-deployed status, got %v", err)
	}
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

func TestCollectWorkloads(t *testing.T) {
	docs := []string{
		// Deployment whose annotation legitimately contains a line that is "---".
		// The document splitter must not break this resource apart (otherwise the
		// Deployment would fail to decode and not be collected).
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: dep1\n  annotations:\n    banner: |\n      line1\n      ---\n      line2",
		"apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: sts1",
		"apiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: ds1",
		"apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: job1",
		// A non-workload object that decodes but is not gated.
		"apiVersion: v1\nkind: Service\nmetadata:\n  name: svc1",
		// A document that does not decode to a Kubernetes object and must be ignored.
		"this: is\nnot: a kubernetes object",
	}
	manifest := strings.Join(docs, "\n---\n")

	var tier workloads
	(&Spray{}).collectWorkloads(&tier, manifest)

	eq := func(name string, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
			return
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s[%d]: got %q, want %q", name, i, got[i], want[i])
			}
		}
	}
	eq("deployments", tier.deployments, "dep1")
	eq("statefulSets", tier.statefulSets, "sts1")
	eq("daemonSets", tier.daemonSets, "ds1")
	eq("jobs", tier.jobs, "job1")
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
