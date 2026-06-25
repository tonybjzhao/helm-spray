package helmspray

import (
	"context"
	"reflect"
	"testing"

	"github.com/ThalesGroup/helm-spray/v4/pkg/helm"
)

// umbrellaRelease builds a helm.Release as it would appear after a spray: its
// chart is the umbrella chart, so prune recognises it as spray-owned.
func umbrellaRelease(name string) helm.Release {
	return helm.Release{Name: name, Chart: "umbrella-0.1.0", Status: "deployed", Revision: "1"}
}

const alphaDeploymentManifest = "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: alpha-dep\n"

func TestUninstallReverseWeightOrder(t *testing.T) {
	fh := &fakeHelm{releases: map[string]helm.Release{
		"alpha": umbrellaRelease("alpha"),
		"beta":  umbrellaRelease("beta"),
		"gamma": umbrellaRelease("gamma"),
	}}
	s := &Spray{ChartName: "testdata/umbrella", Namespace: "ns", helmClient: fh}

	if err := s.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// Weight 1 (beta, gamma) is torn down before weight 0 (alpha).
	want := []string{"beta", "gamma", "alpha"}
	if !reflect.DeepEqual(fh.uninstalls, want) {
		t.Errorf("uninstall order = %v, want %v", fh.uninstalls, want)
	}
}

func TestUninstallSkipsAbsentReleases(t *testing.T) {
	fh := &fakeHelm{releases: map[string]helm.Release{"alpha": umbrellaRelease("alpha")}}
	s := &Spray{ChartName: "testdata/umbrella", Namespace: "ns", helmClient: fh}

	if err := s.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !reflect.DeepEqual(fh.uninstalls, []string{"alpha"}) {
		t.Errorf("only the deployed release should be uninstalled, got %v", fh.uninstalls)
	}
}

func TestUninstallTargeted(t *testing.T) {
	fh := &fakeHelm{releases: map[string]helm.Release{
		"alpha": umbrellaRelease("alpha"),
		"beta":  umbrellaRelease("beta"),
		"gamma": umbrellaRelease("gamma"),
	}}
	s := &Spray{ChartName: "testdata/umbrella", Namespace: "ns", Targets: []string{"beta"}, helmClient: fh}

	if err := s.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !reflect.DeepEqual(fh.uninstalls, []string{"beta"}) {
		t.Errorf("only the targeted release should be uninstalled, got %v", fh.uninstalls)
	}
}

func TestSprayHooksOnlySubchartSucceeds(t *testing.T) {
	// A sub-chart whose only resources are helm hooks renders an empty .manifest
	// (helm reports hooks separately), so the tier has no workloads to gate on.
	// The spray must still complete (issue #13). The readiness checker is set to
	// never-ready to prove it is not consulted for an empty tier.
	fh := &fakeHelm{manifests: map[string]string{"alpha": ""}}
	s := &Spray{
		ChartName:  "testdata/umbrella",
		Namespace:  "ns",
		Targets:    []string{"alpha"},
		Timeout:    30,
		helmClient: fh,
		readiness:  &fakeReadiness{notReady: true},
	}

	if err := s.Spray(context.Background()); err != nil {
		t.Fatalf("spray of a hooks-only sub-chart should succeed, got %v", err)
	}
}

func TestSprayPruneRemovesOrphans(t *testing.T) {
	fh := &fakeHelm{
		releases: map[string]helm.Release{
			"alpha": umbrellaRelease("alpha"),
			"delta": umbrellaRelease("delta"), // a sub-chart removed from the umbrella
		},
		manifests: map[string]string{"alpha": alphaDeploymentManifest},
	}
	s := &Spray{
		ChartName:  "testdata/umbrella",
		Namespace:  "ns",
		Targets:    []string{"alpha"},
		Prune:      true,
		Timeout:    30,
		helmClient: fh,
		readiness:  &fakeReadiness{},
	}

	if err := s.Spray(context.Background()); err != nil {
		t.Fatalf("Spray: %v", err)
	}
	if !reflect.DeepEqual(fh.uninstalls, []string{"delta"}) {
		t.Errorf("prune should remove only the orphan, got %v", fh.uninstalls)
	}
}

func TestSprayPruneIgnoresForeignReleases(t *testing.T) {
	fh := &fakeHelm{
		releases: map[string]helm.Release{
			"alpha":   umbrellaRelease("alpha"),
			"foreign": {Name: "foreign", Chart: "other-1.0.0", Status: "deployed"},
		},
		manifests: map[string]string{"alpha": alphaDeploymentManifest},
	}
	s := &Spray{
		ChartName:  "testdata/umbrella",
		Namespace:  "ns",
		Targets:    []string{"alpha"},
		Prune:      true,
		Timeout:    30,
		helmClient: fh,
		readiness:  &fakeReadiness{},
	}

	if err := s.Spray(context.Background()); err != nil {
		t.Fatalf("Spray: %v", err)
	}
	if len(fh.uninstalls) != 0 {
		t.Errorf("prune must not remove releases from other charts, got %v", fh.uninstalls)
	}
}

func TestSprayPrunePrefixIsolation(t *testing.T) {
	fh := &fakeHelm{
		releases: map[string]helm.Release{
			"env1-alpha": umbrellaRelease("env1-alpha"),
			"env1-delta": umbrellaRelease("env1-delta"), // orphan in our environment
			"env2-delta": umbrellaRelease("env2-delta"), // orphan owned by another environment
		},
		manifests: map[string]string{"env1-alpha": alphaDeploymentManifest},
	}
	s := &Spray{
		ChartName:      "testdata/umbrella",
		Namespace:      "ns",
		Targets:        []string{"alpha"},
		PrefixReleases: "env1",
		Prune:          true,
		Timeout:        30,
		helmClient:     fh,
		readiness:      &fakeReadiness{},
	}

	if err := s.Spray(context.Background()); err != nil {
		t.Fatalf("Spray: %v", err)
	}
	if !reflect.DeepEqual(fh.uninstalls, []string{"env1-delta"}) {
		t.Errorf("prune should only remove orphans bearing our prefix, got %v", fh.uninstalls)
	}
}
