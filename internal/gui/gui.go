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
	"time"

	"github.com/ThalesGroup/helm-spray/v4/internal/log"
	"github.com/ThalesGroup/helm-spray/v4/pkg/helmspray"
	cliValues "helm.sh/helm/v4/pkg/cli/values"
)

// maxRequestBody bounds the size of a /api/plan request body.
const maxRequestBody = 1 << 20 // 1 MiB

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

func handlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var req planRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}
	if req.Chart == "" {
		writeError(w, http.StatusBadRequest, "a chart is required")
		return
	}

	namespace := req.Namespace
	if namespace == "" {
		namespace = "default"
	}
	s := &helmspray.Spray{
		ChartName:                   req.Chart,
		Namespace:                   namespace,
		Targets:                     req.Targets,
		Excludes:                    req.Excludes,
		PrefixReleases:              req.PrefixReleases,
		PrefixReleasesWithNamespace: req.PrefixReleasesWithNamespace,
		ValuesOpts:                  cliValues.Options{Values: req.Set, ValueFiles: req.ValueFiles},
	}

	plan, err := s.Plan()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(plan)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
