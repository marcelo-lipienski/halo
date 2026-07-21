//go:build windows

package doctor

import (
	"syscall"
	"unsafe"
)

type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

// GetHostMemory returns the total system memory in bytes on Windows using Win32 API
func GetHostMemory() (uint64, error) {
	h := syscall.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GlobalMemoryStatusEx")

	var ms memoryStatusEx
	ms.dwLength = uint32(unsafe.Sizeof(ms))

	r, _, err := c.Call(uintptr(unsafe.Pointer(&ms)))
	if r == 0 {
		return 0, err
	}
	return ms.ullTotalPhys, nil
}
