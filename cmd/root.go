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
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ThalesGroup/helm-spray/v4/internal/gui"
	"github.com/ThalesGroup/helm-spray/v4/internal/log"
	"github.com/ThalesGroup/helm-spray/v4/pkg/helm"
	"github.com/ThalesGroup/helm-spray/v4/pkg/helmspray"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	globalUsage = `
This command upgrades sub charts from an umbrella chart supporting deployment orders.
A release is created for each subchart

Arguments shall be a chart reference, a path to a packaged chart,
a path to an unpacked chart directory or a URL.

To override values in a chart, use either the '--values'/'-f' flag and pass in a file name
or use the '--set' flag and pass configuration from the command line.
To force string values in '--set', use '--set-string' instead.
In case a value is large and therefore you want not to use neither '--values' 
nor '--set', use '--set-file' to read the single large value from file.

 $ helm spray -f myvalues.yaml ./umbrella-chart
 $ helm spray --set key1=val1,key2=val2 ./umbrella-chart
 $ helm spray stable/umbrella-chart
 $ helm spray umbrella-chart-1.0.0-rc.1+build.32.tgz -f myvalues.yaml

You can specify the '--values'/'-f' flag several times or provide a single comma separated value.
You can specify the '--set' flag several times or provide a single comma separated value.
Helm Spray reserves Helm Conditions for its own use ('<chart name or alias>.enabled')
and supports Helm Tags, with some restrictions.

To preview a release's rendered manifests without installing it, combine the
'--debug' and '--dry-run' flags. To preview the whole solution's weight-ordered
deployment plan without contacting the cluster, use '--output json'.

There are four different ways you can express the chart you want to install:

 1. By chart reference within a repo: helm spray stable/umbrella-chart
 2. By path to a packaged chart: helm spray umbrella-chart-1.0.0-rc.1+build.32.tgz
 3. By path to an unpacked chart directory: helm spray ./umbrella-chart
 4. By absolute URL: helm spray https://example.com/charts/umbrella-chart-1.0.0-rc.1+build.32.tgz

When specifying a chart reference or a chart URL, it installs the latest version
of that chart unless you also supply a version number with the '--version' flag.

To see the list of installed releases, use 'helm list'.

To see the list of chart repositories, use 'helm repo list'. To search for
charts in a repository, use 'helm search'.
`
)

var version = "SNAPSHOT"

// releasePrefixPattern enforces the documented character set for
// --prefix-releases (a-z, A-Z, 0-9 and -), so an invalid prefix is rejected
// up front rather than surfacing later as an opaque helm release-name error.
var releasePrefixPattern = regexp.MustCompile("^[a-zA-Z0-9-]+$")

