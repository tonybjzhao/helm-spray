package helmspray

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ThalesGroup/helm-spray/v5/internal/dependencies"
)

func TestSortedWeights(t *testing.T) {
	deps := []dependencies.Dependency{
		{Weight: 3}, {Weight: 1}, {Weight: 1}, {Weight: 0}, {Weight: 2}, {Weight: 0},
	}
	if got, want := sortedWeights(deps), []int{0, 1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Errorf("sortedWeights = %v, want %v (deduplicated, ascending)", got, want)
	}
	if got := sortedWeights(nil); len(got) != 0 {
		t.Errorf("sortedWeights(nil) = %v, want empty", got)
	}
}

// On timeout the error must name every still-pending workload kind, and only
// those kinds that actually have workloads in the tier.
func TestSprayTimeoutErrorNamesPendingWorkloads(t *testing.T) {
	fh := &fakeHelm{manifests: map[string]string{
		"alpha": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: alpha-dep\n" +
			"---\napiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: alpha-sts\n",
	}}
	s := &Spray{
		ChartName:  "testdata/umbrella",
		Namespace:  "ns",
		Targets:    []string{"alpha"},
		Timeout:    0, // deadline is immediate
		helmClient: fh,
		readiness:  &fakeReadiness{notReady: true},
	}

	err := s.Spray(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
	for _, want := range []string{"deployments", "alpha-dep", "statefulsets", "alpha-sts"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout error %q should name pending %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "daemonsets") || strings.Contains(err.Error(), "jobs") {
		t.Errorf("timeout error should not name kinds with no pending workloads: %q", err.Error())
	}
}
