package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// ParseEnv parses a .env file and returns a map of keys to values.
func ParseEnv(path string) (map[string]string, error) {
	return godotenv.Read(path)
}

// ComposeConfig represents the root of docker-compose.yml
type ComposeConfig struct {
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]interface{}    `yaml:"volumes"`
}

// StringOrSlice represents a YAML field that can be a single string or a slice of strings
type StringOrSlice []string

// UnmarshalYAML implements custom decoding for StringOrSlice
func (ss *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*ss = []string{s}
	case yaml.SequenceNode:
		var s []string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*ss = s
	}
	return nil
}

// ComposePorts represents a YAML field that can be a single string, a list of strings, or list of maps
type ComposePorts []string

// UnmarshalYAML implements custom decoding for ComposePorts to support long-form ports syntax
func (cp *ComposePorts) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*cp = []string{s}
	case yaml.SequenceNode:
		for _, node := range value.Content {
			switch node.Kind {
			case yaml.ScalarNode:
				var s string
				if err := node.Decode(&s); err != nil {
					return err
				}
				*cp = append(*cp, s)
			case yaml.MappingNode:
				var m struct {
					Target    interface{} `yaml:"target"`
					Published interface{} `yaml:"published"`
					Protocol  string      `yaml:"protocol"`
					Mode      string      `yaml:"mode"`
				}
				if err := node.Decode(&m); err != nil {
					return err
				}
				target := fmt.Sprintf("%v", m.Target)
				published := fmt.Sprintf("%v", m.Published)
				proto := m.Protocol
				if proto == "" {
					proto = "tcp"
				}
				if published != "" && published != "<nil>" {
					*cp = append(*cp, fmt.Sprintf("%s:%s/%s", published, target, proto))
				} else {
					*cp = append(*cp, fmt.Sprintf("%s/%s", target, proto))
				}
			}
		}
	}
	return nil
}

// ComposeService represents a service inside docker-compose.yml
type ComposeService struct {
	Environment   ComposeEnvironment `yaml:"environment"`
	Ports         ComposePorts       `yaml:"ports"`
	Volumes       []ComposeVolume    `yaml:"volumes"`
	Image         string             `yaml:"image"`
	ContainerName string             `yaml:"container_name"`
	Entrypoint    StringOrSlice      `yaml:"entrypoint"`
	Command       StringOrSlice      `yaml:"command"`
}

// ComposeEnvironment is a custom map type to handle both string slice and map syntax for env vars
type ComposeEnvironment map[string]string

// UnmarshalYAML implements custom YAML decoding for environment variables
func (ce *ComposeEnvironment) UnmarshalYAML(value *yaml.Node) error {
	*ce = make(map[string]string)
	switch value.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}
		*ce = m
	case yaml.SequenceNode:
		var s []string
		if err := value.Decode(&s); err != nil {
			return err
		}
		for _, item := range s {
			parts := strings.SplitN(item, "=", 2)
			key := parts[0]
			val := ""
			if len(parts) == 2 {
				val = parts[1]
			}
			(*ce)[key] = val
		}
	}
	return nil
}

// ComposeVolume represents a volume mount configuration
type ComposeVolume struct {
	Source   string
	Target   string
	Type     string // "bind" or "volume"
	ReadOnly bool
	BaseDir  string // Directory where the compose file containing this volume is located
}

