package instancelock

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Yacobolo/libredash/internal/securefs"
)

const FileName = ".instance.lock"

type Lock struct {
	file *os.File
}

func Acquire(home string) (*Lock, error) {
	if err := securefs.EnsurePrivateDir(home); err != nil {
		return nil, err
	}
	path := filepath.Join(home, FileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, securefs.PrivateFileMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(securefs.PrivateFileMode); err != nil {
		_ = file.Close()
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !opened.Mode().IsRegular() || !pathInfo.Mode().IsRegular() || !os.SameFile(opened, pathInfo) {
		_ = file.Close()
		return nil, fmt.Errorf("instance lock path is not a stable regular file")
	}
	acquired, err := tryLock(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !acquired {
		_ = file.Close()
		return nil, fmt.Errorf("another LibreDash process is already using LIBREDASH_HOME %q", home)
	}
	return &Lock{file: file}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	if err := unlock(l.file); err != nil {
		_ = l.file.Close()
		return err
	}
	err := l.file.Close()
	l.file = nil
	return err
}
