// Package format renders tmux-style format templates into styled lines. It
// exists so a template written for another sidebar tool keeps working here.
package format

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"github.com/AlienResidents/sideline/internal/theme"
)

// style is the stateful fg/bg/attribute set a template accumulates as
// directives appear. It mirrors tmux: a directive applies until #[default].
type style struct {
	fg, bg                   string
	bold, dim, italic, uline bool
	reverse                  bool
}

// reset restores the palette default text style: palette fg, no background,
// no attributes.
func (s *style) reset(pal theme.Palette) {
	s.fg = pal.Text
	s.bg = ""
	s.bold, s.dim, s.italic, s.uline, s.reverse = false, false, false, false, false
}

// sgr returns the ANSI SGR sequence for the style, or "" when nothing is set.
func (s *style) sgr() string {
	var codes []string
	if r, g, b, ok := hexRGB(s.fg); ok {
		codes = append(codes, "38;2;"+r+";"+g+";"+b)
	}
	if s.bg != "" {
		if r, g, b, ok := hexRGB(s.bg); ok {
			codes = append(codes, "48;2;"+r+";"+g+";"+b)
		}
	}
	if s.bold {
		codes = append(codes, "1")
	}
	if s.dim {
		codes = append(codes, "2")
	}
	if s.italic {
		codes = append(codes, "3")
	}
	if s.uline {
		codes = append(codes, "4")
	}
	if s.reverse {
		codes = append(codes, "7")
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// hexRGB splits an #rrggbb colour into decimal component strings.
func hexRGB(hex string) (r, g, b string, ok bool) {
	hex = strings.TrimSpace(hex)
	if len(hex) != 7 || hex[0] != '#' {
		return "", "", "", false
	}
	dv := func(s string) (int, bool) {
		n, err := strconv.ParseUint(s, 16, 8)
		return int(n), err == nil
	}
	rn, ok1 := dv(hex[1:3])
	gn, ok2 := dv(hex[3:5])
	bn, ok3 := dv(hex[5:7])
	if !ok1 || !ok2 || !ok3 {
		return "", "", "", false
	}
	return strconv.Itoa(rn), strconv.Itoa(gn), strconv.Itoa(bn), true
}

// Render expands vars and tmux style directives, then pads or truncates the
// result to exactly width visible columns. Unknown var names expand to
// nothing; unknown directives are dropped without corrupting the rest of the
// line. {fill} expands to the run of spaces that reaches width, and to
// nothing when the line already exceeds it.
func Render(tmpl string, vars map[string]string, pal theme.Palette, width int) string {
	if width <= 0 {
		return ""
	}
	st := &style{}
	st.reset(pal)

	// Pass 1: tokenise, substitute vars, record {fill} slots.
	type seg struct {
		text string
		sgr  string // style active when this text begins
	}
	var segs []seg
	var fillAt []int
	cur := ""
	curSGR := st.sgr()
	flush := func() {
		if cur != "" {
			segs = append(segs, seg{text: cur, sgr: curSGR})
		}
		cur = ""
		curSGR = st.sgr()
	}

	// scan copies s into the segments, honouring directives. allowVars is
	// false for variable values: a title containing braces stays literal, but
	// a value that carries its own colour directives (the status icon, the
	// selected title) is still styled, which is how the reference tool's
	// templates behave.
	var scan func(s string, allowVars bool)
	scan = func(s string, allowVars bool) {
		for i := 0; i < len(s); {
			switch {
			case strings.HasPrefix(s[i:], "#{"), strings.HasPrefix(s[i:], "#["):
				closing := byte('}')
				if s[i+1] == '[' {
					closing = ']'
				}
				end := strings.IndexByte(s[i:], closing)
				flush()
				if end < 0 {
					// Unterminated directive: tmux renders it literally,
					// and so do we, to the end of the line.
					cur = s[i:]
					i = len(s)
					continue
				}
				applyDirectives(s[i+2:i+end], st, pal)
				curSGR = st.sgr()
				i += end + 1
			case allowVars && s[i] == '{':
				end := strings.IndexByte(s[i:], '}')
				if end < 0 {
					cur = s[i:]
					i = len(s)
					continue
				}
				name := s[i+1 : i+end]
				if name == "fill" {
					flush()
					fillAt = append(fillAt, len(segs))
					segs = append(segs, seg{text: "", sgr: curSGR})
					curSGR = st.sgr()
				} else {
					scan(vars[name], false)
				}
				i += end + 1
			default:
				// Copy one rune so stray braces stay literal.
				r, size := utf8.DecodeRuneInString(s[i:])
				cur += string(r)
				i += size
			}
		}
	}
	scan(tmpl, true)
	flush()

	// Base width without fills decides how wide each fill is.
	base := 0
	for _, s := range segs {
		base += VisibleWidth(s.text)
	}
	pad := width - base
	if pad < 0 {
		pad = 0
	}
	for _, idx := range fillAt {
		segs[idx].text = strings.Repeat(" ", pad)
	}

	// Pass 2: emit SGR per segment, truncating at the width boundary and
	// padding when the template has no {fill} of its own.
	var b strings.Builder
	col := 0
	prev := ""
	for _, s := range segs {
		if col >= width {
			break
		}
		if s.sgr != prev {
			b.WriteString("\x1b[0m")
			if s.sgr != "" {
				b.WriteString(s.sgr)
			}
			prev = s.sgr
		}
		cut := truncateTo(s.text, width-col)
		b.WriteString(cut)
		col += VisibleWidth(cut)
	}
	if col < width {
		if prev != "" {
			b.WriteString("\x1b[0m")
			prev = ""
		}
		b.WriteString(strings.Repeat(" ", width-col))
	}
	if prev != "" {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// applyDirectives applies one comma-separated directive list onto st. Every
// directive changes state for the rest of the line; unknown ones are dropped
// without touching the state.
func applyDirectives(body string, st *style, pal theme.Palette) {
	for _, d := range strings.Split(body, ",") {
		d = strings.TrimSpace(d)
		switch {
		case d == "default":
			st.reset(pal)
		case d == "bold":
			st.bold = true
		case d == "dim":
			st.dim = true
		case d == "italic":
			st.italic = true
		case d == "underline":
			st.uline = true
		case d == "reverse":
			st.reverse = true
		case strings.HasPrefix(d, "fg="):
			if v, ok := colourValue(strings.TrimPrefix(d, "fg="), pal); ok {
				st.fg = v
			}
		case strings.HasPrefix(d, "bg="):
			if v, ok := colourValue(strings.TrimPrefix(d, "bg="), pal); ok {
				st.bg = v
			}
		}
	}
}

// colourValue resolves a fg=/bg= argument, which is a hex colour or a palette
// colour name.
func colourValue(v string, pal theme.Palette) (string, bool) {
	if strings.HasPrefix(v, "#") {
		if _, _, _, ok := hexRGB(v); ok {
			return v, true
		}
		return "", false
	}
	return pal.Named(v)
}

// truncateTo cuts s to at most w visible columns without wrapping.
func truncateTo(s string, w int) string {
	if w <= 0 || s == "" {
		return ""
	}
	if VisibleWidth(s) <= w {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if col+rw > w {
			break
		}
		b.WriteRune(r)
		col += rw
	}
	return b.String()
}

// sgrRe matches one ANSI SGR sequence.
var sgrRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Strip removes ANSI SGR sequences.
func Strip(s string) string {
	return sgrRe.ReplaceAllString(s, "")
}

// VisibleWidth returns the rendered column count, ANSI escapes ignored.
func VisibleWidth(s string) int {
	return lipgloss.Width(Strip(s))
}
