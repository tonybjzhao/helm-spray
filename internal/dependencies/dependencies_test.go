package dependencies

import (
	"testing"

	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

// umbrella builds a minimal umbrella chart whose Metadata declares the given
// dependencies, mirroring what loader.Load produces from a requirements list.
func umbrella(deps ...*chart.Dependency) *chart.Chart {
	// Default each dependency to the condition helm-spray expects, unless the
	// test set one explicitly, so fixtures mirror a correctly authored umbrella.
	for _, d := range deps {
		if d.Condition == "" {
			used := d.Name
			if d.Alias != "" {
				used = d.Alias
			}
			d.Condition = used + ".enabled"
		}
	}
	return &chart.Chart{Metadata: &chart.Metadata{Name: "umbrella", Dependencies: deps}}
}

func byUsedName(deps []Dependency) map[string]Dependency {
	m := make(map[string]Dependency, len(deps))
	for _, d := range deps {
		m[d.UsedName] = d
	}
	return m
}

// A sub-chart that omits its weight must default to 0 rather than erroring.
func TestGetWeightDefaultsToZeroWhenMissing(t *testing.T) {
	vals := common.Values{}
	got, err := Get(umbrella(&chart.Dependency{Name: "svc-a"}), &vals, nil, nil, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Weight != 0 {
		t.Fatalf("expected one dependency with default weight 0, got %+v", got)
	}
}

func TestGetParsesAndValidatesWeight(t *testing.T) {
	cases := []struct {
		name       string
		weight     any
		wantWeight int
		wantErr    bool
	}{
		{"int", 3, 3, false},
		{"float64", float64(2), 2, false},
		{"zero", 0, 0, false},
		{"negative", -1, 0, true},
		{"non-integer", "high", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vals := common.Values{"svc-a": map[string]any{"weight": tc.weight}}
			got, err := Get(umbrella(&chart.Dependency{Name: "svc-a"}), &vals, nil, nil, "", false)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for weight %v", tc.weight)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got[0].Weight != tc.wantWeight {
				t.Errorf("weight: got %d want %d", got[0].Weight, tc.wantWeight)
			}
		})
	}
}

// Tags must be honoured whether provided as a real bool (--set) or as a string
// (YAML values file). Sub-charts without tags are always allowed.
func TestGetTagMatching(t *testing.T) {
	deps := []*chart.Dependency{
		{Name: "front", Tags: []string{"frontend"}},
		{Name: "back", Tags: []string{"backend"}},
		{Name: "always"},
	}
	cases := []struct {
		name                            string
		tags                            map[string]any
		wantFront, wantBack, wantAlways bool
	}{
		{"bool true", map[string]any{"frontend": true}, true, false, true},
		{"string true", map[string]any{"frontend": "true"}, true, false, true},
		{"string True", map[string]any{"frontend": "True"}, true, false, true},
		{"bool false", map[string]any{"frontend": false}, false, false, true},
		{"none", map[string]any{}, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vals := common.Values{"tags": tc.tags}
			got, err := Get(umbrella(deps...), &vals, nil, nil, "", false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			m := byUsedName(got)
			if m["front"].AllowedByTags != tc.wantFront {
				t.Errorf("front allowed: got %v want %v", m["front"].AllowedByTags, tc.wantFront)
			}
			if m["back"].AllowedByTags != tc.wantBack {
				t.Errorf("back allowed: got %v want %v", m["back"].AllowedByTags, tc.wantBack)
			}
			if m["always"].AllowedByTags != tc.wantAlways {
				t.Errorf("always allowed: got %v want %v", m["always"].AllowedByTags, tc.wantAlways)
			}
		})
	}
}

// --target/--exclude operate on the used name (alias when set) and the release
// name carries the configured prefix.
func TestGetTargetingAliasAndPrefix(t *testing.T) {
	deps := []*chart.Dependency{
		{Name: "a"},
		{Name: "b", Alias: "bee"},
	}
	vals := common.Values{}

	got, err := Get(umbrella(deps...), &vals, []string{"bee"}, nil, "rel-", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := byUsedName(got)
	if !m["bee"].Targeted {
		t.Error("aliased sub-chart 'bee' should be targeted")
	}
	if m["a"].Targeted {
		t.Error("'a' should not be targeted when only 'bee' is requested")
	}
	if m["bee"].CorrespondingReleaseName != "rel-bee" {
		t.Errorf("release name: got %q want %q", m["bee"].CorrespondingReleaseName, "rel-bee")
	}

	got, err = Get(umbrella(deps...), &vals, nil, []string{"a"}, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m = byUsedName(got)
	if m["a"].Targeted {
		t.Error("excluded 'a' should not be targeted")
	}
	if !m["bee"].Targeted {
		t.Error("'bee' should be targeted when only 'a' is excluded")
	}
}
