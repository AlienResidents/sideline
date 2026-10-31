package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/tmux"
)

func testConfig() *config.Config {
	c := config.Default()
	return c
}

func pane(session string, win int, id string, sidebar bool, w, h int) tmux.Pane {
	return tmux.Pane{
		Session:     session,
		WindowIndex: win,
		PaneID:      id,
		Width:       w,
		Height:      h,
		Sidebar:     sidebar,
	}
}

func TestPlanAddForWindowWithoutSidebar(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Width = 40
	panes := []tmux.Pane{
		pane("s", 0, "%1", false, 120, 40),
	}
	acts := plan(cfg, panes, true)
	if len(acts) != 1 || acts[0].Kind != ActionAdd {
		t.Fatalf("want one add, got %+v", acts)
	}
	if acts[0].Target != "s:0" {
		t.Errorf("target = %q, want s:0", acts[0].Target)
	}
	if acts[0].Size != 40 {
		t.Errorf("size = %d, want 40", acts[0].Size)
	}
}

func TestPlanResizeWrongSize(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Width = 40
	panes := []tmux.Pane{
		pane("s", 0, "%1", false, 120, 40),
		pane("s", 0, "%2", true, 30, 40),
	}
	acts := plan(cfg, panes, true)
	if len(acts) != 1 || acts[0].Kind != ActionResize || acts[0].PaneID != "%2" || acts[0].Size != 40 {
		t.Fatalf("want resize %%2 to 40, got %+v", acts)
	}
}

func TestPlanNoopWhenCorrect(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Width = 40
	panes := []tmux.Pane{
		pane("s", 0, "%1", false, 120, 40),
		pane("s", 0, "%2", true, 40, 40),
	}
	if acts := plan(cfg, panes, true); len(acts) != 0 {
		t.Fatalf("want no actions, got %+v", acts)
	}
}

func TestPlanDuplicateSidebars(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Width = 40
	panes := []tmux.Pane{
		pane("s", 0, "%1", false, 120, 40),
		pane("s", 0, "%2", true, 40, 40),
		pane("s", 0, "%3", true, 40, 40),
	}
	acts := plan(cfg, panes, true)
	if len(acts) != 1 || acts[0].Kind != ActionRemove || acts[0].PaneID != "%3" {
		t.Fatalf("want remove %%3, got %+v", acts)
	}
}

func TestPlanSidebarOnlyWindowRemoved(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Width = 40
	panes := []tmux.Pane{
		pane("s", 0, "%1", false, 120, 40),
		pane("s", 0, "%2", true, 40, 40),
		pane("s", 1, "%3", true, 40, 40), // window 1 has no content pane
	}
	acts := plan(cfg, panes, true)
	if len(acts) != 1 || acts[0].Kind != ActionRemove || acts[0].PaneID != "%3" {
		t.Fatalf("want remove %%3, got %+v", acts)
	}
}

func TestPlanDisabledRemovesAll(t *testing.T) {
	cfg := testConfig()
	panes := []tmux.Pane{
		pane("s", 0, "%1", false, 120, 40),
		pane("s", 0, "%2", true, 40, 40),
		pane("t", 3, "%4", true, 40, 40),
	}
	acts := plan(cfg, panes, false)
	if len(acts) != 2 {
		t.Fatalf("want two removes, got %+v", acts)
	}
	for _, a := range acts {
		if a.Kind != ActionRemove {
			t.Errorf("kind = %s, want remove", a.Kind)
		}
	}
}

func TestPlanNilConfigDisabled(t *testing.T) {
	panes := []tmux.Pane{pane("s", 0, "%1", true, 40, 40)}
	if acts := plan(nil, panes, true); len(acts) != 1 || acts[0].Kind != ActionRemove {
		t.Fatalf("nil config disables, got %+v", acts)
	}
}

func TestPlanVerticalUsesHeight(t *testing.T) {
	cfg := testConfig()
	cfg.Sidebar.Position = "bottom"
	cfg.Sidebar.Height = 8
	panes := []tmux.Pane{
		pane("s", 0, "%1", false, 200, 50),
		pane("s", 0, "%2", true, 200, 10),
	}
	acts := plan(cfg, panes, true)
	if len(acts) != 1 || acts[0].Kind != ActionResize || acts[0].PaneID != "%2" || acts[0].Size != 8 {
		t.Fatalf("want resize %%2 to 8 rows, got %+v", acts)
	}
}

