package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// EnabledPath returns the enabled-flag path for state dir d. The flag lives
// in a sibling file so the daemon can find it without a Store.
func EnabledPath(d string) string {
	return filepath.Join(d, "enabled")
}

// IsEnabled reports whether the enabled flag file exists and holds a true
// value. A missing file means false: an unset flag must never resurrect a
// sidebar after a reboot or a cleared state dir.
func IsEnabled(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	v, err := strconv.ParseBool(strings.TrimSpace(string(b)))
	if err != nil {
		// A corrupt flag file is read as disabled; the next Enable
		// rewrites it.
		return false, nil
	}
	return v, nil
}

// SetEnabled writes the enabled flag file: a "1"/"0" byte plus newline. The
// file is removed rather than emptied on disable so a plain existence check
// also works.
func SetEnabled(path string, on bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove enabled flag: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(path, []byte("1\n"), 0o600); err != nil {
		return fmt.Errorf("write enabled flag: %w", err)
	}
	return nil
}
