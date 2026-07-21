//go:build !windows

package doctor

import (
	"syscall"
)

// GetFreeDiskSpace returns free disk space in bytes.
func GetFreeDiskSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
