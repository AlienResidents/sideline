package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTmux puts a stand-in tmux on PATH that records its argv and answers
// list-panes with the given lines. It proves what the client actually runs,
// which is where a wrong target or a dropped flag hides.
func fakeTmux(t *testing.T, listOutput []string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shellQuoteForTest(logPath) + "\n"
	script += "if [ \"$1\" = list-panes ]; then\n  :\n"
	for _, line := range listOutput {
		script += "  printf '%s\\n' " + shellQuoteForTest(line) + "\n"
	}
	script += "fi\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// shellQuoteForTest wraps a value in single quotes for the sh script.
func shellQuoteForTest(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func callsFrom(t *testing.T, logPath string) []string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func TestSplitArgsAreDetachedAndSized(t *testing.T) {
	cases := []struct {
		dir  Direction
		flag string
	}{
		{Left, "-hb"},
		{Right, "-h"},
		{Top, "-vb"},
		{Bottom, "-v"},
	}
	for _, c := range cases {
		args := splitArgs("sess:1", c.dir, 43, "sideline render", []string{"A=1"})
		joined := strings.Join(args, " ")
		if args[0] != "split-window" || !strings.Contains(joined, c.flag) {
			t.Errorf("%v: args = %v", c.dir, args)
		}
		// -d is what stops the sidebar taking focus.
		if !strings.Contains(joined, " -d ") {
			t.Errorf("%v: split is not detached: %v", c.dir, args)
		}
		if !strings.Contains(joined, "-l 43") || !strings.Contains(joined, "-t sess:1") {
			t.Errorf("%v: size or target missing: %v", c.dir, args)
		}
		if !strings.Contains(joined, "-e A=1") {
			t.Errorf("%v: environment not passed: %v", c.dir, args)
		}
	}
}

func TestSelectPaneRunsOneCommand(t *testing.T) {
	log := fakeTmux(t, nil)
	client := Client{}
	if err := client.SelectPane("%7"); err != nil {
		t.Fatal(err)
	}
	calls := callsFrom(t, log)
	if len(calls) != 1 || calls[0] != "select-pane -t %7" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestSelectWindowAndKillPaneTargets(t *testing.T) {
	log := fakeTmux(t, nil)
	client := Client{}
	if err := client.SelectWindow("work:3"); err != nil {
		t.Fatal(err)
	}
	if err := client.KillPane("%9"); err != nil {
		t.Fatal(err)
	}
	calls := callsFrom(t, log)
	if len(calls) != 2 || calls[0] != "select-window -t work:3" || calls[1] != "kill-pane -t %9" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestListPanesParsesWhatTmuxPrints(t *testing.T) {
	line := strings.Join([]string{
		"main", "2", "work", "1", "%7", "omp", "", "Build", "/tmp",
		"120", "40", "1", "0", "renderer", "1793430000",
	}, "\x1f")
	fakeTmux(t, []string{line})
	panes, err := (Client{}).ListPanes()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes = %+v", panes)
	}
	p := panes[0]
	if p.Session != "main" || p.WindowIndex != 2 || p.PaneID != "%7" || !p.Sidebar {
		t.Fatalf("pane = %+v", p)
	}
	if p.Boot != "1793430000" {
		t.Fatalf("boot = %q", p.Boot)
	}
}

func TestClientPassesTheSocketFlag(t *testing.T) {
	log := fakeTmux(t, nil)
	client := Client{Socket: "/tmp/other"}
	if err := client.SelectPane("%1"); err != nil {
		t.Fatal(err)
	}
	calls := callsFrom(t, log)
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "-S /tmp/other select-pane -t %1") {
		t.Fatalf("calls = %v", calls)
	}
}

func TestNoServerIsRecognised(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'no server running on /tmp/x' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := (Client{}).ListPanes(); err != ErrNoServer {
		t.Fatalf("err = %v, want ErrNoServer", err)
	}
}
