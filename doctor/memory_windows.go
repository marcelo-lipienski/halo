//go:build windows

package doctor

import (
	"syscall"
	"unsafe"
)

var (
	modKernel32Mem           = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modKernel32Mem.NewProc("GlobalMemoryStatusEx")
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

// GetHostMemory returns total system memory in bytes.
func GetHostMemory() (uint64, error) {
	var ms memoryStatusEx
	ms.dwLength = uint32(unsafe.Sizeof(ms))

	r, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r == 0 {
		return 0, err
	}
	return ms.ullTotalPhys, nil
}
