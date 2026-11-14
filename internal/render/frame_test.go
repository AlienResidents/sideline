package render

import (
	"strings"
	"testing"
	"time"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/format"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/theme"
	"github.com/AlienResidents/sideline/internal/tmux"
)

// testConfig returns a config with plain ASCII templates so assertions can
// read the output without guessing which template a line came from.
func testConfig() *config.Config {
	cfg := config.Default()
	cfg.Sidebar.Position = "left"
	cfg.Sidebar.Width = 43
	cfg.Sidebar.Height = 20
	cfg.Sidebar.Layout = "tiles"
	// Geometry tests are about template expansion; the gutter has its own
	// test and would shift every expected column.
	cfg.Sidebar.Gutter = 0
	cfg.Sidebar.Templates = config.Templates{
		Compact:    "{status_icon} {title} {note}",
		Tiles:      []string{"{primary}", "{group}", "{note}"},
		Horizontal: []string{"H# {title}"},
	}
	cfg.Status = map[string]config.StatusStyle{
		"todo":    {Icon: "o", Colour: "#7aa2f7"},
		"working": {Icon: "w", Colour: "#7aa2f7"},
		"done":    {Icon: "d", Colour: "#9ece6a"},
	}
	return cfg
}

// checkPane asserts every line of a frame is exactly width visible columns
// and there are exactly height lines.
func checkPane(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("frame has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := format.VisibleWidth(l); w != width {
			t.Fatalf("line %d is %d visible columns, want %d: %q", i, w, width, l)
		}
	}
}

func TestFrameDimensionsMixedRoster(t *testing.T) {
	cfg := testConfig()
	cfg.Tasks = []config.TaskDecl{
		{ID: "alpha", Title: "Alpha declared", Group: "grp-a"},
		{ID: "beta", Title: "Beta declared"},
	}
	st := &model.State{
		Version: model.StateVersion,
		Tasks: []model.Task{
			{ID: "alpha", Title: "Alpha state", Status: "working"},
			{ID: "gamma", Title: "Gamma extra", Status: "done"},
		},
	}
	out := Frame(Options{Config: cfg, State: st, Width: 43, Height: 20})
	checkPane(t, out, 43, 20)

	plain := format.Strip(out)
	for _, want := range []string{"Alpha state", "Beta declared", "Gamma extra"} {
		if !strings.Contains(plain, want) {
			t.Errorf("frame %q is missing %q", plain, want)
		}
	}
}

func TestStateOverridesConfig(t *testing.T) {
	cfg := testConfig()
	cfg.Tasks = []config.TaskDecl{{ID: "alpha", Title: "Config Title"}}
	st := &model.State{Tasks: []model.Task{{ID: "alpha", Title: "State Title"}}}
	plain := format.Strip(Frame(Options{Config: cfg, State: st, Width: 43, Height: 20}))
	if !strings.Contains(plain, "State Title") {
		t.Errorf("state title did not override config: %q", plain)
	}
	if strings.Contains(plain, "Config Title") {
		t.Errorf("config title survived a state override: %q", plain)
	}
}

// tileTitles collects every non-blank rendered line. Tile height varies with
// the content now (an empty variable drops its line), so stepping a fixed
// stride would skip rows.
func tileTitles(t *testing.T, out string) []string {
	t.Helper()
	var titles []string
	for _, line := range strings.Split(format.Strip(out), "\n") {
		line = strings.TrimRight(line, " ")
		if line != "" {
			titles = append(titles, line)
		}
	}
	return titles
}

func TestCommandIconPerProcess(t *testing.T) {
	// An omp pane and a shell must be distinguishable at the start of a row.
	cfg := testConfig()
	cfg.Sidebar.Templates = config.Templates{
		Compact: "{command_icon}{title}",
		Tiles:   []string{"{command_icon}{title}"},
	}
	panes := []tmux.Pane{
		{PaneID: "%1", Command: "omp", WindowIndex: 0, Session: "s"},
		{PaneID: "%2", Command: "bash", WindowIndex: 1, Session: "s"},
	}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "agent", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		{ID: "b", Title: "shell", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
		{ID: "c", Title: "loose"},
	}}
	out := format.Strip(Frame(Options{Config: cfg, State: st, Panes: panes, Width: 40, Height: 20}))
	if !strings.Contains(out, "⌥agent") {
		t.Fatalf("omp row missing its icon:\n%s", out)
	}
	if !strings.Contains(out, "\uf120shell") {
		t.Fatalf("bash row missing its icon:\n%s", out)
	}
	if !strings.Contains(out, "loose") {
		t.Fatalf("unbound row missing:\n%s", out)
	}
}

