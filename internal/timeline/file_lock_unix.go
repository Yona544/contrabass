//go:build !windows

package timeline

import "syscall"

func (l *fileLock) lock() error {
	return syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX)
}

func (l *fileLock) unlock() error {
	defer l.file.Close()
	return syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
}
