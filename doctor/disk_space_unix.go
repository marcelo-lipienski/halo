//go:build !windows

package doctor

import (
	"syscall"
)

// GetFreeDiskSpace returns the free disk space in bytes in the directory specified by path
func GetFreeDiskSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