func TestGutterMarksCurrentRow(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Gutter = 2
	cfg.Sidebar.Templates = config.Templates{Tiles: []string{"{primary}"}}
	panes := []tmux.Pane{
		{PaneID: "%1", Command: "omp", Active: true, WindowActive: true, Session: "s"},
		{PaneID: "%2", Command: "omp", Active: true, WindowActive: false, Session: "s"},
	}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "front", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		{ID: "b", Title: "back", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
	}}
	out := Frame(Options{Config: cfg, State: st, Panes: panes, Width: 43, Height: 20})
	checkPane(t, out, 43, 20)
	lines := strings.Split(format.Strip(out), "\n")
	for _, want := range []string{"▌ front", "▌ back"} {
		found := false
		for _, l := range lines {
			if strings.HasPrefix(l, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing gutter bar for %q in:\n%s", want, out)
		}
	}
}

func TestGutterUsesStatusColourOnInactiveRows(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Gutter = 2
	cfg.Sidebar.Templates = config.Templates{Tiles: []string{"{primary}"}}
	cfg.Status = map[string]config.StatusStyle{
		"waiting": {Colour: "#e0af68"},
	}
	panes := []tmux.Pane{
		{PaneID: "%1", Command: "omp", Active: true, WindowActive: true, Session: "s"},
		{PaneID: "%2", Command: "omp", Active: true, WindowActive: false, Session: "s"},
		{PaneID: "%3", Command: "omp", Active: true, WindowActive: false, Session: "s"},
	}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "front", Status: "waiting", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		{ID: "b", Title: "idle-wait", Status: "waiting", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
		{ID: "c", Title: "idle-plain", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%3"}},
	}}
	out := Frame(Options{Config: cfg, State: st, Panes: panes, Width: 43, Height: 20})
	checkPane(t, out, 43, 20)

	const gutterFg = "\x1b[38;2;187;154;247m▌"
	const statusFg = "\x1b[38;2;224;175;104m▌"
	const dimmedFg = "\x1b[38;2;86;95;137m▌"

	var frontLine, waitLine, plainLine string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "front"):
			frontLine = line
		case strings.Contains(line, "idle-wait"):
			waitLine = line
		case strings.Contains(line, "idle-plain"):
			plainLine = line
		}
	}
	if !strings.Contains(frontLine, gutterFg) {
		t.Fatalf("current row should use GutterFg even with status, got %q", frontLine)
	}
	if !strings.Contains(waitLine, statusFg) {
		t.Fatalf("inactive row with status should use status colour, got %q", waitLine)
	}
	if !strings.Contains(plainLine, dimmedFg) {
		t.Fatalf("inactive row without status should fall back to Dimmed, got %q", plainLine)
	}
}

