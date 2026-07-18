package config

import (
	"bufio"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseEnv parses a .env file and returns a map of keys to values.
func ParseEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines or comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip surrounding quotes
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}
		env[key] = val
	}
	return env, scanner.Err()
}

// ComposeConfig represents the root of docker-compose.yml
type ComposeConfig struct {
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]interface{}    `yaml:"volumes"`
}

// ComposeService represents a service inside docker-compose.yml
type ComposeService struct {
	Environment ComposeEnvironment `yaml:"environment"`
	Ports       []string           `yaml:"ports"`
	Volumes     []ComposeVolume    `yaml:"volumes"`
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
	Source string
	Target string
	Type   string // "bind" or "volume"
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
		if len(parts) > 0 {
			cv.Source = parts[0]
		}
		if len(parts) > 1 {
			cv.Target = parts[1]
		}
		cv.Type = "bind"
		// If Source is a simple name (doesn't start with path characters), assume it is a named volume
		if !strings.HasPrefix(cv.Source, "/") && !strings.HasPrefix(cv.Source, "./") && !strings.HasPrefix(cv.Source, "../") && cv.Source != "~" && cv.Source != "." {
			cv.Type = "volume"
		}
	case yaml.MappingNode:
		var m struct {
			Type   string `yaml:"type"`
			Source string `yaml:"source"`
			Target string `yaml:"target"`
		}
		if err := value.Decode(&m); err != nil {
			return err
		}
		cv.Source = m.Source
		cv.Target = m.Target
		cv.Type = m.Type
		if cv.Type == "" {
			cv.Type = "bind"
			if !strings.HasPrefix(cv.Source, "/") && !strings.HasPrefix(cv.Source, "./") && !strings.HasPrefix(cv.Source, "../") && cv.Source != "~" && cv.Source != "." {
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
	return &config, nil
}
