//go:build integration

// Package helmspray integration test. It performs a real weight-ordered rollout
// against a live Kubernetes cluster using the actual helm and kubectl binaries,
// so it is gated behind the "integration" build tag and excluded from unit runs.
//
//	go test -tags integration ./pkg/helmspray/ -run TestSprayIntegration -v
package helmspray

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

func TestSprayIntegration(t *testing.T) {
	const ns = "helm-spray-itest"

	deleteNamespace := func() {
		_ = exec.Command("kubectl", "delete", "namespace", ns, "--ignore-not-found", "--wait=false").Run()
	}
	// Start from a clean slate and always tear down afterwards.
	_ = exec.Command("helm", "uninstall", "a", "b", "--namespace", ns).Run()
	deleteNamespace()
	t.Cleanup(func() {
		_ = exec.Command("helm", "uninstall", "a", "b", "--namespace", ns).Run()
		deleteNamespace()
	})

	s := &Spray{
		ChartName:       "testdata/itest",
		Namespace:       ns,
		CreateNamespace: true,
		Timeout:         180,
		Verbose:         true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	if err := s.Spray(ctx); err != nil {
		t.Fatalf("Spray against the live cluster failed: %v", err)
	}

	// Spray returning nil already implies each tier became ready before the next
	// (otherwise wait() would have errored). Confirm both per-sub-chart releases
	// exist and are deployed.
	out, err := exec.Command("helm", "list", "--namespace", ns, "-o", "json").Output()
	if err != nil {
		t.Fatalf("helm list failed: %v", err)
	}
	var releases []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &releases); err != nil {
		t.Fatalf("parsing helm list output: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, r := range releases {
		got[r.Name] = r.Status
	}
	for _, name := range []string{"a", "b"} {
		if status := got[name]; status != "deployed" {
			t.Errorf("release %q: status = %q, want deployed (releases=%v)", name, status, got)
		}
	}
}