func TestOnlySelectedRowGetsBand(t *testing.T) {
	// The full-width band belongs to the selected row alone. The band used
	// to follow `selected OR current`, so moving the cursor off the live
	// window banded two distinct rows, and two rows bound to the same pane
	// banded together: either way the sidebar read as more highlighted rows
	// than one.
	cfg := testConfig()
	cfg.Sidebar.Gutter = 0
	cfg.Sidebar.Templates = config.Templates{Tiles: []string{"{primary}", "{note}"}}
	panes := []tmux.Pane{
		{PaneID: "%1", Command: "omp", Active: true, WindowActive: true, Session: "s"},
		{PaneID: "%2", Command: "omp", Active: true, WindowActive: false, Session: "s"},
	}
	st := &model.State{Tasks: []model.Task{
		// Selected, and its pane is not the one in front.
		{ID: "a", Title: "chosen", Note: "x", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
		// The live row, on its own pane, left unselected.
		{ID: "b", Title: "front", Note: "y", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		// A second row on the same live pane.
		{ID: "c", Title: "sibling", Note: "z", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
	}}
	out := Frame(Options{Config: cfg, State: st, Panes: panes, Selected: "a", Width: 43, Height: 20})
	// #33467c is the default current_row_bg.
	const band = "\x1b[48;2;51;70;124m"
	var chosen, other []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "chosen") {
			chosen = append(chosen, line)
		}
		if strings.Contains(line, "front") || strings.Contains(line, "sibling") {
			other = append(other, line)
		}
	}
	if len(chosen) == 0 || !strings.Contains(chosen[0], band) {
		t.Fatalf("selected row has no background band: %q", chosen)
	}
	for _, line := range other {
		if strings.Contains(line, band) {
			t.Fatalf("unselected row carries the band: %q", line)
		}
	}
}

func TestUnselectedCurrentRowKeepsMarkers(t *testing.T) {
	// Losing the band must not cost the live row its own markers: the
	// bright gutter bar and the current_worktree_fg title stay, so the row
	// of the window in front is still findable without it.
	cfg := testConfig()
	cfg.Sidebar.Gutter = 2
	cfg.Sidebar.Templates = config.Templates{Tiles: []string{"{primary}"}}
	panes := []tmux.Pane{
		{PaneID: "%1", Command: "omp", Active: true, WindowActive: true, Session: "s"},
		{PaneID: "%2", Command: "omp", Active: true, WindowActive: false, Session: "s"},
	}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "chosen", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
		{ID: "b", Title: "front", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
	}}
	out := Frame(Options{Config: cfg, State: st, Panes: panes, Selected: "a", Width: 43, Height: 20})
	const band = "\x1b[48;2;51;70;124m"
	// #7aa2f7 is the default current_worktree_fg (live row title), and
	// #bb9af7 is the default gutter_fg (bright gutter bar).
	const current = "\x1b[38;2;122;162;247;1m"
	const gutter = "\x1b[38;2;187;154;247m"
	var front []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "front") {
			front = append(front, line)
		}
	}
	if len(front) == 0 || !strings.Contains(front[0], current) {
		t.Fatalf("live row lost its gutter/title colour: %q", front)
	}
	if !strings.Contains(front[0], gutter) {
		t.Fatalf("live row lost its gutter colour: %q", front)
	}
	if strings.Contains(front[0], band) {
		t.Fatalf("unselected live row carries the band: %q", front)
	}
}

func TestActiveWindowRowIsHighlighted(t *testing.T) {
	// The reference sidebar highlights the row for the window you are
	// looking at, not a cursor, so the eye lands on the live session.
	cfg := testConfig()
	cfg.Theme.Custom = map[string]string{"current_worktree_fg": "#123456"}
	cfg.Sidebar.Templates = config.Templates{Tiles: []string{"{primary}"}}
	panes := []tmux.Pane{
		{PaneID: "%1", Command: "omp", Active: true, WindowActive: true, Session: "s", WindowIndex: 0},
		{PaneID: "%2", Command: "omp", Active: true, WindowActive: false, Session: "s", WindowIndex: 1},
	}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "front", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		{ID: "b", Title: "back", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
	}}
	out := Frame(Options{Config: cfg, State: st, Panes: panes, Width: 40, Height: 20})
	if got := strings.Count(out, "38;2;18;52;86"); got != 1 {
		t.Fatalf("highlight colour applied %d times, want 1:\n%s", got, out)
	}
}

func TestAgentLabelPerProcess(t *testing.T) {
	// {agent_label} carries the process name, so a reference-tool template
	// that prints it works unchanged.
	cfg := testConfig()
	cfg.Sidebar.Templates = config.Templates{
		Compact: "{agent_label}",
		Tiles:   []string{"{agent_label}"},
	}
	panes := []tmux.Pane{{PaneID: "%1", Command: "omp", WindowIndex: 0, Session: "s"}}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "t", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
	}}
	out := format.Strip(Frame(Options{Config: cfg, State: st, Panes: panes, Width: 40, Height: 10}))
	if !strings.Contains(out, "Oh My Pi") {
		t.Fatalf("agent label missing:\n%s", out)
	}
}

