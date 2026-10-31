package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// pidFile is the held lock on the daemon pid file. Closing it releases the
// lock; the OS does that even after a crash, so a stale file is harmless.
type pidFile struct {
	f    *os.File
	path string
}

// lockPID takes an exclusive lock on path and writes this process id. A second
// daemon on the same state directory fails with ErrRunning here, which covers
// the window before either process has a socket listener.
func lockPID(path string) (*pidFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open pid file %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, ErrRunning
	}
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("truncate pid file %s: %w", path, err)
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write pid file %s: %w", path, err)
	}
	return &pidFile{f: f, path: path}, nil
}

// Close releases the lock.
func (p *pidFile) Close() error {
	if p == nil || p.f == nil {
		return nil
	}
	return p.f.Close()
}

// ReadPID returns the pid recorded in path, or 0 when the file is missing or
// unreadable. It is a diagnostic, not a liveness check: only the lock proves
// the process is the daemon.
func ReadPID(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0
	}
	return pid
}
