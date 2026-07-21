package snapshot

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

type FileDiff struct {
	Path    string `json:"path"`
	Change  string `json:"change"` // "added", "removed", "modified"
	OldSize int64  `json:"old_size,omitempty"`
	NewSize int64  `json:"new_size,omitempty"`
	OldHash string `json:"old_hash,omitempty"`
	NewHash string `json:"new_hash,omitempty"`
}

type VarDiff struct {
	File     string `json:"file"`
	Key      string `json:"key"`
	Change   string `json:"change"` // "added", "removed", "modified"
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

type PortDiff struct {
	Service        string `json:"service"`
	Port           string `json:"port"`
	Protocol       string `json:"protocol"`
	Change         string `json:"change"` // "added", "removed", "status_changed"
	OldOccupied    bool   `json:"old_occupied"`
	NewOccupied    bool   `json:"new_occupied"`
	OldProcessName string `json:"old_process_name,omitempty"`
	NewProcessName string `json:"new_process_name,omitempty"`
	OldPID         int    `json:"old_pid,omitempty"`
	NewPID         int    `json:"new_pid,omitempty"`
}

type ContainerDiff struct {
	Service    string `json:"service"`
	Change     string `json:"change"` // "added", "removed", "state_changed", "image_changed", "health_changed"
	OldState   string `json:"old_state,omitempty"`
	NewState   string `json:"new_state,omitempty"`
	OldStatus  string `json:"old_status,omitempty"`
	NewStatus  string `json:"new_status,omitempty"`
	OldImageID string `json:"old_image_id,omitempty"`
	NewImageID string `json:"new_image_id,omitempty"`
	OldImage   string `json:"old_image,omitempty"`
	NewImage   string `json:"new_image,omitempty"`
}

type EnvironmentDiff struct {
	Project    string          `json:"project"`
	Files      []FileDiff      `json:"files,omitempty"`
	Variables  []VarDiff       `json:"variables,omitempty"`
	Ports      []PortDiff      `json:"ports,omitempty"`
	Containers []ContainerDiff `json:"containers,omitempty"`
}

// Diff compares old and new snapshots
func Diff(old, new *EnvironmentSnapshot) *EnvironmentDiff {
	diff := &EnvironmentDiff{
		Project: new.Project,
	}

	// 1. Diff Files
	allFiles := make(map[string]bool)
	for k := range old.Files {
		allFiles[k] = true
	}
	for k := range new.Files {
		allFiles[k] = true
	}
	
	var sortedFiles []string
	for k := range allFiles {
		sortedFiles = append(sortedFiles, k)
	}
	sort.Strings(sortedFiles)

	for _, path := range sortedFiles {
		oldF, inOld := old.Files[path]
		newF, inNew := new.Files[path]

		if inOld && !inNew {
			diff.Files = append(diff.Files, FileDiff{
				Path:    path,
				Change:  "removed",
				OldSize: oldF.Size,
				OldHash: oldF.Hash,
			})
		} else if !inOld && inNew {
			diff.Files = append(diff.Files, FileDiff{
				Path:    path,
				Change:  "added",
				NewSize: newF.Size,
				NewHash: newF.Hash,
			})
		} else if inOld && inNew {
			if oldF.Hash != newF.Hash {
				diff.Files = append(diff.Files, FileDiff{
					Path:    path,
					Change:  "modified",
					OldSize: oldF.Size,
					NewSize: newF.Size,
					OldHash: oldF.Hash,
					NewHash: newF.Hash,
				})
			}
		}
	}

	// 2. Diff Variables
	allVarFiles := make(map[string]bool)
	for k := range old.Variables {
		allVarFiles[k] = true
	}
	for k := range new.Variables {
		allVarFiles[k] = true
	}
	
	var sortedVarFiles []string
	for k := range allVarFiles {
		sortedVarFiles = append(sortedVarFiles, k)
	}
	sort.Strings(sortedVarFiles)

	for _, file := range sortedVarFiles {
		oldVars := old.Variables[file]
		newVars := new.Variables[file]

		allKeys := make(map[string]bool)
		for k := range oldVars {
			allKeys[k] = true
		}
		for k := range newVars {
			allKeys[k] = true
		}

		var sortedKeys []string
		for k := range allKeys {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)

		for _, key := range sortedKeys {
			oldVal, inOld := oldVars[key]
			newVal, inNew := newVars[key]

			if inOld && !inNew {
				diff.Variables = append(diff.Variables, VarDiff{
					File:     file,
					Key:      key,
					Change:   "removed",
					OldValue: oldVal,
				})
			} else if !inOld && inNew {
				diff.Variables = append(diff.Variables, VarDiff{
					File:     file,
					Key:      key,
					Change:   "added",
					NewValue: newVal,
				})
			} else if inOld && inNew {
				if oldVal != newVal {
					diff.Variables = append(diff.Variables, VarDiff{
						File:     file,
						Key:      key,
						Change:   "modified",
						OldValue: oldVal,
						NewValue: newVal,
					})
				}
			}
		}
	}

	// 3. Diff Ports
	type portKey struct {
		service  string
		port     string
		protocol string
	}
	
	oldPorts := make(map[portKey]PortSnapshot)
	for _, p := range old.Ports {
		oldPorts[portKey{service: p.Service, port: p.HostPort, protocol: p.Protocol}] = p
	}
	
	newPorts := make(map[portKey]PortSnapshot)
	for _, p := range new.Ports {
		newPorts[portKey{service: p.Service, port: p.HostPort, protocol: p.Protocol}] = p
	}

	allPortKeys := make(map[portKey]bool)
	for k := range oldPorts {
		allPortKeys[k] = true
	}
	for k := range newPorts {
		allPortKeys[k] = true
	}

	var sortedPortKeys []portKey
	for k := range allPortKeys {
		sortedPortKeys = append(sortedPortKeys, k)
	}
	// Sort ports deterministically by service, then port, then protocol
	sort.Slice(sortedPortKeys, func(i, j int) bool {
		if sortedPortKeys[i].service != sortedPortKeys[j].service {
			return sortedPortKeys[i].service < sortedPortKeys[j].service
		}
		if sortedPortKeys[i].port != sortedPortKeys[j].port {
			return sortedPortKeys[i].port < sortedPortKeys[j].port
		}
		return sortedPortKeys[i].protocol < sortedPortKeys[j].protocol
	})

	for _, k := range sortedPortKeys {
		oldP, inOld := oldPorts[k]
		newP, inNew := newPorts[k]

		if inOld && !inNew {
			diff.Ports = append(diff.Ports, PortDiff{
				Service:        k.service,
				Port:           k.port,
				Protocol:       k.protocol,
				Change:         "removed",
				OldOccupied:    oldP.IsOccupied,
				OldProcessName: oldP.ProcessName,
				OldPID:         oldP.PID,
			})
		} else if !inOld && inNew {
			diff.Ports = append(diff.Ports, PortDiff{
				Service:        k.service,
				Port:           k.port,
				Protocol:       k.protocol,
				Change:         "added",
				NewOccupied:    newP.IsOccupied,
				NewProcessName: newP.ProcessName,
				NewPID:         newP.PID,
			})
		} else if inOld && inNew {
			if oldP.IsOccupied != newP.IsOccupied || oldP.ProcessName != newP.ProcessName || oldP.PID != newP.PID {
				diff.Ports = append(diff.Ports, PortDiff{
					Service:        k.service,
					Port:           k.port,
					Protocol:       k.protocol,
					Change:         "status_changed",
					OldOccupied:    oldP.IsOccupied,
					NewOccupied:    newP.IsOccupied,
					OldProcessName: oldP.ProcessName,
					NewProcessName: newP.ProcessName,
					OldPID:         oldP.PID,
					NewPID:         newP.PID,
				})
			}
		}
	}

	// 4. Diff Services & Containers
	allServices := make(map[string]bool)
	for k := range old.Services {
		allServices[k] = true
	}
	for k := range new.Services {
		allServices[k] = true
	}

	var sortedServices []string
	for k := range allServices {
		sortedServices = append(sortedServices, k)
	}
	sort.Strings(sortedServices)

	for _, svc := range sortedServices {
		oldS, inOld := old.Services[svc]
		newS, inNew := new.Services[svc]

		if inOld && !inNew {
			diff.Containers = append(diff.Containers, ContainerDiff{
				Service:    svc,
				Change:     "removed",
				OldState:   oldS.State,
				OldStatus:  oldS.Status,
				OldImage:   oldS.Image,
				OldImageID: oldS.ImageID,
			})
		} else if !inOld && inNew {
			diff.Containers = append(diff.Containers, ContainerDiff{
				Service:    svc,
				Change:     "added",
				NewState:   newS.State,
				NewStatus:  newS.Status,
				NewImage:   newS.Image,
				NewImageID: newS.ImageID,
			})
		} else if inOld && inNew {
			stateChanged := oldS.State != newS.State
			healthChanged := oldS.Status != newS.Status
			imageChanged := oldS.ImageID != newS.ImageID || oldS.Image != newS.Image

			if stateChanged || healthChanged || imageChanged {
				change := "modified"
				if stateChanged {
					change = "state_changed"
				} else if healthChanged {
					change = "health_changed"
				} else if imageChanged {
					change = "image_changed"
				}

				diff.Containers = append(diff.Containers, ContainerDiff{
					Service:    svc,
					Change:     change,
					OldState:   oldS.State,
					NewState:   newS.State,
					OldStatus:  oldS.Status,
					NewStatus:  newS.Status,
					OldImage:   oldS.Image,
					NewImage:   newS.Image,
					OldImageID: oldS.ImageID,
					NewImageID: newS.ImageID,
				})
			}
		}
	}

	return diff
}

// Helper to determine if color output is enabled
func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func colorize(s, ansi string, color bool) string {
	if !color {
		return s
	}
	return "\033[" + ansi + "m" + s + "\033[0m"
}

// RenderText writes the diff report in human-readable text format
func RenderText(w io.Writer, diff *EnvironmentDiff, oldCreatedAt time.Time) {
	color := useColor(w)

	fmt.Fprintln(w, "=== halo Snapshot Diff ===")
	fmt.Fprintf(w, "Comparing current state against snapshot taken at: %s\n", oldCreatedAt.Format(time.RFC3339))
	
	hasDiffs := len(diff.Files) > 0 || len(diff.Variables) > 0 || len(diff.Ports) > 0 || len(diff.Containers) > 0
	if !hasDiffs {
		fmt.Fprintln(w)
		fmt.Fprintln(w, colorize("✓ Environment matches snapshot exactly. No changes detected.", "32", color))
		return
	}

	// 1. Files Diff
	if len(diff.Files) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "[Configuration Files]")
		for _, f := range diff.Files {
			switch f.Change {
			case "added":
				fmt.Fprintf(w, "  %s %s (added)\n", colorize("+", "32", color), f.Path)
			case "removed":
				fmt.Fprintf(w, "  %s %s (removed)\n", colorize("-", "31", color), f.Path)
			case "modified":
				fmt.Fprintf(w, "  %s %s (modified)\n", colorize("~", "33", color), f.Path)
			}
		}
	}

	// 2. Variables Diff
	if len(diff.Variables) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "[Environment Variables]")
		
		// Group by file
		byFile := make(map[string][]VarDiff)
		for _, v := range diff.Variables {
			byFile[v.File] = append(byFile[v.File], v)
		}
		
		var sortedFiles []string
		for f := range byFile {
			sortedFiles = append(sortedFiles, f)
		}
		sort.Strings(sortedFiles)

		for _, file := range sortedFiles {
			fmt.Fprintf(w, "  In %s:\n", file)
			for _, v := range byFile[file] {
				switch v.Change {
				case "added":
					fmt.Fprintf(w, "    %s %s: added (value: %q)\n", colorize("+", "32", color), v.Key, v.NewValue)
				case "removed":
					fmt.Fprintf(w, "    %s %s: removed (old value: %q)\n", colorize("-", "31", color), v.Key, v.OldValue)
				case "modified":
					fmt.Fprintf(w, "    %s %s: modified (%q -> %q)\n", colorize("~", "33", color), v.Key, v.OldValue, v.NewValue)
				}
			}
		}
	}

	// 3. Services / Containers Diff
	if len(diff.Containers) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "[Services & Containers]")
		for _, c := range diff.Containers {
			switch c.Change {
			case "added":
				fmt.Fprintf(w, "  %s Service %s: container added (state: %s, image: %s)\n", colorize("+", "32", color), c.Service, c.NewState, c.NewImage)
			case "removed":
				fmt.Fprintf(w, "  %s Service %s: container removed (was state: %s, image: %s)\n", colorize("-", "31", color), c.Service, c.OldState, c.OldImage)
			default:
				fmt.Fprintf(w, "  %s Service %s: container modified\n", colorize("~", "33", color), c.Service)
				if c.OldState != c.NewState {
					fmt.Fprintf(w, "    State:  %s -> %s\n", c.OldState, c.NewState)
				}
				if c.OldStatus != c.NewStatus {
					oldStat := c.OldStatus
					if oldStat == "" {
						oldStat = "none"
					}
					newStat := c.NewStatus
					if newStat == "" {
						newStat = "none"
					}
					fmt.Fprintf(w, "    Health: %s -> %s\n", oldStat, newStat)
				}
				if c.OldImageID != c.NewImageID {
					fmt.Fprintf(w, "    Image:  %s -> %s\n", c.OldImage, c.NewImage)
					fmt.Fprintf(w, "    ID:     %s -> %s\n", c.OldImageID, c.NewImageID)
				}
			}
		}
	}

	// 4. Ports Diff
	if len(diff.Ports) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "[Ports]")
		for _, p := range diff.Ports {
			switch p.Change {
			case "added":
				statusStr := "free"
				if p.NewOccupied {
					statusStr = "occupied"
					if p.NewProcessName != "" {
						statusStr += fmt.Sprintf(" by %s (PID %d)", p.NewProcessName, p.NewPID)
					}
				}
				fmt.Fprintf(w, "  %s Service %s, Port %s (%s): mapping added (status: %s)\n", colorize("+", "32", color), p.Service, p.Port, p.Protocol, statusStr)
			case "removed":
				fmt.Fprintf(w, "  %s Service %s, Port %s (%s): mapping removed\n", colorize("-", "31", color), p.Service, p.Port, p.Protocol)
			case "status_changed":
				oldStatus := "free"
				if p.OldOccupied {
					oldStatus = "occupied"
					if p.OldProcessName != "" {
						oldStatus += fmt.Sprintf(" by %s (PID %d)", p.OldProcessName, p.OldPID)
					}
				}
				newStatus := "free"
				if p.NewOccupied {
					newStatus = "occupied"
					if p.NewProcessName != "" {
						newStatus += fmt.Sprintf(" by %s (PID %d)", p.NewProcessName, p.NewPID)
					}
				}
				fmt.Fprintf(w, "  %s Service %s, Port %s (%s): %s -> %s\n", colorize("~", "33", color), p.Service, p.Port, p.Protocol, oldStatus, newStatus)
			}
		}
	}

	fmt.Fprintln(w)
}
