//go:build windows

package doctor

import (
	"syscall"
	"unsafe"
)

// GetFreeDiskSpace returns free disk space in bytes.
func GetFreeDiskSpace(path string) (uint64, error) {
	h := syscall.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GetDiskFreeSpaceExW")

	var freeBytes uint64
	var totalBytes uint64
	var totalFreeBytes uint64

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	r, _, err := c.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r == 0 {
		return 0, err
	}
	return freeBytes, nil
}
