package render

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/AlienResidents/sideline/internal/config"
)

// writeConfig writes a config with the given width and a distinct mtime, so a
// reload cannot mistake it for an unchanged file.
func writeConfig(t *testing.T, path string, width int, mtime time.Time) {
	t.Helper()
	body := "sidebar:\n  width: " + strconv.Itoa(width) + "\n  auto_reload: false\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestReloadConfigHonoursAutoReloadOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	base := time.Now().Add(-time.Hour)
	writeConfig(t, path, 40, base)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{cfg: cfg, pal: palette(cfg), cfgPath: path, cfgMod: base}

	writeConfig(t, path, 60, base.Add(time.Minute))
	m.reloadConfig(false)
	if m.cfg.Sidebar.Width != 40 {
		t.Fatalf("auto_reload off still reloaded: width %d", m.cfg.Sidebar.Width)
	}

	// A forced reload is the `sideline reload` path and must apply.
	m.reloadConfig(true)
	if m.cfg.Sidebar.Width != 60 {
		t.Fatalf("forced reload ignored: width %d", m.cfg.Sidebar.Width)
	}
}

func TestReloadConfigAppliesWhenAutoReloadOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	base := time.Now().Add(-time.Hour)
	body := "sidebar:\n  width: 40\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, base, base); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{cfg: cfg, pal: palette(cfg), cfgPath: path, cfgMod: base}

	if err := os.WriteFile(path, []byte("sidebar:\n  width: 60\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	m.reloadConfig(false)
	if m.cfg.Sidebar.Width != 60 {
		t.Fatalf("auto_reload on did not apply: width %d", m.cfg.Sidebar.Width)
	}
}

func TestReloadConfigSkipsInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	base := time.Now().Add(-time.Hour)
	writeConfig(t, path, 40, base)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{cfg: cfg, pal: palette(cfg), cfgPath: path, cfgMod: base}

	if err := os.WriteFile(path, []byte("sidebar:\n  width: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	m.reloadConfig(true)
	if m.cfg.Sidebar.Width != 40 {
		t.Fatalf("invalid config replaced the good one: width %d", m.cfg.Sidebar.Width)
	}
}
