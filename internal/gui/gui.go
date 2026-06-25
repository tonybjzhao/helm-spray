// Package gui serves the helm-spray web UI: a single-binary web application
// (assets embedded via go:embed) that lets an operator configure a spray and
// visualise the weight-ordered deployment plan of an umbrella chart.
package gui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ThalesGroup/helm-spray/v5/internal/log"
	"github.com/ThalesGroup/helm-spray/v5/pkg/helm"
	"github.com/ThalesGroup/helm-spray/v5/pkg/helmspray"
	cliValues "helm.sh/helm/v4/pkg/cli/values"
)

// maxRequestBody bounds the size of an API request body.
const maxRequestBody = 1 << 20 // 1 MiB

// Version is the helm-spray version displayed in the UI. The command layer sets
// it at start-up so the binary and the UI report the same version.
var Version = "SNAPSHOT"

//go:embed web
var webFS embed.FS

// planRequest is the JSON body accepted by POST /api/plan.
type planRequest struct {
	Chart                       string   `json:"chart"`
	Namespace                   string   `json:"namespace"`
	Targets                     []string `json:"targets"`
	Excludes                    []string `json:"excludes"`
	Set                         []string `json:"set"`
	ValueFiles                  []string `json:"valueFiles"`
	PrefixReleases              string   `json:"prefixReleases"`
	PrefixReleasesWithNamespace bool     `json:"prefixReleasesWithNamespace"`
}

// Handler returns the HTTP handler for the web UI: the embedded static assets
// plus the JSON plan API. It is exported so it can be tested with httptest.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, fmt.Errorf("preparing embedded web assets: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/plan", handlePlan)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/version", handleVersion)
	return mux, nil
}

// Serve starts the web UI on addr and blocks until the server stops.
func Serve(addr string) error {
	handler, err := Handler()
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if host, _, splitErr := net.SplitHostPort(addr); splitErr != nil || !isLoopback(host) {
		log.Info(1, "warning: the web UI has no authentication; binding to a non-loopback address (%s) exposes it on the network", addr)
	}
	log.Info(1, "helm-spray UI listening on http://%s", addr)
	return srv.ListenAndServe()
}

// isLoopback reports whether host is the loopback interface.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isRemoteRef reports whether s is a remote chart or value-file reference that the
// read-only web UI must not fetch server-side.
func isRemoteRef(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "oci://")
}

// sprayFromRequest decodes and validates a planRequest and builds the
// corresponding Spray. On any problem it writes the error response and returns
// ok=false, so callers can simply `return`.
func sprayFromRequest(w http.ResponseWriter, r *http.Request) (*helmspray.Spray, bool) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var req planRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return nil, false
	}
	if req.Chart == "" {
		writeError(w, http.StatusBadRequest, "a chart is required")
		return nil, false
	}
	// The web UI previews local charts only. Reject remote chart references and
	// remote value-file URLs so the server cannot be coerced into fetching
	// arbitrary URLs on the caller's behalf (SSRF).
	if isRemoteRef(req.Chart) {
		writeError(w, http.StatusBadRequest, "the web UI previews local charts only; use the 'helm spray' CLI for remote chart references")
		return nil, false
	}
	for _, vf := range req.ValueFiles {
		if isRemoteRef(vf) {
			writeError(w, http.StatusBadRequest, "remote value-file URLs are not allowed from the web UI; pass local files")
			return nil, false
		}
	}
	if len(req.Targets) > 0 && len(req.Excludes) > 0 {
		writeError(w, http.StatusBadRequest, "cannot use both targets and excludes together")
		return nil, false
	}

	namespace := req.Namespace
	if namespace == "" {
		namespace = "default"
	}
	return &helmspray.Spray{
		ChartName:                   req.Chart,
		Namespace:                   namespace,
		Targets:                     req.Targets,
		Excludes:                    req.Excludes,
		PrefixReleases:              req.PrefixReleases,
		PrefixReleasesWithNamespace: req.PrefixReleasesWithNamespace,
		ValuesOpts:                  cliValues.Options{Values: req.Set, ValueFiles: req.ValueFiles},
	}, true
}

// handlePlan computes the weight-ordered deployment plan without contacting the
// cluster.
func handlePlan(w http.ResponseWriter, r *http.Request) {
	s, ok := sprayFromRequest(w, r)
	if !ok {
		return
	}
	plan, err := s.Plan()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plan)
}

// handleStatus returns the plan augmented with the live helm status of each
// release, so the UI can colour the tiers while a deployment runs. It is
// read-only: the actual deploy stays on the CLI.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	s, ok := sprayFromRequest(w, r)
	if !ok {
		return
	}
	plan, releases, err := s.LiveStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"plan": plan, "releases": releases})
}

// handleVersion reports the helm-spray version and the version of the helm CLI
// it would drive. A missing helm binary yields an empty helm field rather than
// an error, so the UI still loads.
func handleVersion(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{"spray": Version, "helm": ""}
	if v, err := helm.HostVersion(r.Context()); err == nil {
		resp["helm"] = v
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
