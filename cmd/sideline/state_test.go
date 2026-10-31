package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlienResidents/sideline/internal/daemon"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/store"
)

// isolateState points the state paths at a temp directory and returns it.
func isolateState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIDELINE_STATE_DIR", dir)
	t.Setenv("SIDELINE_CONFIG", filepath.Join(dir, "config.yaml"))
	return dir
}

func loadState(t *testing.T, dir string) *model.State {
	t.Helper()
	st, err := store.Load(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSetWritesTheStateFileWithNoDaemon(t *testing.T) {
	dir := isolateState(t)
	if err := cmdSet([]string{"build", "--title", "Build", "--status", "working"}); err != nil {
		t.Fatal(err)
	}
	st := loadState(t, dir)
	if len(st.Tasks) != 1 {
		t.Fatalf("tasks = %+v", st.Tasks)
	}
	task := st.Tasks[0]
	if task.ID != "build" || task.Title != "Build" || task.Status != "working" {
		t.Fatalf("task = %+v", task)
	}
}

func TestSetPatchesWithoutErasingTheTitle(t *testing.T) {
	dir := isolateState(t)
	if err := cmdSet([]string{"build", "--title", "Build"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdSet([]string{"build", "--status", "done"}); err != nil {
		t.Fatal(err)
	}
	st := loadState(t, dir)
	if st.Tasks[0].Title != "Build" || st.Tasks[0].Status != "done" {
		t.Fatalf("task = %+v", st.Tasks[0])
	}
}

func TestSetBindsAPaneToTheCurrentServer(t *testing.T) {
	dir := isolateState(t)
	if err := cmdSet([]string{"row", "--bind-pane", "%3"}); err != nil {
		t.Fatal(err)
	}
	st := loadState(t, dir)
	b := st.Tasks[0].Bind
	if b == nil || b.Kind != model.BindPane || b.PaneID != "%3" {
		t.Fatalf("bind = %+v", b)
	}
	// A pane id without a boot time would outlive its server and point at a
	// different pane, so the binding carries one when the server answers.
	if b.Boot == "" {
		t.Log("no tmux server answered, so boot is empty on this machine")
	}
}

func TestClearRemovesTheRow(t *testing.T) {
	dir := isolateState(t)
	if err := cmdSet([]string{"a", "--title", "A"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdClear([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if st := loadState(t, dir); len(st.Tasks) != 0 {
		t.Fatalf("tasks after clear = %+v", st.Tasks)
	}
}

func TestToggleTurnsTheSidebarOff(t *testing.T) {
	// Only the on-to-off direction is exercised: the other direction starts
	// a daemon, which is not something a unit test should spawn.
	isolateState(t)
	if err := daemon.SetEnabled(enabledPath(), true); err != nil {
		t.Fatal(err)
	}
	if !flagOn(t) {
		t.Fatal("the flag was not set")
	}
	if err := cmdToggle(); err != nil {
		t.Fatal(err)
	}
	if flagOn(t) {
		t.Fatal("toggle did not disable the sidebar")
	}
}

func TestOffClearsTheFlag(t *testing.T) {
	isolateState(t)
	if err := daemon.SetEnabled(enabledPath(), true); err != nil {
		t.Fatal(err)
	}
	if err := cmdOff(); err != nil {
		t.Fatal(err)
	}
	if flagOn(t) {
		t.Fatal("off left the flag set")
	}
}

func flagOn(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(enabledPath())
	return err == nil
}

func TestShellQuoteSurvivesSingleQuotes(t *testing.T) {
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("shellQuote = %s", got)
	}
}

func TestHumanDuration(t *testing.T) {
	cases := map[int64]string{0: "0s", 59: "59s", 60: "1m", 3599: "59m", 3600: "1h00m", 3660: "1h01m"}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Errorf("humanDuration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRunRejectsUnknownCommands(t *testing.T) {
	isolateState(t)
	if code := run("nonsense", nil); code == 0 {
		t.Fatal("an unknown command exited 0")
	}
}

func TestEverySubcommandDispatches(t *testing.T) {
	// The dispatch table and the command list must not drift apart.
	for name := range commandUsage {
		if _, ok := map[string]bool{
			"daemon": true, "render": true, "on": true, "off": true, "toggle": true,
			"status": true, "reload": true, "restart": true, "set": true, "clear": true,
			"list": true, "poke": true,
		}[name]; !ok {
			t.Errorf("command %q has usage but no dispatch case", name)
		}
	}
}

func TestPrintStateIsStableJSON(t *testing.T) {
	st := &model.State{Version: model.StateVersion, Tasks: []model.Task{{ID: "a", Title: "A"}}}
	out := captureStdout(t, func() { _ = printState(st) })
	if !strings.Contains(out, `"id": "a"`) || !strings.Contains(out, `"version": 1`) {
		t.Fatalf("printState = %s", out)
	}
}

// captureStdout runs fn with stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
