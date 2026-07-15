package runtimeview

import (
	"context"
	"fmt"
	"os"
	"time"
)

const lockRetryInterval = 10 * time.Millisecond

type fileLock struct {
	file *os.File
}

func acquireFileLock(ctx context.Context, path string) (*fileLock, error) {
	return acquireLock(ctx, path, false)
}

func acquireSharedFileLock(ctx context.Context, path string) (*fileLock, error) {
	return acquireLock(ctx, path, true)
}

func acquireLock(ctx context.Context, path string, shared bool) (*fileLock, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	for {
		acquired, err := tryLockFile(file, shared)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if acquired {
			return &fileLock{file: file}, nil
		}
		timer := time.NewTimer(lockRetryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func tryAcquireFileLock(path string) (*fileLock, bool, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, false, err
	}
	acquired, err := tryLockFile(file, false)
	if err != nil {
		_ = file.Close()
		return nil, false, err
	}
	if !acquired {
		_ = file.Close()
		return nil, false, nil
	}
	return &fileLock{file: file}, true, nil
}

func openLockFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() || !pathInfo.Mode().IsRegular() || !os.SameFile(openedInfo, pathInfo) {
		_ = file.Close()
		return nil, fmt.Errorf("lock path is not a stable regular file")
	}
	return file, nil
}

func (l *fileLock) release() error {
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock file: %w", unlockErr)
	}
	return closeErr
}
