//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !openbsd && !windows

package maintenance

import "errors"

func filesystemFreeBytes(string) (uint64, error) {
	return 0, errors.New("filesystem capacity inspection is unsupported")
}
