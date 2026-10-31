package format

import (
	"strings"
	"testing"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/theme"
)

func pal() theme.Palette { return theme.Builtin("default", "dark") }

func TestRenderPlainFill(t *testing.T) {
	got := Render("{status_icon} {title} {fill} {elapsed}", map[string]string{
		"status_icon": "o", "title": "webapp", "elapsed": "7m",
	}, pal(), 43)
	if w := VisibleWidth(got); w != 43 {
		t.Fatalf("width = %d, want 43", w)
	}
	want := "o webapp" + strings.Repeat(" ", 43-len("o webapp 7m")) + " 7m"
	if Strip(got) != want {
		t.Fatalf("plain = %q, want %q", Strip(got), want)
	}
}

func TestRenderStripEqualsPlain(t *testing.T) {
	got := Render("{status_icon} #[bold]{title}#[default] {fill} {elapsed}", map[string]string{
		"status_icon": "o", "title": "docs", "elapsed": "42s",
	}, pal(), 43)
	if s := Strip(got); s != "o docs"+strings.Repeat(" ", 43-len("o docs 42s"))+" 42s" {
		t.Fatalf("Strip(Render) = %q", s)
	}
}

func TestRenderDefaultTemplates43(t *testing.T) {
	c := config.Default()
	vars := map[string]string{
		"status_icon": "o", "title": "webapp", "elapsed": "7m",
		"group": "docs", "git": "main", "note": "fix login", "pane": "%12",
	}
	tmpls := append([]string{c.Sidebar.Templates.Compact}, c.Sidebar.Templates.Tiles...)
	tmpls = append(tmpls, c.Sidebar.Templates.Horizontal...)
	for _, tmpl := range tmpls {
		for _, w := range []int{43, 20, 80} {
			got := Render(tmpl, vars, pal(), w)
			if gw := VisibleWidth(got); gw != w {
				t.Fatalf("template %q width %d: got %d", tmpl, w, gw)
			}
		}
	}
}

func TestRenderTruncates(t *testing.T) {
	got := Render("#{bold}this line is far too long for the pane", nil, pal(), 20)
	if w := VisibleWidth(got); w != 20 {
		t.Fatalf("width = %d, want 20", w)
	}
	if s := Strip(got); s != "this line is far too" {
		t.Fatalf("truncated text = %q", s)
	}
}

func TestRenderOverflowKeepsTail(t *testing.T) {
	got := Render("{title} {fill} {tail}", map[string]string{
		"title": strings.Repeat("x", 30), "tail": "end",
	}, pal(), 20)
	if w := VisibleWidth(got); w != 20 {
		t.Fatalf("width = %d, want 20", w)
	}
	// The text before the fill slot shrinks, so the trailing column
	// (the elapsed time in a real template) is never the casualty.
	s := Strip(got)
	if !strings.HasSuffix(s, "end") {
		t.Fatalf("tail was cut: %q", s)
	}
	if !strings.Contains(s, "…") {
		t.Fatalf("cut should be marked: %q", s)
	}
	if s != strings.Repeat("x", 15)+"… end" {
		t.Fatalf("overflow = %q", s)
	}
}

func TestRenderOverflowShavesAllPrefixSegments(t *testing.T) {
	// When one segment cannot absorb the whole overflow, earlier segments
	// give up their columns too, right to left.
	got := Render("ab{title}{fill}{tail}", map[string]string{
		"title": strings.Repeat("y", 30), "tail": "12m",
	}, pal(), 20)
	if w := VisibleWidth(got); w != 20 {
		t.Fatalf("width = %d, want 20", w)
	}
	if s := Strip(got); !strings.HasSuffix(s, "12m") {
		t.Fatalf("tail was cut: %q", s)
	}
}

func TestRenderFillMiddle(t *testing.T) {
	got := Render("A{fill}Z", nil, pal(), 43)
	if Strip(got) != "A"+strings.Repeat(" ", 41)+"Z" {
		t.Fatalf("middle fill = %q", Strip(got))
	}
}

