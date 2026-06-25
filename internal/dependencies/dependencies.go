// Package dependencies derives the per-sub-chart deployment metadata (weight,
// targeting and tag allowance) from an umbrella chart and the merged values.
package dependencies

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ThalesGroup/helm-spray/v4/internal/log"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
)

// Dependency is the per-sub-chart metadata helm-spray computes from the umbrella
// chart and the merged values. UsedName is the alias when one is set, otherwise
// the chart Name; CorrespondingReleaseName is UsedName with the configured
// release prefix applied. Targeted reflects --target/--exclude, and
// AllowedByTags reflects whether the sub-chart's tags are enabled.
type Dependency struct {
	Name                     string
	Alias                    string
	UsedName                 string
	AppVersion               string
	Targeted                 bool
	Weight                   int
	CorrespondingReleaseName string
	HasTags                  bool
	AllowedByTags            bool
}

// Get derives the deployment metadata for every sub-chart declared in the
// umbrella chart: its name and alias, its weight (defaulting to 0 when unset),
// whether it is targeted given --target/--exclude, whether its tags are allowed
// by the provided tag values, its AppVersion, and its release name. It performs
// no I/O.
func Get(chart *chart.Chart, values *common.Values, targets []string, excludes []string, releasePrefix string, verbose bool) ([]Dependency, error) {
	// Compute tags
	providedTags := tags(values, verbose)

	// Build the list of all dependencies, and their key attributes
	dependencies := make([]Dependency, len(chart.Metadata.Dependencies))
	for i, req := range chart.Metadata.Dependencies {
		// Dependency name and alias
		dependencies[i].Name = req.Name
		dependencies[i].Alias = req.Alias
		if req.Alias == "" {
			dependencies[i].UsedName = dependencies[i].Name
		} else {
			dependencies[i].UsedName = dependencies[i].Alias
		}

		// Is dependency targeted?
		// If --target or --excludes are specified, it should match the name of the current dependency;
		// If neither --target nor --exclude are specified, then all dependencies are targeted
		if len(targets) > 0 {
			dependencies[i].Targeted = false
			for j := range targets {
				if targets[j] == dependencies[i].UsedName {
					dependencies[i].Targeted = true
				}
			}

		} else if len(excludes) > 0 {
			dependencies[i].Targeted = true
			for j := range excludes {
				if excludes[j] == dependencies[i].UsedName {
					dependencies[i].Targeted = false
				}
			}

		} else {
			dependencies[i].Targeted = true
		}

		// Loop on the tags associated to the dependency and check with the tags provided in the values
		dependencies[i].AllowedByTags = false
		if len(req.Tags) == 0 {
			dependencies[i].HasTags = false
			dependencies[i].AllowedByTags = true
		} else {
			dependencies[i].HasTags = true
			for _, tag := range req.Tags {
				for k, v := range providedTags {
					if k == tag && isTagTrue(v) {
						dependencies[i].AllowedByTags = true
					}
				}
			}
		}

		// Weight of the dependency. If no weight is specified, it defaults to 0
		// (as documented). A genuinely malformed weight is still reported.
		dependencies[i].Weight = 0
		weightValue, err := values.PathValue(dependencies[i].UsedName + ".weight")
		if err == nil {
			weight, convErr := toWeight(weightValue)
			if convErr != nil {
				return nil, fmt.Errorf("computing weight value for sub-chart \"%s\": %w", dependencies[i].UsedName, convErr)
			}
			dependencies[i].Weight = weight
		} else if noValue := (common.ErrNoValue{}); !errors.As(err, &noValue) {
			return nil, fmt.Errorf("computing weight value for sub-chart \"%s\": %w", dependencies[i].UsedName, err)
		}
		dependencies[i].CorrespondingReleaseName = releasePrefix + dependencies[i].UsedName

		// Get the AppVersion that is contained in the Chart.yaml file of the dependency sub-chart
		for _, subChart := range chart.Dependencies() {
			if subChart.Metadata.Name == dependencies[i].Name {
				dependencies[i].AppVersion = subChart.Metadata.AppVersion
				break
			}
		}
	}
	return dependencies, nil
}

func tags(values *common.Values, verbose bool) map[string]any {
	// Get the list of "tags" specified in the values...
	// (locally-provided values only; values coming from server are not considered)
	if verbose {
		log.Info(1, "looking for \"tags\" in values provided through \"--values/-f\", \"--set\", \"--set-string\", and \"--set-file\"...")
	}
	var providedTags map[string]any
	tags, err := values.Table("tags")
	if err == nil {
		providedTags = tags.AsMap()
	}
	if verbose {
		for k, v := range providedTags {
			log.Info(2, "found tag \"%s: %s\"", k, fmt.Sprint(v))
		}
	}
	return providedTags
}

// isTagTrue reports whether a tag value supplied through values enables the tag.
// It accepts a boolean true as well as the string spellings understood by
// strconv.ParseBool (e.g. "true", "True", "1"), so that a tag set in a YAML
// values file (where it may be parsed as a string) behaves like a tag set via
// --set (where Helm coerces it to a bool).
func isTagTrue(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(val))
		return err == nil && b
	default:
		return false
	}
}

// toWeight converts a weight value parsed from the merged values into a
// non-negative int. Depending on the YAML/JSON parser the raw value may be a
// json.Number, a float64 or an int.
func toWeight(raw any) (int, error) {
	var weight int
	switch v := raw.(type) {
	case json.Number:
		w, err := v.Int64()
		if err != nil {
			return 0, err
		}
		weight = int(w)
	case float64:
		weight = int(v)
	case int:
		weight = v
	default:
		return 0, fmt.Errorf("value shall be an integer")
	}
	if weight < 0 {
		return 0, fmt.Errorf("value shall be positive or equal to zero")
	}
	return weight, nil
}
