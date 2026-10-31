package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Sidebar.Position != "left" || c.Sidebar.Width != 43 || c.Sidebar.Height != 8 {
		t.Fatalf("sidebar defaults wrong: %+v", c.Sidebar)
	}
	if c.Sidebar.Layout != "tiles" || c.Sidebar.Sort != "config" {
		t.Fatalf("layout/sort defaults wrong: %+v", c.Sidebar)
	}
	if c.Sidebar.DimStale || c.Sidebar.StaleAfter != 3600 || !c.Sidebar.Nerdfont {
		t.Fatalf("stale defaults wrong: %+v", c.Sidebar)
	}
	if c.Sidebar.Templates.Compact == "" || len(c.Sidebar.Templates.Tiles) != 3 {
		t.Fatalf("template defaults wrong: %+v", c.Sidebar.Templates)
	}
	for _, s := range []string{"todo", "working", "waiting", "done", "blocked"} {
		st, ok := c.Status[s]
		if !ok {
			t.Fatalf("default status missing %q", s)
		}
		if st.Icon == "" || st.Colour == "" {
			t.Fatalf("default status %q has blank icon or colour", s)
		}
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if c.Sidebar.Width != 43 {
		t.Fatalf("missing file should return defaults, got width %d", c.Sidebar.Width)
	}
}

func TestLoadMalformed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("sidebar: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("malformed yaml should error")
	}
}

func TestExampleConfigLoads(t *testing.T) {
	// The shipped example must parse under KnownFields; otherwise a new
	// user's first copy fails and the project looks broken.
	cfg, err := Load(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatalf("example config: %v", err)
	}
	if len(cfg.Tasks) == 0 || cfg.Sidebar.Position != "left" {
		t.Fatalf("example config parsed wrong: %+v", cfg.Sidebar)
	}
}

func TestLoadUnknownKeyFails(t *testing.T) {
	// A misspelled key must not be silently ignored: it looks like the
	// setting works and does nothing.
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("sidebar:\n  postion: right\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil {
		t.Fatal("unknown key accepted")
	}
	if !strings.Contains(err.Error(), "postion") {
		t.Fatalf("error does not name the key: %v", err)
	}
}

func TestLoadInvalidPosition(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	y := "sidebar:\n  position: sideways\n"
	if err := os.WriteFile(p, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("unknown position should error")
	}
}

func TestLoadFull(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	y := `
sidebar:
  position: right
  width: 500
  height: 1
  layout: compact
theme:
  scheme: custom
  custom:
    text: "#ffffff"
status:
  todo:
    icon: "o"
    colour: "#7aa2f7"
tasks:
  - id: webapp
    title: Fix login
`
	if err := os.WriteFile(p, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Sidebar.Position != "right" || !c.IsVertical() {
		t.Fatalf("position: %+v", c.Sidebar)
	}
	// Clamped, not rejected.
	if c.Sidebar.Width != 200 || c.Sidebar.Height != 3 {
		t.Fatalf("clamp: width %d height %d", c.Sidebar.Width, c.Sidebar.Height)
	}
	if c.Theme.Custom["text"] != "#ffffff" {
		t.Fatalf("theme custom: %+v", c.Theme)
	}
	if c.Status["todo"].Icon != "o" {
		t.Fatalf("status: %+v", c.Status)
	}
	if len(c.Tasks) != 1 || c.Tasks[0].ID != "webapp" {
		t.Fatalf("tasks: %+v", c.Tasks)
	}
}

func TestValidateEmptyStatus(t *testing.T) {
	c := Default()
	c.Status = nil
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Status["todo"].Colour == "" {
		t.Fatal("empty status map should get defaults")
	}
}

func TestValidateUnknownLayout(t *testing.T) {
	c := Default()
	c.Sidebar.Layout = "fancy"
	if err := c.Validate(); err == nil {
		t.Fatal("unknown layout should error")
	}
}

func TestIsVertical(t *testing.T) {
	c := Default()
	for pos, want := range map[string]bool{"left": true, "right": true, "top": false, "bottom": false} {
		c.Sidebar.Position = pos
		if c.IsVertical() != want {
			t.Fatalf("IsVertical(%q) = %v", pos, c.IsVertical())
		}
	}
}

func TestPath(t *testing.T) {
	t.Setenv("SIDELINE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home(t), ".config", "sideline", "config.yaml"); p != want {
		t.Fatalf("Path() = %q, want %q", p, want)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if p, _ = Path(); p != "/xdg/sideline/config.yaml" {
		t.Fatalf("XDG path = %q", p)
	}
	t.Setenv("SIDELINE_CONFIG", "/tmp/sideline.yaml")
	if p, _ = Path(); p != "/tmp/sideline.yaml" {
		t.Fatalf("env path = %q", p)
	}
}

func TestLoadDefault(t *testing.T) {
	t.Setenv("SIDELINE_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))
	c, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if c.Sidebar.Width != 43 {
		t.Fatal("LoadDefault should fall back to defaults")
	}
}

func home(t *testing.T) string {
	t.Helper()
	h, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no HOME")
	}
	return h
}

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		now, ts int64
		want    string
	}{
		{100, 0, ""},
		{100, 100, "0s"},
		{100, 58, "42s"},
		{100, 40, "1m"},
		{3600, 300, "55m"},
		{86400, 3600, "23h"},
		{90000, 100, "1d"},
		{200000, 100, "2d"},
		{100000, 0, ""},
	}
	for _, tc := range tests {
		if got := FormatElapsed(tc.now, tc.ts); got != tc.want {
		}
	}
}
