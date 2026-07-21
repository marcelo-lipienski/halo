package init_cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type EnvEntry struct {
	Key           string
	Value         string
	IsPlaceholder bool
}

type Result struct {
	Added          []EnvEntry
	AlreadyPresent int
}

func IsPlaceholder(value string) bool {
	val := strings.TrimSpace(value)
	if val == "" {
		return true
	}
	lower := strings.ToLower(val)
	if lower == "changeme" || lower == "todo" || lower == "replace_me" {
		return true
	}
	if strings.HasPrefix(lower, "your_") {
		return true
	}
	if strings.HasPrefix(val, "<") && strings.HasSuffix(val, ">") {
		return true
	}
	return false
}

// MergeEnvFiles merges env keys from example to target. See ADR-0002.
func MergeEnvFiles(examplePath, targetPath string, dryRun bool) (Result, error) {
	var result Result

	exampleLines, err := os.ReadFile(examplePath)
	if err != nil {
		return result, fmt.Errorf("failed to read example file: %w", err)
	}

	targetExists := true
	existingKeys := make(map[string]bool)
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		targetExists = false
	} else if err != nil {
		return result, fmt.Errorf("failed to stat target file: %w", err)
	}

	if targetExists {
		envMap, err := godotenv.Read(targetPath)
		if err != nil {
			return result, fmt.Errorf("failed to read target env file: %w", err)
		}
		for k := range envMap {
			existingKeys[k] = true
			result.AlreadyPresent++
		}
	}

	lines := strings.Split(string(exampleLines), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var linesToAdd []string
	var addedEntries []EnvEntry

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if !targetExists {
				linesToAdd = append(linesToAdd, line)
			}
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 0 {
			if !targetExists {
				linesToAdd = append(linesToAdd, line)
			}
			continue
		}

		key := strings.TrimSpace(parts[0])
		if strings.HasPrefix(key, "export ") {
			key = strings.TrimPrefix(key, "export ")
			key = strings.TrimSpace(key)
		}

		val := ""
		if len(parts) > 1 {
			val = strings.TrimSpace(parts[1])
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) || (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				if len(val) >= 2 {
					val = val[1 : len(val)-1]
				}
			}
		}

		if key == "" {
			if !targetExists {
				linesToAdd = append(linesToAdd, line)
			}
			continue
		}

		if existingKeys[key] {
			if !targetExists {
				linesToAdd = append(linesToAdd, line)
			}
			continue
		}

		linesToAdd = append(linesToAdd, line)
		existingKeys[key] = true

		addedEntries = append(addedEntries, EnvEntry{
			Key:           key,
			Value:         val,
			IsPlaceholder: IsPlaceholder(val),
		})
	}

	result.Added = addedEntries

	if (len(linesToAdd) > 0 || !targetExists) && !dryRun {
		var out *os.File
		if targetExists {
			if len(linesToAdd) > 0 {
				out, err = os.OpenFile(targetPath, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return result, err
				}
				defer func() {
					_ = out.Close()
				}()

				info, err := os.Stat(targetPath)
				if err == nil && info.Size() > 0 {
					f, err := os.Open(targetPath)
					if err == nil {
						if _, errSeek := f.Seek(-1, io.SeekEnd); errSeek == nil {
							b := make([]byte, 1)
							if _, errRead := f.Read(b); errRead == nil {
								if b[0] != '\n' {
									_, _ = out.WriteString("\n")
								}
							}
						}
						_ = f.Close()
					}
				}

				_, _ = out.WriteString("\n# Added by halo init\n")
				for _, l := range linesToAdd {
					_, _ = out.WriteString(l + "\n")
				}
			}
		} else {
			out, err = os.Create(targetPath)
			if err != nil {
				return result, err
			}
			defer func() {
				_ = out.Close()
			}()
			for _, l := range linesToAdd {
				_, _ = out.WriteString(l + "\n")
			}
		}
	}

	return result, nil
}
