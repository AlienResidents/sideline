package render

import (
	"fmt"
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
	SpinnerFrame int
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
	text, _ := FrameWithMap(o)
	return text
}

// FrameWithMap renders the frame and reports which task row owns each line.
// A line with no row, such as a blank separator or trailing padding, maps to
// "". The renderer keeps that map so a mouse click can be turned into a row.
func FrameWithMap(o Options) (string, []string) {
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

	var lines, ids []string
	if len(rows) == 0 {
		lines = append(lines, format.Render("#[fg=dimmed]no tasks#[default]", nil, pal, width))
		ids = append(ids, "")
	} else {
		lines, ids = blockLines(cfg, rows, o, pal, now, width, height)
		if len(lines) > height {
			lines = lines[:height]
			ids = ids[:height]
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
		ids = append(ids, "")
	}
	return strings.Join(lines, "\n"), ids
}

// blockLines renders the task blocks for one frame in the configured
// orientation and layout.
func blockLines(cfg *config.Config, rows []row, o Options, pal theme.Palette, now int64, width, height int) ([]string, []string) {
	stale := func(r row) bool {
		return cfg.Sidebar.DimStale && now-r.activityTS > int64(cfg.Sidebar.StaleAfter)
	}

	if !cfg.IsVertical() {
		// Top or bottom: one full-width block per task, no blank separator.
		var lines, ids []string
		for _, r := range rows {
			block := taskLines(r, cfg.Sidebar.Templates.Horizontal, stale(r), r.id == o.Selected, cfg, pal, now, width, o.SpinnerFrame)
			lines = append(lines, block...)
			ids = append(ids, repeatID(r.id, len(block))...)
		}
		return lines, ids
	}

	if o.layout(cfg) == "compact" {
		var lines, ids []string
		for _, r := range rows {
			block := taskLines(r, []string{cfg.Sidebar.Templates.Compact}, stale(r), r.id == o.Selected, cfg, pal, now, width, o.SpinnerFrame)
			lines = append(lines, block...)
			ids = append(ids, repeatID(r.id, len(block))...)
		}
		return lines, ids
	}

	// tiles: one block per task, a blank line between blocks, and a block
	// that would not fit in the remaining height is dropped whole.
	tiles := cfg.Sidebar.Templates.Tiles
	if len(tiles) == 0 {
		tiles = []string{"{title}"}
	}
	// The gutter costs columns from every tile line, not from the pane.
	gutter := cfg.Sidebar.Gutter
	if gutter < 0 || gutter > width/2 {
		gutter = 0
	}
	contentWidth := width - gutter

	var lines, ids []string
	remaining := height
	for _, r := range rows {
		// The block decides its own height: an empty variable (a task with
		// no note) drops that template line instead of leaving a blank row,
		// so two empty lines never appear between tiles.
		block := taskLines(r, tiles, stale(r), r.id == o.Selected, cfg, pal, now, contentWidth, o.SpinnerFrame)
		if len(block) == 0 {
			continue
		}
		if gutter > 0 {
			bar := gutterBar(r, pal, gutter)
			for i := range block {
				block[i] = bar + block[i]
			}
		}
		// Only the selected row takes the full-width band. Several rows can
		// share the live pane, and banding every one of them made the
		// highlight read as more rows than one. The live pane keeps its own
		// markers: the gutter bar and the current_worktree_fg title.
		if r.id == o.Selected {
			for i := range block {
				block[i] = rowBackground(block[i], pal.CurrentRowBg)
			}
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
			ids = append(ids, "")
		}
		lines = append(lines, block...)
		ids = append(ids, repeatID(r.id, len(block))...)
		remaining -= cost
	}
	return lines, ids
}

// repeatID is the per-line owner list for one task block.
func repeatID(id string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = id
	}
	return out
}

// rowBackground paints a subtle band behind a whole row. The band is applied
// after rendering and re-applied after every reset, because the template's own
// style changes would otherwise punch holes in it.
func rowBackground(line, hex string) string {
	sgr := bgSGR(hex)
	if sgr == "" {
		return line
	}
	return sgr + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+sgr) + "\x1b[0m"
}

// bgSGR turns "#rrggbb" into a truecolor background escape. Anything else
// (a named colour, an empty value) yields "" so the row renders unstyled.
func bgSGR(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return ""
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", v>>16&0xff, v>>8&0xff, v&0xff)
}

// gutterBar is the indicator column left of a tile: a bright bar for the
// window the user is looking at, a dim one for every other row. It renders at
// exactly n columns, so the tile content still ends at the pane edge.
func gutterBar(r row, pal theme.Palette, n int) string {
	colour := pal.Dimmed
	if isCurrentRow(r) {
		colour = pal.GutterFg
	}
	if colour == "" {
		colour = pal.Text
	}
	return format.Render("#[fg="+colour+"]▌#[default]", nil, pal, n)
}

