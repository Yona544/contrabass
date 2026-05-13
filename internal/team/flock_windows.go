//go:build windows

package team

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// FileLock provides cross-process advisory locking using Windows byte-range locks.
// It is safe for concurrent use across OS processes accessing the same file.
type FileLock struct {
	path       string
	file       *os.File
	overlapped windows.Overlapped
}

// NewFileLock creates a lock backed by the given file path.
// The lock file is created if it doesn't exist.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path + ".lock"}
}

// Lock acquires an exclusive lock. Blocks until available.
func (l *FileLock) Lock() error {
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open lock file %s: %w", l.path, err)
	}

	handle := windows.Handle(f.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &l.overlapped); err != nil {
		_ = f.Close()
		l.file = nil
		return fmt.Errorf("lock file %s: %w", l.path, err)
	}

	l.file = f
	return nil
}

// Unlock releases the lock.
func (l *FileLock) Unlock() error {
	if l.file == nil {
		return nil
	}

	handle := windows.Handle(l.file.Fd())
	if err := windows.UnlockFileEx(handle, 0, 1, 0, &l.overlapped); err != nil {
		_ = l.file.Close()
		l.file = nil
		return fmt.Errorf("unlock file %s: %w", l.path, err)
	}

	err := l.file.Close()
	l.file = nil
	if err != nil {
		return fmt.Errorf("close lock file %s: %w", l.path, err)
	}

	return nil
}