func NewRootCmd() *cobra.Command {

	s := &helmspray.Spray{}

	var output string

	cmd := &cobra.Command{
		Use:          "spray [CHART]",
		Short:        fmt.Sprintf("upgrade subcharts from an umbrella chart (helm-spray %s)", version),
		Long:         globalUsage,
		SilenceUsage: true,
		// The chart is a positional argument, not a sub-command. Once the "ui"
		// sub-command is registered, cobra's default validation would otherwise
		// reject any non-sub-command first argument as an "unknown command", so
		// arbitrary args are accepted here and validated in RunE.
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) == 0 {
				return errors.New("this command needs at least 1 argument: chart name")
			} else if len(args) > 1 {
				return errors.New("this command accepts only 1 argument: chart name")
			}

			s.ChartName = args[0]
			resolveNamespace(cmd, s)
			applyHelmDebug(s)

			if s.ChartVersion != "" {
				if strings.HasSuffix(s.ChartName, "tgz") {
					return errors.New("cannot use --version together with chart archive")
				}

				if _, err := os.Stat(s.ChartName); err == nil {
					return errors.New("cannot use --version together with chart directory")
				}

				if strings.HasPrefix(s.ChartName, "http://") || strings.HasPrefix(s.ChartName, "https://") {
					return errors.New("cannot use --version together with chart HTTP(S) URL")
				}
			}

			if s.PrefixReleasesWithNamespace && s.PrefixReleases != "" {
				return errors.New("cannot use both --prefix-releases and --prefix-releases-with-namespace together")
			}

			if s.PrefixReleases != "" && !releasePrefixPattern.MatchString(s.PrefixReleases) {
				return errors.New("invalid --prefix-releases value: allowed characters are a-z, A-Z, 0-9 and -")
			}

			if len(s.Targets) > 0 && len(s.Excludes) > 0 {
				return errors.New("cannot use both --target and --exclude together")
			}

			if s.ResetValues && s.ReuseValues {
				return errors.New("cannot use both --reset-values and --reuse-values together")
			}

			if output != "" && output != "json" {
				return fmt.Errorf("unsupported --output format %q (supported: json)", output)
			}

			// Fetch the chart when it is a remote reference or not present locally.
			// The cleanup must outlive the spray, so it is deferred here in RunE.
			cleanup, err := prepareChartSource(cmd.Context(), s)
			if err != nil {
				return err
			}
			defer cleanup()

			if output == "json" {
				plan, err := s.Plan()
				if err != nil {
					return fmt.Errorf("computing deployment plan: %w", err)
				}
				encoded, err := json.MarshalIndent(plan, "", "  ")
				if err != nil {
					return fmt.Errorf("encoding deployment plan: %w", err)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
				return nil
			}

			return s.Spray(cmd.Context())
		},
	}

	f := cmd.Flags()
	f.StringVarP(&s.Namespace, "namespace", "n", "", "namespace to spray the chart into (overrides the HELM_NAMESPACE environment variable; defaults to \"default\")")
	f.StringVarP(&s.ChartVersion, "version", "", "", "specify the exact chart version to install. If this is not specified, the latest version is installed")
	f.StringSliceVarP(&s.Targets, "target", "t", []string{}, "specify the subchart to target (can specify multiple). If '--target' is not specified, all subcharts are targeted")
	f.StringSliceVarP(&s.Excludes, "exclude", "x", []string{}, "specify the subchart to exclude (can specify multiple): process all subcharts except the ones specified in '--exclude'")
	f.StringVarP(&s.PrefixReleases, "prefix-releases", "", "", "prefix the releases by the given string, resulting into releases names formats:\n    \"<prefix>-<chart name or alias>\"\nAllowed characters are a-z A-Z 0-9 and -")
	f.BoolVar(&s.PrefixReleasesWithNamespace, "prefix-releases-with-namespace", false, "prefix the releases by the name of the namespace, resulting into releases names formats:\n    \"<namespace>-<chart name or alias>\"")
	f.BoolVar(&s.CreateNamespace, "create-namespace", false, "automatically create the namespace if necessary")
	f.BoolVar(&s.ResetValues, "reset-values", false, "when upgrading, reset the values to the ones built into the chart")
	f.BoolVar(&s.ReuseValues, "reuse-values", false, "when upgrading, reuse the last release's values and merge in any overrides from the command line via '--set' and '-f'.\nCannot be combined with --reset-values")
	f.StringSliceVarP(&s.ValuesOpts.ValueFiles, "values", "f", []string{}, "specify values in a YAML file or a URL (can specify multiple)")
	f.StringArrayVar(&s.ValuesOpts.Values, "set", []string{}, "set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	f.StringArrayVar(&s.ValuesOpts.StringValues, "set-string", []string{}, "set STRING values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	f.StringArrayVar(&s.ValuesOpts.FileValues, "set-file", []string{}, "set values from respective files specified via the command line (can specify multiple or separate values with commas: key1=path1,key2=path2)")
	f.BoolVar(&s.Force, "force", false, "force resource update through delete/recreate if needed")
	f.BoolVar(&s.Prune, "prune", false, "after deploying, uninstall releases for sub-charts that are no longer part of the umbrella chart")
	f.IntVar(&s.Timeout, "timeout", 300, "time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks)\nand for liveness and readiness (like Deployments and regular Jobs completion)")
	f.BoolVar(&s.DryRun, "dry-run", false, "simulate a spray")
	f.BoolVarP(&s.Verbose, "verbose", "v", false, "enable spray verbose output")
	f.BoolVar(&s.Debug, "debug", false, "enable helm debug output (also include spray verbose output)")
	f.StringVarP(&output, "output", "o", "", "print the resolved, weight-ordered deployment plan and exit without deploying. Supported format: 'json'")

	cmd.AddCommand(newUICmd())
	cmd.AddCommand(newUninstallCmd())

	return cmd
}

// resolveNamespace resolves the target namespace: an explicit --namespace flag
// wins; otherwise HELM_NAMESPACE (exported by helm for its plugins); otherwise
// "default".
func resolveNamespace(cmd *cobra.Command, s *helmspray.Spray) {
	if !cmd.Flags().Changed("namespace") {
		if ns := os.Getenv("HELM_NAMESPACE"); ns != "" {
			s.Namespace = ns
		} else {
			s.Namespace = "default"
		}
	}
}

// applyHelmDebug enables debug output (and, with it, verbose output) when helm
// signals it through the HELM_DEBUG environment variable or when --debug was
// passed on the command line.
func applyHelmDebug(s *helmspray.Spray) {
	helmDebug := os.Getenv("HELM_DEBUG")
	if helmDebug == "1" || strings.EqualFold(helmDebug, "true") || strings.EqualFold(helmDebug, "on") {
		s.Debug = true
	}
	if s.Debug {
		s.Verbose = true
	}
}

