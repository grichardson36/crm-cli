//go:build !windows && !linux && !darwin

package doctor

import "errors"

func freeBytes(string) (uint64, error) {
	return 0, errors.New("free space is not supported on this system")
}
