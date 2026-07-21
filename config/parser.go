package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Services map[string]ComposeService   `yaml:"services"`
	Volumes  map[string]interface{}      `yaml:"volumes"`
	Secrets  map[string]ComposeSecret    `yaml:"secrets"`
	Configs  map[string]ComposeConfigDef `yaml:"configs"`
}

// StringOrSlice represents a YAML field that can be a single string or a slice of strings
type StringOrSlice []string

// ComposeSecret represents a secret definition in compose
type ComposeSecret struct {
	File     string      `yaml:"file"`
	External interface{} `yaml:"external"`
	BaseDir  string
}

// ComposeConfigDef represents a config definition in compose
type ComposeConfigDef struct {
	File     string      `yaml:"file"`
	External interface{} `yaml:"external"`
	BaseDir  string
}

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

// ComposeEnvFile represents an environment file mapping in compose
type ComposeEnvFile struct {
	File     string
	Required bool
	BaseDir  string
}

// ComposeEnvFiles represents a custom type for unmarshaling env_file
type ComposeEnvFiles []ComposeEnvFile

// UnmarshalYAML implements custom decoding for ComposeEnvFiles
func (cef *ComposeEnvFiles) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*cef = []ComposeEnvFile{{File: s, Required: true}}
	case yaml.SequenceNode:
		for _, node := range value.Content {
			switch node.Kind {
			case yaml.ScalarNode:
				var s string
				if err := node.Decode(&s); err != nil {
					return err
				}
				*cef = append(*cef, ComposeEnvFile{File: s, Required: true})
			case yaml.MappingNode:
				var m struct {
					File     string `yaml:"file"`
					Required *bool  `yaml:"required"`
				}
				if err := node.Decode(&m); err != nil {
					return err
				}
				req := true
				if m.Required != nil {
					req = *m.Required
				}
				*cef = append(*cef, ComposeEnvFile{File: m.File, Required: req})
			}
		}
	}
	return nil
}

// ComposeServiceSecret represents a service secret mapping entry
type ComposeServiceSecret struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// ComposeServiceSecrets represents a custom type for unmarshaling service secrets
type ComposeServiceSecrets []ComposeServiceSecret

// UnmarshalYAML implements custom decoding for ComposeServiceSecrets
func (css *ComposeServiceSecrets) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*css = []ComposeServiceSecret{{Source: s}}
	case yaml.SequenceNode:
		for _, node := range value.Content {
			switch node.Kind {
			case yaml.ScalarNode:
				var s string
				if err := node.Decode(&s); err != nil {
					return err
				}
				*css = append(*css, ComposeServiceSecret{Source: s})
			case yaml.MappingNode:
				var m struct {
					Source string `yaml:"source"`
					Target string `yaml:"target"`
				}
				if err := node.Decode(&m); err != nil {
					return err
				}
				*css = append(*css, ComposeServiceSecret{Source: m.Source, Target: m.Target})
			}
		}
	}
	return nil
}

// ComposeServiceConfig represents a service config mapping entry
type ComposeServiceConfig struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// ComposeServiceConfigs represents a custom type for unmarshaling service configs
type ComposeServiceConfigs []ComposeServiceConfig

// UnmarshalYAML implements custom decoding for ComposeServiceConfigs
func (csc *ComposeServiceConfigs) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*csc = []ComposeServiceConfig{{Source: s}}
	case yaml.SequenceNode:
		for _, node := range value.Content {
			switch node.Kind {
			case yaml.ScalarNode:
				var s string
				if err := node.Decode(&s); err != nil {
					return err
				}
				*csc = append(*csc, ComposeServiceConfig{Source: s})
			case yaml.MappingNode:
				var m struct {
					Source string `yaml:"source"`
					Target string `yaml:"target"`
				}
				if err := node.Decode(&m); err != nil {
					return err
				}
				*csc = append(*csc, ComposeServiceConfig{Source: m.Source, Target: m.Target})
			}
		}
	}
	return nil
}

// ComposeDeploy represents the deploy section in compose service
type ComposeDeploy struct {
	Resources ComposeResources `yaml:"resources"`
}

// ComposeResources represents resource constraints in deploy section
type ComposeResources struct {
	Limits       ComposeResourceLimits `yaml:"limits"`
	Reservations ComposeResourceLimits `yaml:"reservations"`
}

// ComposeResourceLimits represents memory and cpu constraints
type ComposeResourceLimits struct {
	Memory string `yaml:"memory"`
	CPUs   string `yaml:"cpus"`
}

// ComposeBuild represents the build configuration of a service
type ComposeBuild struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile"`
}

// UnmarshalYAML custom decodes ComposeBuild to handle string context or detailed mapping formats
func (cb *ComposeBuild) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		cb.Context = s
	case yaml.MappingNode:
		var m struct {
			Context    string `yaml:"context"`
			Dockerfile string `yaml:"dockerfile"`
		}
		if err := value.Decode(&m); err != nil {
			return err
		}
		cb.Context = m.Context
		cb.Dockerfile = m.Dockerfile
	}
	return nil
}

// ComposeService represents a service inside docker-compose.yml
type ComposeService struct {
	Environment   ComposeEnvironment    `yaml:"environment"`
	Ports         ComposePorts          `yaml:"ports"`
	Volumes       []ComposeVolume       `yaml:"volumes"`
	EnvFiles      ComposeEnvFiles       `yaml:"env_file"`
	Secrets       ComposeServiceSecrets `yaml:"secrets"`
	Configs       ComposeServiceConfigs `yaml:"configs"`
	Image         string                `yaml:"image"`
	ContainerName string                `yaml:"container_name"`
	Entrypoint    StringOrSlice         `yaml:"entrypoint"`
	Command       StringOrSlice         `yaml:"command"`
	Deploy        ComposeDeploy         `yaml:"deploy"`
	Build         ComposeBuild          `yaml:"build"`
}

