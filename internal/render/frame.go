package render

import (
	"strconv"
	"strings"
	"time"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/format"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/theme"
	"github.com/AlienResidents/sideline/internal/tmux"
)

// Options is everything a frame needs. It is a value, not a pointer, so a
// caller can snapshot it per tick.
type Options struct {
	Config   *config.Config
	State    *model.State
	Panes    []tmux.Pane
	Width    int
	Height   int
	Selected string // task id
	Filter   string // case-insensitive substring over id, title, group
	Layout   string // "" falls back to Config.Sidebar.Layout
}

// layout resolves the effective layout: the per-call override first, then
// the config, then tiles.
func (o Options) layout(cfg *config.Config) string {
	if o.Layout != "" {
		return o.Layout
	}
	if cfg.Sidebar.Layout != "" {
		return cfg.Sidebar.Layout
	}
	return "tiles"
}

// palette resolves the config theme block into the render palette. The two
// Theme structs are field-identical by design; they are separate types so
// theme does not import config.
func palette(cfg *config.Config) theme.Palette {
	return theme.Resolve(theme.Theme{
		Scheme: cfg.Theme.Scheme,
		Mode:   cfg.Theme.Mode,
		Custom: cfg.Theme.Custom,
	})
}

// Frame renders one complete pane-sized frame: exactly Height lines, each
// exactly Width visible columns, so a repaint never leaves stale text.
func Frame(o Options) string {
	cfg := o.Config
	if cfg == nil {
		cfg = config.Default()
	}
	pal := palette(cfg)
	now := time.Now().Unix()
	rows := roster(cfg, o.State, o.Panes, o.Filter)
	width, height := o.Width, o.Height
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	var lines []string
	if len(rows) == 0 {
		lines = append(lines, format.Render("#[fg=dimmed]no tasks#[default]", nil, pal, width))
	} else {
		lines = append(lines, blockLines(cfg, rows, o, pal, now, width, height)...)
		if len(lines) > height {
			lines = lines[:height]
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

// blockLines renders the task blocks for one frame in the configured
// orientation and layout.
func blockLines(cfg *config.Config, rows []row, o Options, pal theme.Palette, now int64, width, height int) []string {
	stale := func(r row) bool {
		return cfg.Sidebar.DimStale && now-r.activityTS > int64(cfg.Sidebar.StaleAfter)
	}

	if !cfg.IsVertical() {
		// Top or bottom: one full-width block per task, no blank separator.
		var lines []string
		for _, r := range rows {
			lines = append(lines, taskLines(r, cfg.Sidebar.Templates.Horizontal, stale(r), r.id == o.Selected, cfg, pal, now, width)...)
		}
		return lines
	}

	if o.layout(cfg) == "compact" {
		var lines []string
		for _, r := range rows {
			lines = append(lines, taskLines(r, []string{cfg.Sidebar.Templates.Compact}, stale(r), r.id == o.Selected, cfg, pal, now, width)...)
		}
		return lines
	}

	// tiles: one block per task, a blank line between blocks, and a block
	// that would not fit in the remaining height is dropped whole.
	tiles := cfg.Sidebar.Templates.Tiles
	if len(tiles) == 0 {
		tiles = []string{"{title}"}
	}
	var lines []string
	remaining := height
	for _, r := range rows {
		// The block decides its own height: an empty variable (a task with
		// no note) drops that template line instead of leaving a blank row,
		// so two empty lines never appear between tiles.
		block := taskLines(r, tiles, stale(r), r.id == o.Selected, cfg, pal, now, width)
		if len(block) == 0 {
			continue
		}
		cost := len(block)
		if len(lines) > 0 {
			cost++ // the blank separator line
		}
		if cost > remaining {
			break
		}
		if len(lines) > 0 {
			lines = append(lines, strings.Repeat(" ", width))
		}
		lines = append(lines, block...)
		remaining -= cost
	}
	return lines
}

// taskLines renders one task through each template line, padded or truncated
// to exactly width visible columns. A stale row is wrapped in a dim fg for the
// whole block.
func taskLines(r row, tpls []string, stale, selected bool, cfg *config.Config, pal theme.Palette, now int64, width int) []string {
	lines := make([]string, 0, len(tpls))
	for _, tmpl := range tpls {
		if stale {
			// The wrap goes on the template, before format.Render, so the
			// directive is expanded into a real escape rather than shown.
			tmpl = "#[fg=" + pal.Dimmed + "]" + tmpl + "#[default]"
		}
		rendered := format.Render(tmpl, rowVars(r, selected, cfg, pal, now), pal, width)
		// A template line that expands to nothing but padding carries no
		// information; keeping it would space every tile as if it had a
		// third line and push real content off the pane.
		if strings.TrimSpace(format.Strip(rendered)) == "" {
			continue
		}
		lines = append(lines, rendered)
	}
	return lines
}

// rowVars builds the template variables for one task row.
func rowVars(r row, selected bool, cfg *config.Config, pal theme.Palette, now int64) map[string]string {
	primary := r.title
	if selected && pal.CurrentFg != "" {
		primary = "#[fg=" + pal.CurrentFg + ",bold]" + r.title + "#[default]"
	}
	secondary := r.group
	if r.group != "" && pal.Accent != "" {
		secondary = "#[fg=" + pal.Accent + "]" + r.group + "#[default]"
	}
	pane, paneTitle, command, windowIndex := "", "", "", ""
	if r.pane != nil {
		pane = r.pane.Title
		paneTitle = r.pane.Title
		command = r.pane.Command
		windowIndex = strconv.Itoa(r.pane.WindowIndex)
	}
	paneSuffix := ""
	if pane != "" {
		paneSuffix = " " + pane
	}
	return map[string]string{
		"id":           r.id,
		"title":        r.title,
		"primary":      primary,
		"secondary":    secondary,
		"group":        r.group,
		"note":         r.note,
		"status":       r.status,
		"status_icon":  statusIcon(r.status, cfg.Status, pal),
		"elapsed":      config.FormatElapsed(now, r.activityTS),
		"pane":         pane,
		"pane_title":   paneTitle,
		"pane_suffix":  paneSuffix,
		"command":      command,
		"window_index": windowIndex,
		// Placeholder vars exist so templates written for the reference tool
		// load unchanged; sideline fills none of them in v1.
		"git":         "",
		"git_stats":   "",
		"pr_checks":   "",
		"agent_icon":  "",
		"agent_label": "",
	}
}

// statusIcon wraps the configured status icon in a fg directive so
// format.Render colours it. A status with no configured icon renders blank;
// a style without a colour falls back to the palette text colour.
func statusIcon(status string, styles map[string]config.StatusStyle, pal theme.Palette) string {
	st, ok := styles[status]
	if !ok || st.Icon == "" {
		return ""
	}
	colour := st.Colour
	if colour == "" {
		colour = pal.Text
	}
	return "#[fg=" + colour + "]" + st.Icon + "#[default]"
}