// isCurrentRow reports whether this row belongs to the window in front.
func isCurrentRow(r row) bool {
	return r.pane != nil && r.pane.Active && r.pane.WindowActive
}

// taskLines renders one task through each template line, padded or truncated
// to exactly width visible columns. A stale row is wrapped in a dim fg for the
// whole block.
func taskLines(r row, tpls []string, stale, selected bool, cfg *config.Config, pal theme.Palette, now int64, width, spinnerFrame int) []string {
	lines := make([]string, 0, len(tpls))
	for _, tmpl := range tpls {
		if stale {
			// The wrap goes on the template, before format.Render, so the
			// directive is expanded into a real escape rather than shown.
			tmpl = "#[fg=" + pal.Dimmed + "]" + tmpl + "#[default]"
		}
		rendered := format.Render(tmpl, rowVars(r, selected, cfg, pal, now, spinnerFrame), pal, width)
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
func rowVars(r row, selected bool, cfg *config.Config, pal theme.Palette, now int64, spinnerFrame int) map[string]string {
	// The reference sidebar highlights the row for the current window, not a
	// cursor position, so the eye lands on what is in front of you.
	current := isCurrentRow(r)
	// A shell row tracks the pane's foreground process, so the row reads
	// `vim` or `claude` without any writer having to notice. The stored title
	// is the directory and stays as the fallback.
	text := r.title
	status := r.status
	if r.owner == model.OwnerShell && r.pane != nil {
		if r.pane.Command != "" {
			text = r.pane.Command
		}
		// A shell sitting at its prompt is waiting for input. Anything else
		// in the pane is work in progress.
		if isShellCommand(r.pane.Command) {
			status = "waiting"
		} else {
			status = "working"
		}
	}
	primary := text
	if (selected || current) && pal.CurrentFg != "" {
		primary = "#[fg=" + pal.CurrentFg + ",bold]" + text + "#[default]"
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
		"status":       status,
		"status_icon":  statusIcon(status, cfg.Status, pal, spinnerFrame),
		"command_icon": commandIcon(r.pane, cfg.Icons, pal),
		"agent_icon":   commandIcon(r.pane, cfg.Icons, pal),
		"agent_label":  commandLabel(r.pane, cfg.Icons),
		"elapsed":      config.FormatElapsed(now, r.activityTS),
		"pane":         pane,
		"pane_title":   paneTitle,
		"pane_suffix":  paneSuffix,
		"command":      command,
		"window_index": windowIndex,
		// These exist so templates written for the reference tool load
		// unchanged. sideline has no git or PR integration, so they stay
		// empty; {agent_icon} and {agent_label} above are filled.
		"git":       "",
		"git_stats": "",
		"pr_checks": "",
	}
}

// commandIcon is the glyph for the process running in the bound pane, so an
// omp session and a shell are distinguishable at the start of a row. An
// unbound task, or an unknown process, renders nothing. agent_icon is an
// alias, kept so templates written for the reference tool keep working.
func commandIcon(pane *tmux.Pane, icons map[string]config.IconStyle, pal theme.Palette) string {
	if pane == nil || pane.Command == "" {
		return ""
	}
	st, ok := icons[pane.Command]
	if !ok || st.Icon == "" {
		return ""
	}
	colour := st.Colour
	if colour == "" {
		colour = pal.Text
	}
	return "#[fg=" + colour + "]" + st.Icon + "#[default]"
}

// commandLabel is the human name for the pane's process, feeding
// {agent_label}. Empty when the task is unbound or the process is unknown.
func commandLabel(pane *tmux.Pane, icons map[string]config.IconStyle) string {
	if pane == nil {
		return ""
	}
	return icons[pane.Command].Label
}

// statusIcon wraps the configured status icon in a fg directive so
// format.Render colours it. A status with no configured icon renders blank;
// a style without a colour falls back to the palette text colour.
var workingSpinnerFrames = []string{"⠋⠙", "⠙⠹", "⠹⠸", "⠸⠼", "⠼⠴", "⠴⠦", "⠦⠧", "⠧⠇", "⠇⠏", "⠏⠋"}

func statusIcon(status string, styles map[string]config.StatusStyle, pal theme.Palette, spinnerFrame int) string {
	st, ok := styles[status]
	colour := st.Colour
	if colour == "" {
		colour = pal.Text
	}
	if status == "working" {
		glyph := workingSpinnerFrames[spinnerFrame%len(workingSpinnerFrames)]
		return "#[fg=" + colour + "]" + glyph + "#[default]"
	}
	if !ok || st.Icon == "" {
		return ""
	}
	return "#[fg=" + colour + "]" + st.Icon + "#[default]"
}
