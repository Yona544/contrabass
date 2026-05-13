//go:build windows

package timeline

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func (l *fileLock) lock() error {
	if err := windows.LockFileEx(windows.Handle(l.file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{}); err != nil {
		return fmt.Errorf("lock file %s: %w", l.path, err)
	}
	return nil
}

func (l *fileLock) unlock() error {
	defer l.file.Close()
	if err := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &windows.Overlapped{}); err != nil {
		return fmt.Errorf("unlock file %s: %w", l.path, err)
	}
	return nil
}
