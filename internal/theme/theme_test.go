package theme

import "testing"

func TestBuiltinDark(t *testing.T) {
	p := Builtin("default", "dark")
	want := map[string]string{
		"Text": "#c0caf5", "Dimmed": "#565f89", "Border": "#2f3549",
		"HelpBorder": "#2f3549", "HelpMuted": "#565f89", "Header": "#bb9af7",
		"Keycap": "#7dcfff", "Info": "#7aa2f7", "Success": "#9ece6a",
		"Warning": "#e0af68", "Danger": "#f7768e", "Accent": "#bb9af7",
		"CurrentFg": "#7aa2f7", "CurrentRowBg": "#33467c", "HighlightRowBg": "#292e42",
	}
	got := map[string]string{
		"Text": p.Text, "Dimmed": p.Dimmed, "Border": p.Border,
		"HelpBorder": p.HelpBorder, "HelpMuted": p.HelpMuted, "Header": p.Header,
		"Keycap": p.Keycap, "Info": p.Info, "Success": p.Success,
		"Warning": p.Warning, "Danger": p.Danger, "Accent": p.Accent,
		"CurrentFg": p.CurrentFg, "CurrentRowBg": p.CurrentRowBg, "HighlightRowBg": p.HighlightRowBg,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("dark %s = %q, want %q", k, got[k], w)
		}
	}
}

func TestBuiltinLight(t *testing.T) {
	p := Builtin("light", "dark")
	if p.Text == "#c0caf5" {
		t.Fatal("light scheme should not return the dark text colour")
	}
	if p.Text == "" || p.Danger == "" || p.CurrentRowBg == "" {
		t.Fatal("light palette has blank fields")
	}
	// Mode picks when scheme is default.
	if Builtin("default", "light").Text != p.Text {
		t.Fatal("mode=light should select the light palette")
	}
}

func TestBuiltinCustomStartsFromDark(t *testing.T) {
	// custom has no palette of its own; it starts from dark so Resolve can
	// overlay the user colours on something readable.
	if Builtin("custom", "light").Text != Builtin("default", "dark").Text {
		t.Fatal("custom should start from the dark palette")
	}
}

func TestResolve(t *testing.T) {
	p := Resolve(Theme{
		Scheme: "custom",
		Mode:   "dark",
		Custom: map[string]string{
			"text":                "#abcdef",
			"current_worktree_fg": "#123456",
			"highlight_row_bg":    "#000000",
			"nonsense":            "#ffffff",
		},
	})
	dark := Builtin("default", "dark")
	if p.Text != "#abcdef" {
		t.Errorf("text override: %q", p.Text)
	}
	if p.CurrentFg != "#123456" {
		t.Errorf("current_worktree_fg alias: %q", p.CurrentFg)
	}
	if p.HighlightRowBg != "#000000" {
		t.Errorf("highlight_row_bg: %q", p.HighlightRowBg)
	}
	if p.Header != dark.Header {
		t.Errorf("untouched field changed: %q vs %q", p.Header, dark.Header)
	}
}

func TestNamed(t *testing.T) {
	p := Builtin("default", "dark")
	for _, name := range []string{"red", "green", "yellow", "blue", "magenta", "cyan", "white", "black", "default", "accent", "text", "dimmed", "border", "header", "keycap", "info", "success", "warning", "danger"} {
		if v, ok := p.Named(name); !ok || v == "" {
			t.Errorf("Named(%q) = %q, %v", name, v, ok)
		}
	}
	if _, ok := p.Named("chartreuse"); ok {
		t.Error("unknown name should not resolve")
	}
	if v, _ := p.Named("text"); v != p.Text {
		t.Error("text should resolve to the palette text colour")
	}
	if v, _ := p.Named("default"); v != p.Text {
		t.Error("default should resolve to the palette text colour")
	}
}
