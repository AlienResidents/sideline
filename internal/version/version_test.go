package version

import (
	"strconv"
	"strings"
	"testing"
)

func TestValueIsSemver(t *testing.T) {
	// The embedded file is the only source, so a malformed edit must fail a
	// test rather than ship a version that tools cannot compare.
	if Value == "" {
		t.Fatal("empty version")
	}
	parts := strings.Split(Value, ".")
	if len(parts) != 3 {
		t.Fatalf("version %q is not major.minor.patch", Value)
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			t.Fatalf("version %q has a non-numeric part %q", Value, p)
		}
	}
}

func TestMajorIsZero(t *testing.T) {
	// The major stays 0 while the config, CLI and state file can change.
	if !strings.HasPrefix(Value, "0.") {
		t.Fatalf("version %q left major 0 before the contract was frozen", Value)
	}
}
