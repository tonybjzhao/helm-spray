package helm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestForceFlag(t *testing.T) {
	cases := []struct {
		major int
		want  string
	}{
		{3, "--force"},
		{4, "--force-replace"},
		{5, "--force-replace"},
	}
	for _, tc := range cases {
		if got := forceFlag(tc.major); got != tc.want {
			t.Errorf("forceFlag(%d) = %q, want %q", tc.major, got, tc.want)
		}
	}
}

func TestBuildUpgradeArgs(t *testing.T) {
	base := UpgradeRequest{
		ReleaseName: "rel",
		ChartPath:   "./chart",
		Namespace:   "ns",
		Timeout:     300,
	}

	t.Run("baseline", func(t *testing.T) {
		got := buildUpgradeArgs(base, 4)
		want := []string{"upgrade", "--install", "rel", "./chart", "--namespace", "ns", "--timeout", "300s", "-o", "json"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v\nwant %v", got, want)
		}
	})

	t.Run("values flags in order", func(t *testing.T) {
		req := base
		req.Values = []string{"a=1"}
		req.StringValues = []string{"b=2"}
		req.FileValues = []string{"c=/f"}
		req.ValueFiles = []string{"v.yaml"}
		got := buildUpgradeArgs(req, 4)
		joined := strings.Join(got, " ")
		for _, want := range []string{"--set a=1", "--set-string b=2", "--set-file c=/f", "-f v.yaml"} {
			if !strings.Contains(joined, want) {
				t.Errorf("missing %q in %q", want, joined)
			}
		}
	})

	t.Run("force flag is version-specific", func(t *testing.T) {
		req := base
		req.Force = true
		if got := buildUpgradeArgs(req, 3); !contains(got, "--force") || contains(got, "--force-replace") {
			t.Errorf("v3 should use --force, got %v", got)
		}
		if got := buildUpgradeArgs(req, 4); !contains(got, "--force-replace") || contains(got, "--force") {
			t.Errorf("v4 should use --force-replace, got %v", got)
		}
	})

	t.Run("boolean flags", func(t *testing.T) {
		req := base
		req.ResetValues = true
		req.ReuseValues = true
		req.DryRun = true
		req.CreateNamespace = true
		got := buildUpgradeArgs(req, 4)
		for _, want := range []string{"--reset-values", "--reuse-values", "--dry-run", "--create-namespace"} {
			if !contains(got, want) {
				t.Errorf("missing %q in %v", want, got)
			}
		}
	})
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestRedactArgs(t *testing.T) {
	in := []string{
		"upgrade", "--install", "rel", "chart",
		"--set", "db.password=s3cret",
		"--set-string", "token=abc",
		"--set-file", "key=/path/to/secret",
		"--namespace", "ns",
	}
	want := []string{
		"upgrade", "--install", "rel", "chart",
		"--set", "[REDACTED]",
		"--set-string", "[REDACTED]",
		"--set-file", "[REDACTED]",
		"--namespace", "ns",
	}
	got := redactArgs(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("redactArgs mismatch:\n got %v\nwant %v", got, want)
	}
	// The input slice must not be mutated (it is the vector actually executed).
	if in[5] != "db.password=s3cret" {
		t.Errorf("redactArgs mutated its input: %v", in)
	}
}

func TestListParsesReleases(t *testing.T) {
	orig := run
	defer func() { run = orig }()
	run = func(_ context.Context, _ []string) ([]byte, error) {
		return []byte(`[{"name":"a","status":"deployed"},{"name":"b","status":"failed"}]`), nil
	}
	got, err := List(context.Background(), "ns", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got["a"].Status != "deployed" || got["b"].Status != "failed" {
		t.Errorf("unexpected releases: %+v", got)
	}
}

func TestListPropagatesRunError(t *testing.T) {
	orig := run
	defer func() { run = orig }()
	run = func(_ context.Context, _ []string) ([]byte, error) { return nil, errors.New("boom") }
	if _, err := List(context.Background(), "ns", false); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected the run error to propagate, got %v", err)
	}
}

func TestListRejectsBadJSON(t *testing.T) {
	orig := run
	defer func() { run = orig }()
	run = func(_ context.Context, _ []string) ([]byte, error) { return []byte("not json"), nil }
	if _, err := List(context.Background(), "ns", false); err == nil {
		t.Fatal("expected a parse error for malformed output")
	}
}

func TestHostVersion(t *testing.T) {
	orig := run
	defer func() { run = orig }()
	run = func(_ context.Context, _ []string) ([]byte, error) { return []byte("v4.2.2\n"), nil }
	got, err := HostVersion(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v4.2.2" {
		t.Errorf("HostVersion = %q, want v4.2.2", got)
	}
}

func TestUninstallBuildsArgs(t *testing.T) {
	orig := run
	defer func() { run = orig }()
	var captured []string
	run = func(_ context.Context, args []string) ([]byte, error) {
		captured = args
		return nil, nil
	}
	if err := Uninstall(context.Background(), "ns", "my-release", true, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"uninstall", "my-release", "--namespace", "ns", "--dry-run"}
	if !reflect.DeepEqual(captured, want) {
		t.Errorf("uninstall args = %v, want %v", captured, want)
	}
}

func TestUpgradeWithValuesParsesResult(t *testing.T) {
	orig := run
	defer func() { run = orig }()
	var captured []string
	run = func(_ context.Context, args []string) ([]byte, error) {
		captured = args
		return []byte(`{"info":{"status":"deployed"},"manifest":"kind: Deployment"}`), nil
	}
	rel, err := UpgradeWithValues(context.Background(), UpgradeRequest{ReleaseName: "r", ChartPath: "c", Namespace: "ns", Timeout: 60})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.Info["status"] != "deployed" || rel.Manifest != "kind: Deployment" {
		t.Errorf("unexpected release: %+v", rel)
	}
	if !contains(captured, "upgrade") || !contains(captured, "--install") {
		t.Errorf("expected an upgrade --install invocation, got %v", captured)
	}
}