func TestTilesDropEmptyLines(t *testing.T) {
	// A task with no note must not leave a blank third line plus the
	// separator: two blank lines between tiles looked like a gap.
	cfg := testConfig()
	cfg.Sidebar.Templates.Tiles = []string{"{primary}", "{note}"}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "First"},
		{ID: "b", Title: "Second"},
	}}
	out := format.Strip(Frame(Options{Config: cfg, State: st, Width: 43, Height: 20}))
	lines := strings.Split(out, "\n")
	if strings.TrimSpace(lines[0]) != "First" {
		t.Fatalf("line 0 = %q", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "" {
		t.Fatalf("expected the separator blank at line 1, got %q", lines[1])
	}
	if strings.TrimSpace(lines[2]) != "Second" {
		t.Fatalf("line 2 = %q, want Second", lines[2])
	}
}

func TestTilesKeepNoteLine(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Templates.Tiles = []string{"{primary}", "{note}"}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "First", Note: "waiting on CI"},
	}}
	out := format.Strip(Frame(Options{Config: cfg, State: st, Width: 43, Height: 20}))
	lines := strings.Split(out, "\n")
	if strings.TrimSpace(lines[1]) != "waiting on CI" {
		t.Fatalf("note line = %q", lines[1])
	}
}

func TestSortActivity(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Sort = "activity"
	now := time.Now().Unix()
	st := &model.State{Tasks: []model.Task{
		{ID: "old", Title: "Old task", ActivityTS: now - 900},
		{ID: "new", Title: "New task", ActivityTS: now - 10},
		{ID: "mid", Title: "Mid task", ActivityTS: now - 100},
	}}
	out := Frame(Options{Config: cfg, State: st, Width: 43, Height: 20})
	got := tileTitles(t, out)
	want := []string{"New task", "Mid task", "Old task"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("activity order is %v, want %v", got, want)
	}
}

