package cmd

import (
	"bytes"
	"encoding/json"
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

// --output json must write only the JSON document to stdout (diagnostics go to
// stderr), so the output is directly machine-parseable.
func TestOutputJSONStdoutIsPureJSON(t *testing.T) {
	c := NewRootCmd()
	var stdout bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(io.Discard)
	c.SetArgs([]string{"--output", "json", "../pkg/helmspray/testdata/umbrella"})
	if err := c.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var plan struct {
		Tiers []struct {
			Weight int `json:"weight"`
		} `json:"tiers"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout was:\n%s", err, stdout.String())
	}
	if len(plan.Tiers) == 0 {
		t.Error("expected at least one weight tier in the plan")
	}
}

// The documented --namespace/-n flag must exist on the root command.
func TestNamespaceFlagRegistered(t *testing.T) {
	f := NewRootCmd().Flags().Lookup("namespace")
	if f == nil {
		t.Fatal("--namespace flag is not registered")
	}
	if f.Shorthand != "n" {
		t.Errorf("--namespace shorthand = %q, want \"n\"", f.Shorthand)
	}
}

func TestUnsupportedOutputFormat(t *testing.T) {
	err := runCmd("-o", "yaml", "some-chart")
	if err == nil || !strings.Contains(err.Error(), "unsupported --output format") {
		t.Fatalf("expected an unsupported-format error, got %v", err)
	}
}

func TestParseTimeout(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"300", 300, false},  // bare seconds
		{"300s", 300, false}, // Helm-style duration
		{"5m", 300, false},   // minutes
		{"1m30s", 90, false}, // compound duration
		{"0", 0, false},      // zero is allowed
		{"", 0, true},        // empty is invalid
		{"-5", 0, true},      // negative seconds rejected
		{"-5s", 0, true},     // negative duration rejected
		{"abc", 0, true},     // not a number or duration
	}
	for _, tc := range cases {
		got, err := parseTimeout(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTimeout(%q): expected an error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTimeout(%q): unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("parseTimeout(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
