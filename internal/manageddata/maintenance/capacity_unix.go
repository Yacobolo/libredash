//go:build aix || darwin || dragonfly || freebsd || linux || openbsd

package maintenance

import (
	"math"

	"golang.org/x/sys/unix"
)

func filesystemFreeBytes(path string) (uint64, error) {
	var stats unix.Statfs_t
	if err := unix.Statfs(path, &stats); err != nil {
		return 0, err
	}
	if stats.Bavail <= 0 || stats.Bsize <= 0 {
		return 0, nil
	}
	available := uint64(stats.Bavail)
	blockSize := uint64(stats.Bsize)
	if blockSize != 0 && available > math.MaxUint64/blockSize {
		return math.MaxUint64, nil
	}
	return available * blockSize, nil
}
