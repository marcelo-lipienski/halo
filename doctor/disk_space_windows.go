//go:build windows

package doctor

import (
	"syscall"
	"unsafe"
)

var (
	modKernel32DiskFree     = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW = modKernel32DiskFree.NewProc("GetDiskFreeSpaceExW")
)

// GetFreeDiskSpace returns free disk space in bytes.
func GetFreeDiskSpace(path string) (uint64, error) {
	var freeBytes uint64
	var totalBytes uint64
	var totalFreeBytes uint64

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	r, _, err := procGetDiskFreeSpaceExW.Call(
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
