package gui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ThalesGroup/helm-spray/v4/pkg/helmspray"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	h, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestServesEmbeddedIndex(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "helm-spray") {
		t.Error("served index.html does not contain the app title")
	}
}

func TestPlanAPIRequiresChart(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Post(srv.URL+"/api/plan", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPlanAPIRejectsGet(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Get(srv.URL + "/api/plan")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestPlanAPIReturnsTiers(t *testing.T) {
	srv := newServer(t)
	body := `{"chart":"../../pkg/helmspray/testdata/umbrella","namespace":"demo"}`
	resp, err := http.Post(srv.URL+"/api/plan", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, b)
	}
	var plan helmspray.Plan
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		t.Fatalf("decoding plan: %v", err)
	}
	if plan.Namespace != "demo" {
		t.Errorf("namespace = %q, want demo", plan.Namespace)
	}
	if len(plan.Tiers) != 2 {
		t.Errorf("expected 2 weight tiers, got %d", len(plan.Tiers))
	}
}
