//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package runtimeview

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockFile(file *os.File, shared bool) (bool, error) {
	operation := unix.LOCK_EX
	if shared {
		operation = unix.LOCK_SH
	}
	err := unix.Flock(int(file.Fd()), operation|unix.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