func TestPIDLockExcludesASecondDaemon(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.pid")
	first, err := lockPID(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := ReadPID(path); got != os.Getpid() {
		t.Fatalf("pid file = %d, want %d", got, os.Getpid())
	}
	if _, err := lockPID(path); err == nil {
		t.Fatal("second lock acquired")
	} else if err != ErrRunning {
		t.Fatalf("second lock error = %v, want ErrRunning", err)
	}
	// Releasing lets the next daemon in; a crash does the same through the
	// OS, which is why a stale pid file is not a problem.
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := lockPID(path)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	_ = second.Close()
}

func TestReadPIDMissing(t *testing.T) {
	if got := ReadPID(filepath.Join(t.TempDir(), "nope.pid")); got != 0 {
		t.Fatalf("ReadPID on a missing file = %d", got)
	}
}

func TestInfoReportsState(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Options{
		Config:      testConfig(),
		StatePath:   filepath.Join(dir, "state.json"),
		SocketPath:  filepath.Join(dir, "s.sock"),
		EnabledPath: filepath.Join(dir, "enabled"),
		Version:     "1.2.3",
		// A path with no server behind it: the test must never query the
		// developer's live tmux server and count its panes.
		TmuxSocket: filepath.Join(dir, "no-such-tmux"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.st.BumpReload(); err != nil {
		t.Fatal(err)
	}
	info := d.info()
	if !info.Running || info.Version != "1.2.3" {
		t.Fatalf("info = %+v", info)
	}
	if info.PID == 0 || info.StatePath == "" || info.ConfigPath == "" {
		t.Fatalf("info missing fields: %+v", info)
	}
	// No tmux server in a test, so the pane count is zero rather than an
	// error, and the task count comes from the state file.
	if info.Panes != 0 {
		t.Fatalf("panes = %d", info.Panes)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	// Two `sideline restart` calls racing must not close the channel twice,
	// which would panic the daemon.
	d := &Daemon{shutdown: make(chan struct{})}
	d.stop()
	d.stop()
	select {
	case <-d.shutdown:
	default:
		t.Fatal("shutdown channel not closed")
	}
}

func TestShutdownOpAnswers(t *testing.T) {
	d := &Daemon{shutdown: make(chan struct{})}
	resp := d.handle(model.Request{Op: model.OpShutdown})
	if !resp.OK {
		t.Fatalf("shutdown response = %+v", resp)
	}
	// The stop is deferred to a goroutine so the reply reaches the client
	// before the socket closes.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-d.shutdown:
			return
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	t.Fatal("shutdown channel not closed")
}

func TestEnabledFlag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "enabled")

	on, err := IsEnabled(p)
	if err != nil || on {
		t.Fatalf("missing flag: on=%v err=%v", on, err)
	}
	if err := SetEnabled(p, true); err != nil {
		t.Fatal(err)
	}
	on, err = IsEnabled(p)
	if err != nil || !on {
		t.Fatalf("after enable: on=%v err=%v", on, err)
	}
	if err := SetEnabled(p, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("disable must remove the flag, stat err=%v", err)
	}
}

func TestEnabledFlagCorruptFileIsOff(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "enabled")
	if err := os.WriteFile(p, []byte("not-a-bool\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	on, err := IsEnabled(p)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("corrupt flag file must read as disabled")
	}
}

func TestPokeSignalsTheLoop(t *testing.T) {
	d := &Daemon{wake: make(chan struct{}, 1)}
	if resp := d.handle(model.Request{Op: model.OpPoke}); !resp.OK {
		t.Fatalf("poke failed: %+v", resp)
	}
	select {
	case <-d.wake:
	default:
		t.Fatal("poke did not signal the loop")
	}
	// A burst of events still yields one pass and never blocks.
	for i := 0; i < 5; i++ {
		if resp := d.handle(model.Request{Op: model.OpPoke}); !resp.OK {
			t.Fatalf("poke %d failed", i)
		}
	}
	if len(d.wake) != 1 {
		t.Fatalf("queued wakeups = %d, want 1", len(d.wake))
	}
}
