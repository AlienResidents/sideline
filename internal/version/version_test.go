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

func TestValueIsPlainSemver(t *testing.T) {
	// Release values are bare: a leading v or a pre-release suffix would
	// break the comparison `sideline status` makes against the daemon.
	if strings.HasPrefix(Value, "v") || strings.Contains(Value, "-") || strings.Contains(Value, "+") {
		t.Fatalf("version %q is not a plain release value", Value)
	}
}
