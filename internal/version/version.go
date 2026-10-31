// Package version carries the release version string.
//
// The value is embedded from the VERSION file in this directory, so a plain
// `go build`, a `go install ...@latest`, and `make` all report the same
// thing. Bumping the release means editing that one file.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// Value is the release version, without surrounding whitespace.
var Value = strings.TrimSpace(raw)
