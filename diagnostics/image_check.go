package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/marcelo-lipienski/halo/config"
	"github.com/marcelo-lipienski/halo/output"
)

// isMutableTag helper evaluates if an image string uses a mutable tag.
func isMutableTag(image string) (bool, string, string) {
	if strings.Contains(image, "@sha256:") {
		return false, "", ""
	}
	var hasTag bool
	var tag string
	lastColon := strings.LastIndex(image, ":")
	if lastColon != -1 {
		firstSlash := strings.Index(image, "/")
		if firstSlash == -1 || lastColon > firstSlash {
			tag = image[lastColon+1:]
			hasTag = true
		}
	}
	if !hasTag || tag == "" {
		return true, "", "implicitly uses latest"
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
		return true, tag, fmt.Sprintf("mutable tag '%s'", tag)
	}
	return false, tag, ""
}

// CheckImageTags audits the images defined in compose services for mutable/unlocked tags,
// including base images defined in referenced Dockerfiles.
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

		// 1. Audit compose-level image tag if present
		if svc.Image != "" {
			image := svc.Image
			isMutable, tag, _ := isMutableTag(image)

			if !isMutable {
				results = append(results, output.CheckResult{
					Group:  "Environmental Alignment",
					Name:   "Image Security: " + name,
					Status: output.CheckPassed,
				})
			} else {
				var errStr string
				if tag == "" {
					errStr = fmt.Sprintf("Service '%s' uses image '%s' without an explicit tag (implicitly uses latest)", name, image)
				} else {
					errStr = fmt.Sprintf("Service '%s' uses image '%s' with mutable tag '%s'", name, image, tag)
				}
				results = append(results, output.CheckResult{
					Group:      "Environmental Alignment",
					Name:       "Image Security: " + name,
					Status:     output.CheckWarning,
					Error:      errStr,
					Mitigation: "Pin to a specific version tag or digest (e.g., 'image:1.25.0' or 'image@sha256:...') to ensure reproducible environments",
				})
			}
		}

		// 2. Audit Dockerfile base images if build block is present
		if svc.Build.Context != "" {
			dfResults := e.checkDockerfile(name, svc.Build)
			results = append(results, dfResults...)
		}
	}

	return results
}

func (e *Engine) checkDockerfile(serviceName string, build config.ComposeBuild) []output.CheckResult {
	var results []output.CheckResult

	baseDir := filepath.Dir(e.ComposePath)
	contextDir := filepath.Join(baseDir, build.Context)
	dockerfileName := build.Dockerfile
	if dockerfileName == "" {
		dockerfileName = "Dockerfile"
	}
	dockerfilePath := filepath.Join(contextDir, dockerfileName)

	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		// Skip check if the Dockerfile is missing (could be built dynamically or out of repo context)
		return nil
	}

	lines := strings.Split(string(data), "\n")
	stageAliases := make(map[string]bool)
	var warnings []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		words := strings.Fields(line)
		if len(words) < 2 || strings.ToUpper(words[0]) != "FROM" {
			continue
		}

		var image string
		var imageIdx int
		for idx, word := range words[1:] {
			if strings.HasPrefix(word, "--") {
				continue
			}
			image = word
			imageIdx = idx + 1
			break
		}

		if image == "" {
			continue
		}

		// Track stage aliases for multi-stage builds (e.g. FROM image AS stage_name)
		for i := imageIdx + 1; i < len(words); i++ {
			if strings.ToUpper(words[i]) == "AS" && i+1 < len(words) {
				alias := strings.ToLower(words[i+1])
				stageAliases[alias] = true
				break
			}
		}

		if stageAliases[strings.ToLower(image)] {
			continue
		}

		if isMutable, _, reason := isMutableTag(image); isMutable {
			warnings = append(warnings, fmt.Sprintf("base image '%s' (%s)", image, reason))
		}
	}

	if len(warnings) > 0 {
		results = append(results, output.CheckResult{
			Group:      "Environmental Alignment",
			Name:       "Image Security (Dockerfile): " + serviceName,
			Status:     output.CheckWarning,
			Error:      fmt.Sprintf("Service '%s' build uses mutable base image(s) in %s: %s", serviceName, dockerfileName, strings.Join(warnings, ", ")),
			Mitigation: fmt.Sprintf("Pin the base image in %s to a specific version or digest (e.g. 'FROM image:1.20')", dockerfilePath),
		})
	} else {
		results = append(results, output.CheckResult{
			Group:  "Environmental Alignment",
			Name:   "Image Security (Dockerfile): " + serviceName,
			Status: output.CheckPassed,
		})
	}

	return results
}
