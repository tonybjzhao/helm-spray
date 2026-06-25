package helm

import (
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