// prepareChartSource ensures s.ChartName points at a chart that can be loaded
// locally, fetching remote references (HTTP(S) URL, OCI reference, or repository
// reference) into a temporary directory. It returns a cleanup function the caller
// must defer; for a chart that is already local the cleanup is a no-op.
func prepareChartSource(ctx context.Context, s *helmspray.Spray) (func(), error) {
	fetch := func(source string) (func(), error) {
		if s.ChartVersion != "" {
			log.Info(1, "fetching chart \"%s\" %s with version \"%s\"...", s.ChartName, source, s.ChartVersion)
		} else {
			log.Info(1, "fetching chart \"%s\" %s...", s.ChartName, source)
		}
		name, cleanup, ferr := helm.Fetch(ctx, s.ChartName, s.ChartVersion)
		if ferr != nil {
			return nil, fmt.Errorf("fetching chart %s with version %s: %w", s.ChartName, s.ChartVersion, ferr)
		}
		s.ChartName = name
		return cleanup, nil
	}

	switch {
	case strings.HasPrefix(s.ChartName, "http://"),
		strings.HasPrefix(s.ChartName, "https://"),
		strings.HasPrefix(s.ChartName, "oci://"):
		return fetch("from its URL")
	default:
		if _, statErr := os.Stat(s.ChartName); statErr != nil {
			return fetch("from the configured repositories")
		}
	}
	log.Info(1, "processing chart from local file or directory \"%s\"...", s.ChartName)
	return func() {}, nil
}

// newUninstallCmd builds the "uninstall" sub-command, which removes the releases
// helm-spray created for an umbrella chart's sub-charts, in reverse weight order.
func newUninstallCmd() *cobra.Command {
	s := &helmspray.Spray{}
	c := &cobra.Command{
		Use:   "uninstall [CHART]",
		Short: "uninstall the releases created for an umbrella chart's sub-charts",
		Long: `This command uninstalls the releases that "helm spray" created for an umbrella
chart's sub-charts. Releases are removed in descending weight order, the reverse
of deployment.

The chart argument is the same umbrella chart (path, reference, or URL) used to
deploy, so helm-spray can compute the set of release names to remove. Use
'--target'/'--exclude' to uninstall a subset, '--prefix-releases'/
'--prefix-releases-with-namespace' to match releases that were deployed with a
prefix, and '--dry-run' to preview the removals.`,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("this command needs at least 1 argument: chart name")
			} else if len(args) > 1 {
				return errors.New("this command accepts only 1 argument: chart name")
			}
			s.ChartName = args[0]
			resolveNamespace(cmd, s)
			applyHelmDebug(s)

			if len(s.Targets) > 0 && len(s.Excludes) > 0 {
				return errors.New("cannot use both --target and --exclude together")
			}
			if s.PrefixReleasesWithNamespace && s.PrefixReleases != "" {
				return errors.New("cannot use both --prefix-releases and --prefix-releases-with-namespace together")
			}
			if s.PrefixReleases != "" && !releasePrefixPattern.MatchString(s.PrefixReleases) {
				return errors.New("invalid --prefix-releases value: allowed characters are a-z, A-Z, 0-9 and -")
			}

			cleanup, err := prepareChartSource(cmd.Context(), s)
			if err != nil {
				return err
			}
			defer cleanup()

			return s.Uninstall(cmd.Context())
		},
	}

	f := c.Flags()
	f.StringVarP(&s.Namespace, "namespace", "n", "", "namespace the releases live in (overrides the HELM_NAMESPACE environment variable; defaults to \"default\")")
	f.StringVarP(&s.ChartVersion, "version", "", "", "chart version to resolve the sub-chart set from (for remote chart references)")
	f.StringSliceVarP(&s.Targets, "target", "t", []string{}, "only uninstall the specified subchart(s) (can specify multiple)")
	f.StringSliceVarP(&s.Excludes, "exclude", "x", []string{}, "uninstall all subcharts except the specified one(s) (can specify multiple)")
	f.StringVarP(&s.PrefixReleases, "prefix-releases", "", "", "release-name prefix used at deploy time, needed to locate the releases")
	f.BoolVar(&s.PrefixReleasesWithNamespace, "prefix-releases-with-namespace", false, "the releases were prefixed with the namespace name at deploy time")
	f.BoolVar(&s.DryRun, "dry-run", false, "show which releases would be uninstalled without removing them")
	f.BoolVarP(&s.Verbose, "verbose", "v", false, "enable verbose output")
	f.BoolVar(&s.Debug, "debug", false, "enable helm debug output (also includes spray verbose output)")
	return c
}

// newUICmd builds the "ui" sub-command, which starts the embedded web UI for
// configuring and visualising a deployment.
func newUICmd() *cobra.Command {
	var address string
	c := &cobra.Command{
		Use:          "ui",
		Short:        "start the helm-spray web UI to configure and visualise a deployment",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return gui.Serve(address)
		},
	}
	c.Flags().StringVar(&address, "address", "127.0.0.1:8080", "address the web UI listens on")
	return c
}