// ComposeEnvironment is a custom map type to handle both string slice and map syntax for env vars
type ComposeEnvironment map[string]string

// UnmarshalYAML implements custom YAML decoding for environment variables
func (ce *ComposeEnvironment) UnmarshalYAML(value *yaml.Node) error {
	*ce = make(map[string]string)
	switch value.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(value.Content); i += 2 {
			keyNode := value.Content[i]
			valNode := value.Content[i+1]

			var key string
			if err := keyNode.Decode(&key); err != nil {
				return err
			}

			var val string
			if valNode.Tag == "!!null" {
				val = ""
			} else {
				if err := valNode.Decode(&val); err != nil {
					return err
				}
			}
			(*ce)[key] = val
		}
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
		if !strings.Contains(s, ":") {
			cv.Type = "volume"
			if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || s == "~" || s == "." {
				// Anonymous volume target
				cv.Source = ""
				cv.Target = s
			} else {
				// Named volume source
				cv.Source = s
				cv.Target = ""
			}
		} else if !strings.HasPrefix(cv.Source, "/") && !strings.HasPrefix(cv.Source, "./") && !strings.HasPrefix(cv.Source, "../") && cv.Source != "~" && cv.Source != "." && !isWindowsDrivePath(cv.Source) {
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
	defer func() { _ = file.Close() }()

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
		for i := range svc.EnvFiles {
			svc.EnvFiles[i].BaseDir = baseDir
		}
		config.Services[svcName] = svc
	}

	if config.Secrets == nil {
		config.Secrets = make(map[string]ComposeSecret)
	}
	for name, sec := range config.Secrets {
		sec.BaseDir = baseDir
		config.Secrets[name] = sec
	}

	if config.Configs == nil {
		config.Configs = make(map[string]ComposeConfigDef)
	}
	for name, cfg := range config.Configs {
		cfg.BaseDir = baseDir
		config.Configs[name] = cfg
	}

	return &config, nil
}

// MergeComposeConfigs merges multiple compose configurations from left to right.
// Latter configurations override or append to earlier ones according to Docker Compose rules.
func MergeComposeConfigs(configs ...*ComposeConfig) *ComposeConfig {
	merged := &ComposeConfig{
		Services: make(map[string]ComposeService),
		Volumes:  make(map[string]interface{}),
		Secrets:  make(map[string]ComposeSecret),
		Configs:  make(map[string]ComposeConfigDef),
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

			// EnvFiles: append list
			destSvc.EnvFiles = append(destSvc.EnvFiles, srcSvc.EnvFiles...)

			// Secrets: merge by Source, latter overrides former
			if len(srcSvc.Secrets) > 0 {
				secretMap := make(map[string]ComposeServiceSecret)
				for _, s := range destSvc.Secrets {
					secretMap[s.Source] = s
				}
				for _, s := range srcSvc.Secrets {
					secretMap[s.Source] = s
				}
				var mergedSecrets []ComposeServiceSecret
				for _, s := range secretMap {
					mergedSecrets = append(mergedSecrets, s)
				}
				sort.Slice(mergedSecrets, func(i, j int) bool {
					return mergedSecrets[i].Source < mergedSecrets[j].Source
				})
				destSvc.Secrets = mergedSecrets
			}

			// Configs: merge by Source, latter overrides former
			if len(srcSvc.Configs) > 0 {
				configMap := make(map[string]ComposeServiceConfig)
				for _, c := range destSvc.Configs {
					configMap[c.Source] = c
				}
				for _, c := range srcSvc.Configs {
					configMap[c.Source] = c
				}
				var mergedConfigs []ComposeServiceConfig
				for _, c := range configMap {
					mergedConfigs = append(mergedConfigs, c)
				}
				sort.Slice(mergedConfigs, func(i, j int) bool {
					return mergedConfigs[i].Source < mergedConfigs[j].Source
				})
				destSvc.Configs = mergedConfigs
			}

			// Volumes: latter overrides former if container target path is the same.
			// Anonymous volumes (empty Target) are always appended without de-duplication.
			srcTargets := make(map[string]bool)
			for _, v := range srcSvc.Volumes {
				if v.Target != "" {
					srcTargets[v.Target] = true
				}
			}
			var mergedVols []ComposeVolume
			for _, v := range destSvc.Volumes {
				if v.Target == "" || !srcTargets[v.Target] {
					mergedVols = append(mergedVols, v)
				}
			}
			destSvc.Volumes = append(mergedVols, srcSvc.Volumes...)

			if srcSvc.Deploy.Resources.Limits.Memory != "" || srcSvc.Deploy.Resources.Limits.CPUs != "" {
				destSvc.Deploy = srcSvc.Deploy
			}
			if srcSvc.Build.Context != "" {
				destSvc.Build = srcSvc.Build
			}

			merged.Services[svcName] = destSvc
		}

		// Merge root-level Volumes
		for volName, volDef := range config.Volumes {
			merged.Volumes[volName] = volDef
		}

		// Merge Secrets
		for secName, secDef := range config.Secrets {
			merged.Secrets[secName] = secDef
		}

		// Merge Configs
		for cfgName, cfgDef := range config.Configs {
			merged.Configs[cfgName] = cfgDef
		}
	}

	return merged
}
