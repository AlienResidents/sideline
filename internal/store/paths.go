// Package store owns the on-disk task state file and the rules for
// changing it. The daemon holds one Store; daemonless CLI writers go
// through the same Apply path so the file never sees interleaved writes.
package store

import (
	"os"
	"path/filepath"
)

// Dir returns the sideline state directory. SIDELINE_STATE_DIR wins, then
// XDG_STATE_HOME, then the default under HOME.
func Dir() string {
	if d := os.Getenv("SIDELINE_STATE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "sideline")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "state", "sideline")
}

// StateFile returns the default state file path.
func StateFile() string {
	return filepath.Join(Dir(), "state.json")
}

// SocketPath returns the default daemon socket path.
func SocketPath() string {
	return filepath.Join(Dir(), "sideline.sock")
}

// LockPath returns the flock file guarding the state file at path.
func LockPath(path string) string {
	return path + ".lock"
}
