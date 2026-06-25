package gui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ThalesGroup/helm-spray/v5/pkg/helmspray"
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

func TestVersionAPIReportsSprayVersion(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Get(srv.URL + "/api/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var v map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	if _, ok := v["spray"]; !ok {
		t.Errorf("version response is missing the spray field: %v", v)
	}
}

func TestStatusAPIRequiresChart(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Post(srv.URL+"/api/status", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestStatusAPIRejectsGet(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestPlanAPIRejectsRemoteChart(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Post(srv.URL+"/api/plan", "application/json", strings.NewReader(`{"chart":"https://evil.example/c.tgz"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a remote chart reference must be rejected; status = %d, want 400", resp.StatusCode)
	}
}

func TestPlanAPIRejectsRemoteValueFile(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Post(srv.URL+"/api/plan", "application/json", strings.NewReader(`{"chart":"./local","valueFiles":["http://evil.example/v.yaml"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a remote value-file URL must be rejected; status = %d, want 400", resp.StatusCode)
	}
}

func TestConfigAPIReturnsDefaults(t *testing.T) {
	Defaults = Config{Chart: "./demo", Namespace: "ns1", Targets: []string{"a"}}
	defer func() { Defaults = Config{} }()
	srv := newServer(t)
	resp, err := http.Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var c map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	if c["chart"] != "./demo" || c["namespace"] != "ns1" {
		t.Errorf("config = %v, want the launched chart/namespace", c)
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"localhost": true, "127.0.0.1": true, "::1": true,
		"0.0.0.0": false, "192.168.1.10": false, "example.com": false, "": false,
	}
	for host, want := range cases {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestPlanAPIRejectsOversizedBody(t *testing.T) {
	srv := newServer(t)
	big := `{"chart":"` + strings.Repeat("a", 2<<20) + `"}` // ~2 MiB, over the 1 MiB limit
	resp, err := http.Post(srv.URL+"/api/plan", "application/json", strings.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected an oversized body to be rejected, got %d", resp.StatusCode)
	}
}

func TestPlanAPIRejectsTargetsAndExcludes(t *testing.T) {
	srv := newServer(t)
	body := `{"chart":"x","targets":["a"],"excludes":["b"]}`
	resp, err := http.Post(srv.URL+"/api/plan", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for targets+excludes, got %d", resp.StatusCode)
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
