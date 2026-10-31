// Package config loads and validates the sideline YAML config. Its shape
// deliberately tracks an existing sidebar tool's config so a file written for
// that tool translates by dropping it in place.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the whole config file.
type Config struct {
	Sidebar Sidebar                `yaml:"sidebar"`
	Theme   Theme                  `yaml:"theme"`
	Status  map[string]StatusStyle `yaml:"status"`
	Tasks   []TaskDecl             `yaml:"tasks"`
}

// Sidebar controls pane geometry and row layout.
type Sidebar struct {
	Position   string    `yaml:"position"`
	Width      int       `yaml:"width"`
	Height     int       `yaml:"height"`
	Layout     string    `yaml:"layout"`
	Sort       string    `yaml:"sort"`
	DimStale   bool      `yaml:"dim_stale"`
	StaleAfter int       `yaml:"stale_after"`
	Nerdfont   bool      `yaml:"nerdfont"`
	Templates  Templates `yaml:"templates"`
}

// Templates are tmux-style format templates handed to internal/format.
type Templates struct {
	Compact    string   `yaml:"compact"`
	Tiles      []string `yaml:"tiles"`
	Horizontal []string `yaml:"horizontal"`
}

// StatusStyle is the icon and colour for one task status.
type StatusStyle struct {
	Icon   string `yaml:"icon"`
	Colour string `yaml:"colour"`
}

// Theme selects the colour scheme.
type Theme struct {
	Scheme string            `yaml:"scheme"`
	Mode   string            `yaml:"mode"`
	Custom map[string]string `yaml:"custom"`
}

// TaskDecl is a task row declared directly in the config file.
type TaskDecl struct {
	ID     string `yaml:"id"`
	Title  string `yaml:"title"`
	Group  string `yaml:"group"`
	Status string `yaml:"status"`
	Note   string `yaml:"note"`
}

// Nerd Font glyphs for the default status styles. Blank icons would render a
// hole where the status column is, so the defaults always ship icons.
const (
	iconTodo    = "󰄱" // checkbox blank outline
	iconWorking = "󰅗" // loading spinner
	iconWaiting = "󰅑" // clock
	iconDone    = "󰄲" // checkbox marked outline
	iconBlocked = "󰅖" // alert octagon
)

// defaultStatus is the status map handed out by Default and filled in by
// Validate when the config file has no status block.
func defaultStatus() map[string]StatusStyle {
	return map[string]StatusStyle{
		"todo":    {Icon: iconTodo, Colour: "#7aa2f7"},
		"working": {Icon: iconWorking, Colour: "#7aa2f7"},
		"waiting": {Icon: iconWaiting, Colour: "#e0af68"},
		"done":    {Icon: iconDone, Colour: "#9ece6a"},
		"blocked": {Icon: iconBlocked, Colour: "#f7768e"},
	}
}

// Default returns a fully populated config, valid without any file present.
func Default() *Config {
	return &Config{
		Sidebar: Sidebar{
			Position:   "left",
			Width:      43,
			Height:     8,
			Layout:     "tiles",
			Sort:       "config",
			DimStale:   false,
			StaleAfter: 3600,
			Nerdfont:   true,
			Templates: Templates{
				Compact: "{status_icon} {title} {fill} {elapsed}",
				Tiles: []string{
					"{status_icon} #[bold]{title}#[default] {fill} {elapsed}",
					"{group} {fill} {git}",
					"{note}",
				},
				Horizontal: []string{
					"{status_icon} {title} {pane} {fill} {elapsed}",
					"{group} {fill} {note}",
				},
			},
		},
		Status: defaultStatus(),
	}
}

// Load reads the config at path. A missing file is not an error: the defaults
// come back so a sidebar works before the user has written anything. Malformed
// YAML or invalid values return a descriptive error.
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	// KnownFields turns a misspelled key into an error with its line number.
	// A silently ignored `postion:` or `task:` reads as "the setting does
	// nothing", which is the hardest kind of config bug to find.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

// Path resolves the default config file path: $SIDELINE_CONFIG, else
// $XDG_CONFIG_HOME/sideline/config.yaml, else $HOME/.config/sideline/config.yaml.
func Path() (string, error) {
	if p := os.Getenv("SIDELINE_CONFIG"); p != "" {
		return p, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "sideline", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve default path: %w", err)
	}
	return filepath.Join(home, ".config", "sideline", "config.yaml"), nil
}

// LoadDefault loads the config from Path, falling back to defaults when that
// file does not exist.
func LoadDefault() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return Load(p)
}

// Validate clamps loose values and rejects unknown ones. Unknown position or
// layout is an error rather than a silent default because both decide where
// and how the whole pane is drawn, and a typo would otherwise look like a bug.
func (c *Config) Validate() error {
	switch c.Sidebar.Position {
	case "left", "right", "top", "bottom":
	case "":
		c.Sidebar.Position = "left"
	default:
		return fmt.Errorf("config: unknown sidebar position %q (want left, right, top or bottom)", c.Sidebar.Position)
	}
	switch c.Sidebar.Layout {
	case "tiles", "compact":
	case "":
		c.Sidebar.Layout = "tiles"
	default:
		return fmt.Errorf("config: unknown sidebar layout %q (want tiles or compact)", c.Sidebar.Layout)
	}
	// Clamp rather than reject: a silly width still renders, a rejected one
	// does not render at all.
	if c.Sidebar.Width < 10 {
		c.Sidebar.Width = 10
	}
	if c.Sidebar.Width > 200 {
		c.Sidebar.Width = 200
	}
	if c.Sidebar.Height < 3 {
		c.Sidebar.Height = 3
	}
	if c.Sidebar.Height > 40 {
		c.Sidebar.Height = 40
	}
	if c.Status == nil {
		c.Status = map[string]StatusStyle{}
	}
	if len(c.Status) == 0 {
		c.Status = defaultStatus()
	}
	return nil
}

// IsVertical reports whether the sidebar sits on a side edge, where Width is
// the pane size, rather than top or bottom, where Height is.
func (c *Config) IsVertical() bool {
	return c.Sidebar.Position == "left" || c.Sidebar.Position == "right"
}

// FormatElapsed renders the age of ts as a compact coarse stamp: "", "42s",
// "7m", "3h" or "2d". A zero timestamp means "not set" and stays blank so
// unstarted tasks do not all claim an age.
func FormatElapsed(now, ts int64) string {
	if ts <= 0 {
		return ""
	}
	d := now - ts
	switch {
	case d < 0:
		// Clock skew or a future stamp: show zero seconds rather than nothing.
		return "0s"
	case d < 60:
		return fmt.Sprintf("%ds", d)
	case d < 3600:
		return fmt.Sprintf("%dm", d/60)
	case d < 86400:
		return fmt.Sprintf("%dh", d/3600)
	default:
		return fmt.Sprintf("%dd", d/86400)
	}
}
