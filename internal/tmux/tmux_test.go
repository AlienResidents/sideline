package tmux

import (
	"strings"
	"testing"
)

func TestParsePane(t *testing.T) {
	// One line exactly as list-panes emits it: unit separators, twelve
	// fields plus the sidebar marker.
	line := "main\x1f2\x1fwork\x1f1\x1f%7\x1fomp\x1f\x1fBuild\x1f/tmp\x1f120\x1f40\x1f1\x1f0\x1frenderer\x1f1793430000"
	p, err := parsePane(line)
	if err != nil {
		t.Fatal(err)
	}
	if p.Session != "main" || p.WindowIndex != 2 || p.WindowName != "work" || p.PaneIndex != 1 {
		t.Fatalf("identity = %+v", p)
	}
	if p.PaneID != "%7" || p.Command != "omp" || p.StartCommand != "" {
		t.Fatalf("command fields = %+v", p)
	}
	if p.Title != "Build" || p.Path != "/tmp" {
		t.Fatalf("title/path = %+v", p)
	}
	if p.Width != 120 || p.Height != 40 {
		t.Fatalf("size = %dx%d", p.Width, p.Height)
	}
	if !p.Active || p.WindowActive || !p.Sidebar {
		t.Fatalf("flags = active:%v windowActive:%v sidebar:%v", p.Active, p.WindowActive, p.Sidebar)
	}
	if p.Boot != "1793430000" {
		t.Fatalf("boot = %q", p.Boot)
	}
}

func TestParsePaneRejectsShortLine(t *testing.T) {
	// A tmux version that stops emitting a field must fail loudly rather
	// than produce a row with silently empty values.
	if _, err := parsePane("main\x1f2\x1fwork"); err == nil {
		t.Fatal("short line accepted")
	}
}

func TestPaneTarget(t *testing.T) {
	p := Pane{Session: "main", WindowIndex: 3}
	if got := p.Target(); got != "main:3" {
		t.Fatalf("Target() = %q", got)
	}
}

func TestSplitArgsKeepsFocus(t *testing.T) {
	// -d is what stops the sidebar from taking focus when it is created.
	for _, dir := range []Direction{Left, Right, Top, Bottom} {
		args := splitArgs("main:0", dir, 43, "sideline render", nil)
		found := false
		for _, a := range args {
			if a == "-d" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%v split args missing -d: %v", dir, args)
		}
		if args[len(args)-1] != "sideline render" {
			t.Fatalf("command must be last: %v", args)
		}
	}
}

func TestSplitArgsCarriesEnv(t *testing.T) {
	args := splitArgs("main:0", Left, 40, "sideline render",
		[]string{"SIDELINE_STATE_DIR=/tmp/s", "SIDELINE_CONFIG=/tmp/c"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-e SIDELINE_STATE_DIR=/tmp/s") ||
		!strings.Contains(joined, "-e SIDELINE_CONFIG=/tmp/c") {
		t.Fatalf("env missing from %v", args)
	}
}

func TestParseDirection(t *testing.T) {
	for s, want := range map[string]Direction{
		"left": Left, "": Left, "right": Right, "top": Top, "bottom": Bottom,
	} {
		got, err := ParseDirection(s)
		if err != nil || got != want {
			t.Fatalf("ParseDirection(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := ParseDirection("sideways"); err == nil {
		t.Fatal("unknown direction accepted")
	}
}

func TestDirectionIsVertical(t *testing.T) {
	if !Left.IsVertical() || !Right.IsVertical() {
		t.Fatal("left and right must be vertical")
	}
	if Top.IsVertical() || Bottom.IsVertical() {
		t.Fatal("top and bottom must be horizontal")
	}
}