func TestSortWindow(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Sort = "window"
	panes := []tmux.Pane{
		{PaneID: "%1", WindowIndex: 2, Session: "main", Command: "vim"},
		{PaneID: "%2", WindowIndex: 1, Session: "main", Command: "bash"},
	}
	st := &model.State{Tasks: []model.Task{
		{ID: "later", Title: "Later window", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		{ID: "free", Title: "Unbound task"},
		{ID: "early", Title: "Early window", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
	}}
	out := Frame(Options{Config: cfg, State: st, Panes: panes, Width: 43, Height: 20})
	got := tileTitles(t, out)
	want := []string{"Early window", "Later window", "Unbound task"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("window order is %v, want %v (unbound must sort last)", got, want)
	}
}

func TestFilterDropsRows(t *testing.T) {
	cfg := testConfig()
	st := &model.State{Tasks: []model.Task{
		{ID: "aaa", Title: "Alpha"},
		{ID: "bbb", Title: "Beta"},
		{ID: "ccc", Title: "Gamma", Group: "zzzgroup"},
	}}
	plain := format.Strip(Frame(Options{Config: cfg, State: st, Width: 43, Height: 20, Filter: "zzZ"}))
	if !strings.Contains(plain, "Gamma") {
		t.Errorf("filter dropped the matching row: %q", plain)
	}
	if strings.Contains(plain, "Alpha") || strings.Contains(plain, "Beta") {
		t.Errorf("filter kept non-matching rows: %q", plain)
	}
}

func TestHorizontalLayout(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Position = "top"
	st := &model.State{Tasks: []model.Task{{ID: "a", Title: "Across"}}}
	plain := format.Strip(Frame(Options{Config: cfg, State: st, Width: 43, Height: 20}))
	if !strings.Contains(plain, "H# Across") {
		t.Errorf("horizontal template was not used: %q", plain)
	}
	if strings.Contains(plain, "{primary}") {
		t.Errorf("tile template leaked into horizontal layout: %q", plain)
	}
}

func TestShellRowTracksForegroundCommand(t *testing.T) {
	cfg := testConfig()
	cfg.Status["waiting"] = config.StatusStyle{Icon: "W", Colour: "#e0af68"}
	pal := theme.Resolve(theme.Theme{
		Scheme: cfg.Theme.Scheme,
		Mode:   cfg.Theme.Mode,
		Custom: cfg.Theme.Custom,
	})
	bind := &model.Binding{Kind: model.BindPane, PaneID: "%5"}
	shellTask := model.Task{ID: "pane-5", Owner: model.OwnerShell, Title: "repo", Group: "repo", Bind: bind}
	cases := []struct {
		command string
		text    string
		status  string
		icon    string
	}{
		{"bash", "bash", "waiting", "W"},
		{"vim", "vim", "working", "⠋⠙"},
		{"claude", "claude", "working", "⠋⠙"},
	}
	for _, c := range cases {
		panes := []tmux.Pane{{PaneID: "%5", Command: c.command, Path: "/x/repo"}}
		rows := roster(cfg, &model.State{Tasks: []model.Task{shellTask}}, panes, "")
		if len(rows) != 1 {
			t.Fatalf("%s: rows = %d", c.command, len(rows))
		}
		vars := rowVars(rows[0], false, cfg, pal, 0, 0)
		if vars["primary"] != c.text {
			t.Errorf("%s: primary = %q, want %q", c.command, vars["primary"], c.text)
		}
		if vars["status"] != c.status {
			t.Errorf("%s: status = %q, want %q", c.command, vars["status"], c.status)
		}
		// The icon carries its own colour markup; only the glyph matters.
		if !strings.Contains(vars["status_icon"], c.icon) {
			t.Errorf("%s: icon = %q, want %q", c.command, vars["status_icon"], c.icon)
		}
	}
}

func TestLongTitleTruncated(t *testing.T) {
	cfg := testConfig()
	st := &model.State{Tasks: []model.Task{{ID: "a", Title: strings.Repeat("longtitle", 30)}}}
	out := Frame(Options{Config: cfg, State: st, Width: 43, Height: 20})
	checkPane(t, out, 43, 20)
	first := strings.TrimRight(strings.Split(format.Strip(out), "\n")[0], " ")
	// Measure columns, not bytes: the gutter glyph is three bytes wide.
	if w := format.VisibleWidth(first); w > 43 {
		t.Errorf("title was not truncated: %d columns", w)
	}
}

func TestEmptyStateNoTasks(t *testing.T) {
	cfg := testConfig()
	out := Frame(Options{Config: cfg, State: &model.State{}, Width: 43, Height: 20})
	checkPane(t, out, 43, 20)
	if plain := format.Strip(out); !strings.Contains(plain, "no tasks") {
		t.Errorf("empty roster did not render no tasks: %q", plain)
	}
}

func TestStaleDim(t *testing.T) {
	fresh := testConfig()
	st := &model.State{Tasks: []model.Task{{ID: "a", Title: "Stale task", ActivityTS: time.Now().Unix() - 7200}}}

	staleCfg := testConfig()
	staleCfg.Sidebar.DimStale = true
	staleCfg.Sidebar.StaleAfter = 3600

	freshOut := Frame(Options{Config: fresh, State: st, Width: 43, Height: 20})
	staleOut := Frame(Options{Config: staleCfg, State: st, Width: 43, Height: 20})

	if format.Strip(staleOut) != format.Strip(freshOut) {
		t.Errorf("dimming changed visible text:\n%q\n%q", format.Strip(staleOut), format.Strip(freshOut))
	}
	if staleOut == freshOut {
		t.Errorf("stale row was not dimmed")
	}
}

func TestFrameSizesHold(t *testing.T) {
	cfg := testConfig()
	cfg.Tasks = []config.TaskDecl{{ID: "a", Title: "One"}, {ID: "b", Title: "Two"}}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Title: "One state"},
		{ID: "c", Title: "Three extra", ActivityTS: time.Now().Unix()},
	}}
	for _, sz := range [][2]int{{43, 20}, {200, 8}, {10, 3}} {
		out := Frame(Options{Config: cfg, State: st, Width: sz[0], Height: sz[1]})
		checkPane(t, out, sz[0], sz[1])
	}
}
