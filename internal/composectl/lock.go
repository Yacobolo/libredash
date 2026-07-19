package composectl

import (
	"fmt"
	"os"
	"path/filepath"
)

type controllerLock struct {
	file *os.File
}

func acquireControllerLock(path string) (*controllerLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
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
		return nil, fmt.Errorf("controller lock path is not a stable regular file")
	}
	acquired, err := tryControllerLock(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !acquired {
		_ = file.Close()
		return nil, fmt.Errorf("another LibreDash operation is running")
	}
	return &controllerLock{file: file}, nil
}

func (l *controllerLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	if err := unlockController(l.file); err != nil {
		_ = l.file.Close()
		return err
	}
	err := l.file.Close()
	l.file = nil
	return err
}