func isWindowsDrivePath(path string) bool {
	if len(path) >= 2 {
		drive := path[0]
		isLetter := (drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')
		if isLetter && path[1] == ':' {
			return true
		}
	}
	return false
}

// UnmarshalYAML implements custom YAML decoding for volume mount configurations
func (cv *ComposeVolume) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		parts := strings.Split(s, ":")
		if len(parts) > 1 && len(parts[0]) == 1 && ((parts[0][0] >= 'a' && parts[0][0] <= 'z') || (parts[0][0] >= 'A' && parts[0][0] <= 'Z')) {
			// Windows drive path: C:\path or C:/path
			cv.Source = parts[0] + ":" + parts[1]
			if len(parts) > 2 {
				cv.Target = parts[2]
			}
			if len(parts) > 3 {
				if parts[3] == "ro" {
					cv.ReadOnly = true
				}
			}
		} else {
			if len(parts) > 0 {
				cv.Source = parts[0]
			}
			if len(parts) > 1 {
				cv.Target = parts[1]
			}
			if len(parts) > 2 {
				if parts[2] == "ro" {
					cv.ReadOnly = true
				}
			}
		}
		cv.Type = "bind"
		// If Source is a simple name (doesn't start with path characters or Windows drive letter), assume it is a named volume
		if !strings.HasPrefix(cv.Source, "/") && !strings.HasPrefix(cv.Source, "./") && !strings.HasPrefix(cv.Source, "../") && cv.Source != "~" && cv.Source != "." && !isWindowsDrivePath(cv.Source) {
			cv.Type = "volume"
		}
	case yaml.MappingNode:
		var m struct {
			Type     string `yaml:"type"`
			Source   string `yaml:"source"`
			Target   string `yaml:"target"`
			ReadOnly bool   `yaml:"read_only"`
		}
		if err := value.Decode(&m); err != nil {
			return err
		}
		cv.Source = m.Source
		cv.Target = m.Target
		cv.Type = m.Type
		cv.ReadOnly = m.ReadOnly
		if cv.Type == "" {
			cv.Type = "bind"
			if !strings.HasPrefix(cv.Source, "/") && !strings.HasPrefix(cv.Source, "./") && !strings.HasPrefix(cv.Source, "../") && cv.Source != "~" && cv.Source != "." && !isWindowsDrivePath(cv.Source) {
				cv.Type = "volume"
			}
		}
	}
	return nil
}

// ParseCompose parses a docker-compose.yml file.
func ParseCompose(path string) (*ComposeConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config ComposeConfig
	dec := yaml.NewDecoder(file)
	if err := dec.Decode(&config); err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(absPath)
	for svcName, svc := range config.Services {
		for i := range svc.Volumes {
			svc.Volumes[i].BaseDir = baseDir
		}
		config.Services[svcName] = svc
	}

	return &config, nil
}

// MergeComposeConfigs merges multiple compose configurations from left to right.
// Latter configurations override or append to earlier ones according to Docker Compose rules.
func MergeComposeConfigs(configs ...*ComposeConfig) *ComposeConfig {
	merged := &ComposeConfig{
		Services: make(map[string]ComposeService),
		Volumes:  make(map[string]interface{}),
	}

	for _, config := range configs {
		if config == nil {
			continue
		}

		// Merge Services
		for svcName, srcSvc := range config.Services {
			destSvc, exists := merged.Services[svcName]
			if !exists {
				// Simply insert
				merged.Services[svcName] = srcSvc
				continue
			}

			// Merge service fields
			// Image: latter overrides former if not empty
			if srcSvc.Image != "" {
				destSvc.Image = srcSvc.Image
			}
			// ContainerName: latter overrides former if not empty
			if srcSvc.ContainerName != "" {
				destSvc.ContainerName = srcSvc.ContainerName
			}
			// Entrypoint: latter overrides former if defined
			if len(srcSvc.Entrypoint) > 0 {
				destSvc.Entrypoint = srcSvc.Entrypoint
			}
			// Command: latter overrides former if defined
			if len(srcSvc.Command) > 0 {
				destSvc.Command = srcSvc.Command
			}

			// Environment: merge keys, latter overrides former
			if destSvc.Environment == nil {
				destSvc.Environment = make(ComposeEnvironment)
			}
			for k, v := range srcSvc.Environment {
				destSvc.Environment[k] = v
			}

			// Ports: append lists
			destSvc.Ports = append(destSvc.Ports, srcSvc.Ports...)

			// Volumes: latter overrides former if container target path is the same
			srcTargets := make(map[string]bool)
			for _, v := range srcSvc.Volumes {
				srcTargets[v.Target] = true
			}
			var mergedVols []ComposeVolume
			for _, v := range destSvc.Volumes {
				if !srcTargets[v.Target] {
					mergedVols = append(mergedVols, v)
				}
			}
			destSvc.Volumes = append(mergedVols, srcSvc.Volumes...)

			merged.Services[svcName] = destSvc
		}

		// Merge root-level Volumes
		for volName, volDef := range config.Volumes {
			merged.Volumes[volName] = volDef
		}
	}

	return merged
}
