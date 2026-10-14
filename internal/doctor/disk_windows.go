//go:build windows

package doctor

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func freeBytes(dir string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, fmt.Errorf("free space: %w", err)
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return 0, fmt.Errorf("free space: %w", err)
	}
	return avail, nil
}
