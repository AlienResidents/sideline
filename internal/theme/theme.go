// Package theme holds the colour palettes the sidebar renders with. The
// palettes are background-free: the terminal's own background shows through,
// so dark and light are chosen to stay readable on their ground.
package theme

// Palette is every colour the renderer may ask for by name. Fields are hex
// strings (or "" for terminal default) rather than colour.Color so a palette
// round-trips through YAML and stays printable in logs.
type Palette struct {
	Text           string
	Dimmed         string
	Border         string
	HelpBorder     string
	HelpMuted      string
	Header         string
	Keycap         string
	Info           string
	Success        string
	Warning        string
	Danger         string
	Accent         string
	CurrentFg      string
	CurrentRowBg   string
	HighlightRowBg string
	GutterFg       string
}

// darkPalette is the default ground: a Tokyo Night style tint tuned for a
// transparent terminal on a dark background.
func darkPalette() Palette {
	return Palette{
		Text:           "#c0caf5",
		Dimmed:         "#565f89",
		Border:         "#2f3549",
		HelpBorder:     "#2f3549",
		HelpMuted:      "#565f89",
		Header:         "#bb9af7",
		Keycap:         "#7dcfff",
		Info:           "#7aa2f7",
		Success:        "#9ece6a",
		Warning:        "#e0af68",
		Danger:         "#f7768e",
		Accent:         "#bb9af7",
		CurrentFg:      "#7aa2f7",
		CurrentRowBg:   "#33467c",
		HighlightRowBg: "#292e42",
		GutterFg:       "#bb9af7",
	}
}

// lightPalette keeps readable contrast on a white background.
func lightPalette() Palette {
	return Palette{
		Text:           "#343a40",
		Dimmed:         "#6e7681",
		Border:         "#d0d7de",
		HelpBorder:     "#d0d7de",
		HelpMuted:      "#6e7681",
		Header:         "#8250df",
		Keycap:         "#0969da",
		Info:           "#0969da",
		Success:        "#1a7f37",
		Warning:        "#9a6700",
		Danger:         "#cf222e",
		Accent:         "#8250df",
		CurrentFg:      "#0969da",
		CurrentRowBg:   "#eaeef2",
		HighlightRowBg: "#f0f3f6",
		GutterFg:       "#8250df",
	}
}

// Builtin returns the built-in palette for scheme. Scheme "default" picks by
// mode ("dark" or "light"); "tokyo-night" and unknown schemes fall back to the
// dark palette, and "custom" starts from dark so Resolve has something to
// overlay the custom colours on. Mode is ignored for explicit schemes.
func Builtin(scheme, mode string) Palette {
	switch scheme {
	case "light":
		return lightPalette()
	case "custom", "tokyo-night":
		return darkPalette()
	default:
		if mode == "light" {
			return lightPalette()
		}
		return darkPalette()
	}
}

// Theme is the scheme selector block of the config. It is redeclared here so
// this package does not import internal/config and config does not import this
// one; the fields and yaml names match config.Theme exactly.
type Theme struct {
	Scheme string
	Mode   string
	Custom map[string]string
}

// Resolve returns Builtin(scheme, mode) with Custom applied over it. Custom
// keys map to palette fields by their lower_snake yaml names; unknown keys
// are ignored so a config written for a newer palette still loads.
func Resolve(t Theme) Palette {
	p := Builtin(t.Scheme, t.Mode)
	set := func(field *string, key string) {
		if v, ok := t.Custom[key]; ok {
			*field = v
		}
	}
	set(&p.Text, "text")
	set(&p.Dimmed, "dimmed")
	set(&p.Border, "border")
	set(&p.HelpBorder, "help_border")
	set(&p.HelpMuted, "help_muted")
	set(&p.Header, "header")
	set(&p.Keycap, "keycap")
	set(&p.Info, "info")
	set(&p.Success, "success")
	set(&p.Warning, "warning")
	set(&p.Danger, "danger")
	set(&p.Accent, "accent")
	// current_worktree_fg is the historical key name in existing sidebar
	// configs; keep accepting it so those configs translate unchanged.
	set(&p.CurrentFg, "current_worktree_fg")
	set(&p.CurrentRowBg, "current_row_bg")
	set(&p.HighlightRowBg, "highlight_row_bg")
	set(&p.GutterFg, "gutter_fg")
	return p
}

// Named resolves a colour name ("red", "green", "accent", "text", ...) to a
// hex value, returning false for unknown names. "default" resolves to the
// palette's Text colour, matching the #[default] reset in internal/format.
func (p Palette) Named(name string) (string, bool) {
	switch name {
	case "default":
		return p.Text, true
	case "text":
		return p.Text, true
	case "dimmed":
		return p.Dimmed, true
	case "border":
		return p.Border, true
	case "header":
		return p.Header, true
	case "keycap":
		return p.Keycap, true
	case "info":
		return p.Info, true
	case "success", "green":
		return p.Success, true
	case "warning", "yellow":
		return p.Warning, true
	case "danger", "red":
		return p.Danger, true
	case "accent", "magenta":
		return p.Accent, true
	case "blue":
		return p.Info, true
	case "cyan":
		return p.Keycap, true
	case "white":
		return p.Text, true
	case "black":
		return "#1f2335", true
	}
	return "", false
}
