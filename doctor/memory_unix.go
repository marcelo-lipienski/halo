//go:build !windows

package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// GetHostMemory returns total system memory in bytes.
func GetHostMemory() (uint64, error) {
	// Parse /proc/meminfo on Linux.
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, err := strconv.ParseUint(fields[1], 10, 64)
					if err == nil {
						return kb * 1024, nil
					}
				}
			}
		}
	}
	// Query sysctl on macOS.
	if cmd := exec.Command("sysctl", "-n", "hw.memsize"); cmd != nil {
		if out, err := cmd.Output(); err == nil {
			valStr := strings.TrimSpace(string(out))
			if val, err := strconv.ParseUint(valStr, 10, 64); err == nil {
				return val, nil
			}
		}
	}
	return 0, fmt.Errorf("unable to determine host memory")
}
