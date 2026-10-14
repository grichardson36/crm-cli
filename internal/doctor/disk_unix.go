//go:build linux || darwin

package doctor

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func freeBytes(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("free space: %w", err)
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