func TestRenderUnknownVar(t *testing.T) {
	got := Render("[{nope}]", nil, pal(), 4)
	if Strip(got) != "[]"+strings.Repeat(" ", 2) {
		t.Fatalf("unknown var = %q", Strip(got))
	}
}

func TestRenderUnknownDirective(t *testing.T) {
	got := Render("#{blink,zzz}text", nil, pal(), 4)
	if s := Strip(got); s != "text" {
		t.Fatalf("unknown directive = %q", s)
	}
	if strings.Contains(got, "blink") {
		t.Fatalf("unknown directive leaked into output: %q", got)
	}
}

func TestRenderUnterminatedDirective(t *testing.T) {
	got := Render("a#{fg=#ff0000", nil, pal(), 43)
	if s := Strip(got); !strings.HasPrefix(s, "a#{fg=#ff0000") {
		t.Fatalf("unterminated #{ should stay literal, got %q", s)
	}
}

func TestRenderDefaultReset(t *testing.T) {
	got := Render("#{bold,fg=#ff0000}red#[default]plain", nil, pal(), 43)
	s := Strip(got)
	if s != "redplain"+strings.Repeat(" ", 43-len("redplain")) {
		t.Fatalf("plain = %q", s)
	}
	if strings.Count(got, "\x1b[0m") < 2 {
		t.Fatalf("#[default] should reset SGR: %q", got)
	}
}

func TestRenderColourNames(t *testing.T) {
	got := Render("#{fg=red}a#{fg=default}b", nil, pal(), 2)
	if w := VisibleWidth(got); w != 2 {
		t.Fatalf("width = %d", w)
	}
	// red resolves through the palette (to Danger), not to a fixed ANSI red.
	if !strings.Contains(got, "38;2;247;118;142") {
		t.Fatalf("fg=red should emit the palette danger colour: %q", got)
	}
}

func TestRenderDirectivesStateful(t *testing.T) {
	got := Render("#{bold}a#{underline}b", nil, pal(), 2)
	// underline arrives after bold, so both codes ride the same segment.
	if !strings.Contains(got, ";1m") || !strings.Contains(got, ";4m") {
		t.Fatalf("directives should stack: %q", got)
	}
}

func TestRenderDirectiveInsideVar(t *testing.T) {
	// The status icon and the selected title carry their own colour
	// directives. A literal substitution would print them as text.
	got := Render("{icon} {title}", map[string]string{
		"icon":  "#[fg=#ff0000]o#[default]",
		"title": "task",
	}, pal(), 6)
	if s := Strip(got); s != "o task" {
		t.Fatalf("strip = %q, want %q", s, "o task")
	}
	if !strings.Contains(got, "38;2;255;0;0") {
		t.Fatalf("icon colour missing from %q", got)
	}
}

func TestRenderVarBracesStayLiteral(t *testing.T) {
	got := Render("{title}", map[string]string{"title": "{status}"}, pal(), 8)
	if s := Strip(got); s != "{status}" {
		t.Fatalf("braces in a value were re-expanded: %q", s)
	}
}

func TestRenderNerdfontGlyph(t *testing.T) {
	vars := map[string]string{"icon": "󰄱"}
	got := Render("{icon} {fill} x", vars, pal(), 43)
	if w := VisibleWidth(got); w != 43 {
		t.Fatalf("width = %d, want 43", w)
	}
	s := Strip(got)
	if !strings.HasPrefix(s, "󰄱 ") || !strings.HasSuffix(s, " x") {
		t.Fatalf("nerdfont row = %q", s)
	}
}

func TestStrip(t *testing.T) {
	in := "\x1b[1;38;2;255;0;0mred\x1b[0m plain"
	if got := Strip(in); got != "red plain" {
		t.Fatalf("Strip = %q", got)
	}
}

func TestVisibleWidth(t *testing.T) {
	if VisibleWidth("ab\x1b[1mcd") != 4 {
		t.Fatal("ANSI must not count toward width")
	}
	if VisibleWidth("") != 0 {
		t.Fatal("empty width")
	}
}
