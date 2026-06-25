package cmd

import (
	"io"
	"strings"
	"testing"
)

func runCmd(args ...string) error {
	c := NewRootCmd()
	c.SetArgs(args)
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	return c.Execute()
}

// TestArgumentAndFlagValidation covers the validation rules enforced before any
// chart is fetched or sprayed, so it needs neither helm nor a cluster.
func TestArgumentAndFlagValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no args", []string{}, "at least 1 argument"},
		{"too many args", []string{"a", "b"}, "only 1 argument"},
		{"version with archive", []string{"--version", "1.0", "chart.tgz"}, "chart archive"},
		{"both prefix flags", []string{"--prefix-releases", "p", "--prefix-releases-with-namespace", "dummy"}, "prefix-releases"},
		{"invalid prefix chars", []string{"--prefix-releases", "bad name", "dummy"}, "allowed characters"},
		{"target and exclude", []string{"--target", "x", "--exclude", "y", "dummy"}, "--target and --exclude"},
		{"reset and reuse", []string{"--reset-values", "--reuse-values", "dummy"}, "--reset-values and --reuse-values"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runCmd(tc.args...)
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}
