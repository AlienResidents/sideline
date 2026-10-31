package tmux

import "testing"

func TestParsePane(t *testing.T) {
	// One line exactly as list-panes emits it: unit separators, twelve
	// fields plus the sidebar marker.
	line := "main\x1f2\x1fwork\x1f1\x1f%7\x1fomp\x1f\x1fBuild\x1f/tmp\x1f120\x1f40\x1f1\x1f0\x1frenderer"
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
