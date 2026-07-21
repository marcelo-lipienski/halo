package diagnostics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/marcelo-lipienski/halo/output"
)

// CheckImageTags audits the images defined in compose services for mutable/unlocked tags.
func (e *Engine) CheckImageTags() []output.CheckResult {
	var results []output.CheckResult
	if e.Compose == nil || len(e.Compose.Services) == 0 {
		return results
	}

	var svcNames []string
	for name := range e.Compose.Services {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

	for _, name := range svcNames {
		svc := e.Compose.Services[name]
		if svc.Image == "" {
			continue
		}

		image := svc.Image
		var tag string
		var isDigest bool
		var hasTag bool

		if strings.Contains(image, "@sha256:") {
			isDigest = true
		} else {
			// Find the last colon, but ignore it if it is part of a port in the registry name.
			// The registry part is before the first slash.
			lastColon := strings.LastIndex(image, ":")
			if lastColon != -1 {
				firstSlash := strings.Index(image, "/")
				if firstSlash == -1 || lastColon > firstSlash {
					tag = image[lastColon+1:]
					hasTag = true
				}
			}
		}

		if isDigest {
			results = append(results, output.CheckResult{
				Group:  "Environmental Alignment",
				Name:   "Image Security: " + name,
				Status: output.CheckPassed,
			})
			continue
		}

		if !hasTag || tag == "" {
			results = append(results, output.CheckResult{
				Group:      "Environmental Alignment",
				Name:       "Image Security: " + name,
				Status:     output.CheckWarning,
				Error:      fmt.Sprintf("Service '%s' uses image '%s' without an explicit tag (implicitly uses latest)", name, image),
				Mitigation: "Pin to a specific version tag or digest (e.g., 'image:1.25.0' or 'image@sha256:...') to ensure reproducible environments",
			})
			continue
		}

		mutableTags := map[string]bool{
			"latest":      true,
			"master":      true,
			"main":        true,
			"dev":         true,
			"development": true,
			"nightly":     true,
			"staging":     true,
			"canary":      true,
		}
		if mutableTags[strings.ToLower(tag)] {
			results = append(results, output.CheckResult{
				Group:      "Environmental Alignment",
				Name:       "Image Security: " + name,
				Status:     output.CheckWarning,
				Error:      fmt.Sprintf("Service '%s' uses image '%s' with mutable tag '%s'", name, image, tag),
				Mitigation: "Pin to a specific version tag or digest (e.g., 'image:1.25.0' or 'image@sha256:...') to ensure reproducible environments",
			})
		} else {
			results = append(results, output.CheckResult{
				Group:  "Environmental Alignment",
				Name:   "Image Security: " + name,
				Status: output.CheckPassed,
			})
		}
	}

	return results
}
