/*
(c) Copyright 2018, Gemalto. All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package helm wraps the helm command-line interface. helm-spray drives the
// same helm binary that invoked it (via the HELM_BIN environment variable that
// helm exports to its plugins) and adapts to the differences between the
// supported helm major versions.
package helm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gemalto/helm-spray/v4/internal/log"
)

// UpgradedRelease holds the JSON result of "helm upgrade --install -o json".
type UpgradedRelease struct {
	Info     map[string]interface{} `json:"info"`
	Manifest string                 `json:"manifest"`
}

// Release describes an existing helm release as reported by "helm list -o json".
type Release struct {
	Name       string `json:"name"`
	Revision   string `json:"revision"`
	Updated    string `json:"updated"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
	Namespace  string `json:"namespace"`
}

var (
	helmVersionOnce sync.Once
	helmMajor       int
)

// binary returns the helm executable to invoke. When helm-spray runs as a helm
// plugin, helm exports HELM_BIN pointing at the exact binary that launched the
// plugin; honouring it guarantees the plugin drives the same helm it was invoked
// by, rather than a possibly different "helm" first on PATH.
func binary() string {
	if b := os.Getenv("HELM_BIN"); b != "" {
		return b
	}
	return "helm"
}

// majorVersion returns the major version of the helm CLI (e.g. 3 or 4), detected
// once and cached. If detection fails it defaults to the current major so that
// modern flag spellings are used.
func majorVersion() int {
	helmVersionOnce.Do(func() {
		helmMajor = 4
		out, err := exec.Command(binary(), "version", "--template", "{{.Version}}").Output() // #nosec G204 -- HELM_BIN (set by the helm host) or "helm"; args built internally, no shell
		if err != nil {
			return
		}
		v := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
		if i := strings.IndexByte(v, '.'); i > 0 {
			if m, perr := strconv.Atoi(v[:i]); perr == nil && m > 0 {
				helmMajor = m
			}
		}
	})
	return helmMajor
}

// forceFlag returns the helm upgrade flag that forces resource updates through
// replacement, accounting for the rename from "--force" (helm v3) to
// "--force-replace" (helm v4).
func forceFlag(major int) string {
	if major >= 4 {
		return "--force-replace"
	}
	return "--force"
}

// List returns the helm releases in the given namespace, keyed by release name.
func List(level int, namespace string, debug bool) (map[string]Release, error) {
	myargs := []string{"list", "--namespace", namespace, "-o", "json"}

	if debug {
		log.Info(level, "running helm command : %v", myargs)
	}
	output, err := run(myargs)
	if debug {
		log.Info(level, "helm command returned:\n%s", string(output))
	}
	if err != nil {
		return nil, fmt.Errorf("running helm list in namespace %q: %w", namespace, err)
	}

	var releases []Release
	if err := json.Unmarshal(output, &releases); err != nil {
		return nil, fmt.Errorf("parsing helm list output: %w", err)
	}

	releasesMap := make(map[string]Release, len(releases))
	for _, r := range releases {
		releasesMap[r.Name] = r
	}
	return releasesMap, nil
}

// UpgradeWithValues runs "helm upgrade --install" for a single release and
// returns the parsed JSON result (release info and rendered manifest).
func UpgradeWithValues(level int, namespace string, createNamespace bool, releaseName string, chartPath string, resetValues bool, reuseValues bool, valueFiles []string, valuesSet []string, valuesSetString []string, valuesSetFile []string, force bool, timeout int, dryRun bool, debug bool) (UpgradedRelease, error) {
	myargs := []string{"upgrade", "--install", releaseName, chartPath, "--namespace", namespace, "--timeout", strconv.Itoa(timeout) + "s", "-o", "json"}
	for _, v := range valuesSet {
		myargs = append(myargs, "--set", v)
	}
	for _, v := range valuesSetString {
		myargs = append(myargs, "--set-string", v)
	}
	for _, v := range valuesSetFile {
		myargs = append(myargs, "--set-file", v)
	}
	for _, v := range valueFiles {
		myargs = append(myargs, "-f", v)
	}
	if resetValues {
		myargs = append(myargs, "--reset-values")
	}
	if reuseValues {
		myargs = append(myargs, "--reuse-values")
	}
	if force {
		myargs = append(myargs, forceFlag(majorVersion()))
	}
	if dryRun {
		myargs = append(myargs, "--dry-run")
	}
	if createNamespace {
		myargs = append(myargs, "--create-namespace")
	}

	if debug {
		log.Info(level, "running helm command for \"%s\": %v", releaseName, redactArgs(myargs))
	}
	output, err := run(myargs)
	if debug {
		log.Info(level, "helm command for \"%s\" returned:\n%s", releaseName, string(output))
	}
	if err != nil {
		return UpgradedRelease{}, fmt.Errorf("running helm upgrade for release %q: %w", releaseName, err)
	}

	var upgradedRelease UpgradedRelease
	if err := json.Unmarshal(output, &upgradedRelease); err != nil {
		return UpgradedRelease{}, fmt.Errorf("parsing helm upgrade output for release %q: %w", releaseName, err)
	}
	return upgradedRelease, nil
}

// Fetch downloads the named chart (optionally at a specific version) into a
// freshly created temporary directory and returns the path to the fetched chart
// archive together with a cleanup function the caller must invoke once the chart
// has been loaded. Unlike a plain "helm pull", the chart is never written into
// the current working directory.
func Fetch(chart string, version string) (string, func(), error) {
	noop := func() {}
	tempDir, err := os.MkdirTemp("", "spray-")
	if err != nil {
		return "", noop, fmt.Errorf("creating temporary directory for chart fetch: %w", err)
	}
	cleanup := func() {
		if rerr := os.RemoveAll(tempDir); rerr != nil {
			log.Error("Unable to remove temporary directory: %s", rerr)
		}
	}

	args := []string{"pull", chart, "--destination", tempDir}
	if version != "" {
		args = append(args, "--version", version)
	}
	cmd := exec.Command(binary(), args...) // #nosec G204 -- HELM_BIN (set by the helm host) or "helm"; args built internally, no shell
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("fetching chart %q: %w", chart, err)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		cleanup()
		return "", noop, fmt.Errorf("reading fetched chart directory: %w", err)
	}
	chartFile := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".tgz") {
			chartFile = e.Name()
			break
		}
		if chartFile == "" {
			chartFile = e.Name()
		}
	}
	if chartFile == "" {
		cleanup()
		return "", noop, fmt.Errorf("no chart archive found after fetching %q", chart)
	}
	return filepath.Join(tempDir, chartFile), cleanup, nil
}

// run executes the helm binary with the given arguments and returns its stdout.
// stderr is streamed to the process stderr so helm diagnostics remain visible.
func run(args []string) ([]byte, error) {
	cmd := exec.Command(binary(), args...) // #nosec G204 -- HELM_BIN (set by the helm host) or "helm"; args built internally, no shell
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return stdout.Bytes(), err
}

// redactArgs returns a copy of a helm argument vector with the values that
// follow --set/--set-string/--set-file masked, so secrets passed on the command
// line are not written to debug logs.
func redactArgs(args []string) []string {
	redacted := make([]string, len(args))
	copy(redacted, args)
	for i := 0; i+1 < len(redacted); i++ {
		switch redacted[i] {
		case "--set", "--set-string", "--set-file":
			redacted[i+1] = "[REDACTED]"
		}
	}
	return redacted
}
