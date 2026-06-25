package helm

import (
	"reflect"
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
