package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlienResidents/sideline/internal/config"
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
