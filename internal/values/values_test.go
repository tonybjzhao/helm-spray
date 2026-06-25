package values

import (
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

func chartWithValues(valuesYAML string, files map[string]string) *chart.Chart {
	ch := &chart.Chart{
		Raw: []*common.File{{Name: "values.yaml", Data: []byte(valuesYAML)}},
	}
	for name, data := range files {
		ch.Files = append(ch.Files, &common.File{Name: name, Data: []byte(data)})
	}
	return ch
}

func TestProcessIncludeWholeFile(t *testing.T) {
	ch := chartWithValues("a:\n  weight: 0\n#! {{ .Files.Get inc.yaml }}\n", map[string]string{
		"inc.yaml": "b:\n  key: val\n",
	})
	got, err := processIncludeInValuesFile(ch, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "b:") || !strings.Contains(got, "key: val") {
		t.Errorf("include was not expanded:\n%s", got)
	}
	if strings.Contains(got, "#!") {
		t.Errorf("include directive was not removed:\n%s", got)
	}
}

func TestProcessIncludeWithIndent(t *testing.T) {
	ch := chartWithValues("a:\n#! {{ .Files.Get inc.yaml | indent 2 }}\n", map[string]string{
		"inc.yaml": "key: val\n",
	})
	got, err := processIncludeInValuesFile(ch, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "  key: val") {
		t.Errorf("expected 2-space indented include, got:\n%q", got)
	}
}

func TestProcessIncludePickSubPath(t *testing.T) {
	ch := chartWithValues("#! {{ pick (.Files.Get inc.yaml) foo }}\n", map[string]string{
		"inc.yaml": "foo:\n  nested: deepvalue\nbar:\n  other: skipme\n",
	})
	got, err := processIncludeInValuesFile(ch, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "nested: deepvalue") {
		t.Errorf("expected picked sub-path content, got:\n%s", got)
	}
	if strings.Contains(got, "skipme") {
		t.Errorf("content outside the picked path should not be included:\n%s", got)
	}
}

func TestProcessIncludeMissingFileErrors(t *testing.T) {
	ch := chartWithValues("#! {{ .Files.Get nope.yaml }}\n", nil)
	if _, err := processIncludeInValuesFile(ch, false); err == nil {
		t.Fatal("expected an error referencing the missing include file")
	}
}

func TestMergeMapsDeep(t *testing.T) {
	base := map[string]any{
		"section": map[string]any{"a": 1, "b": 2},
		"keep":    "yes",
	}
	override := map[string]any{
		"section": map[string]any{"b": 3, "c": 4},
		"add":     "new",
	}
	out := mergeMaps(base, override)

	section, ok := out["section"].(map[string]any)
	if !ok {
		t.Fatalf("section is not a map: %T", out["section"])
	}
	if section["a"] != 1 || section["b"] != 3 || section["c"] != 4 {
		t.Errorf("deep merge incorrect: %v", section)
	}
	if out["keep"] != "yes" || out["add"] != "new" {
		t.Errorf("top-level merge incorrect: %v", out)
	}
}
