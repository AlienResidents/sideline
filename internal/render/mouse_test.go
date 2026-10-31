package render

import (
	"testing"

	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/tmux"
	"github.com/charmbracelet/bubbletea"
)

type stubClient struct {
	panes    []tmux.Pane
	windows  []string
	panesSel []string
}

func (s *stubClient) ListPanes() ([]tmux.Pane, error) { return s.panes, nil }
func (s *stubClient) SelectWindow(target string) error {
	s.windows = append(s.windows, target)
	return nil
}
func (s *stubClient) SelectPane(paneID string) error {
	s.panesSel = append(s.panesSel, paneID)
	return nil
}

func mouseFixture(t *testing.T) (*Model, *stubClient) {
	t.Helper()
	cfg := testConfig()
	cfg.Sidebar.Position = "left"
	cfg.Sidebar.Layout = "tiles"
	cfg.Sidebar.Width = 40
	cfg.Sidebar.Height = 12
	panes := []tmux.Pane{
		{PaneID: "%1", Session: "s", WindowIndex: 0, Command: "omp"},
		{PaneID: "%2", Session: "s", WindowIndex: 1, Command: "bash"},
	}
	st := &model.State{Tasks: []model.Task{
		{ID: "first", Title: "First", Group: "g1", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		{ID: "second", Title: "Second", Group: "g2", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
	}}
	cl := &stubClient{panes: panes}
	m := &Model{cfg: cfg, state: st, panes: panes, client: cl, width: 40, height: 12, layout: "tiles"}
	return m, cl
}

// lineOf returns the first line the frame maps to id.
func lineOf(ids []string, id string) int {
	for i, got := range ids {
		if got == id {
			return i
		}
	}
	return -1
}

func TestFrameWithMapTiles(t *testing.T) {
	m, _ := mouseFixture(t)
	m.View()
	ids := m.lineIDs
	if len(ids) != 12 {
		t.Fatalf("line map has %d entries, want the frame height", len(ids))
	}
	first, second := lineOf(ids, "first"), lineOf(ids, "second")
	if first < 0 || second < 0 {
		t.Fatalf("rows missing from the map: %v", ids)
	}
	if first >= second {
		t.Fatalf("first row at %d, second at %d", first, second)
	}
	// Every line of a block maps to its row, and the separator between
	// blocks maps to nothing.
	for i := first; i < first+2; i++ {
		if ids[i] != "first" {
			t.Errorf("line %d = %q, want first", i, ids[i])
		}
	}
	gap := first + 2
	if ids[gap] != "" {
		t.Errorf("separator line %d = %q, want empty", gap, ids[gap])
	}
	// Padding below the last block maps to nothing either.
	if ids[len(ids)-1] != "" {
		t.Errorf("padding maps to %q", ids[len(ids)-1])
	}
}

func TestFrameWithMapCompact(t *testing.T) {
	m, _ := mouseFixture(t)
	m.layout = "compact"
	m.View()
	ids := m.lineIDs
	if ids[0] != "first" || ids[1] != "second" {
		t.Fatalf("compact map = %v", ids[:3])
	}
}

func TestFrameWithMapHorizontal(t *testing.T) {
	m, _ := mouseFixture(t)
	m.cfg.Sidebar.Position = "top"
	m.View()
	ids := m.lineIDs
	if ids[0] != "first" {
		t.Fatalf("horizontal map starts with %q", ids[0])
	}
	// testConfig's horizontal template is one line, and horizontal blocks
	// have no separator, so the second row starts on the next line.
	if lineOf(ids, "second") != 1 {
		t.Fatalf("second row at %d, want 1", lineOf(ids, "second"))
	}
}

func TestClickSelectsTheRowAndJumps(t *testing.T) {
	m, cl := mouseFixture(t)
	m.selected = "first"
	m.View()
	y := lineOf(m.lineIDs, "second")

	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: y})
	got := next.(*Model)
	if got.selected != "second" {
		t.Fatalf("selection = %q after click on %q", got.selected, m.lineIDs[y])
	}
	// The window is selected before the pane: select-pane alone does not
	// change the current window.
	if len(cl.windows) != 1 || cl.windows[0] != "s:1" {
		t.Fatalf("window targets = %v, want [s:1]", cl.windows)
	}
	if len(cl.panesSel) != 1 || cl.panesSel[0] != "%2" {
		t.Fatalf("pane targets = %v, want [%%2]", cl.panesSel)
	}
}

func TestClickOnASeparatorDoesNothing(t *testing.T) {
	m, cl := mouseFixture(t)
	m.selected = "first"
	m.View()
	y := lineOf(m.lineIDs, "first") + 2
	if m.lineIDs[y] != "" {
		t.Fatalf("line %d is %q, expected the separator", y, m.lineIDs[y])
	}
	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: y})
	got := next.(*Model)
	if got.selected != "first" || len(cl.windows) != 0 {
		t.Fatalf("a click on padding acted: selected=%q windows=%v", got.selected, cl.windows)
	}
}

func TestClickIgnoresOtherButtonsAndReleases(t *testing.T) {
	m, cl := mouseFixture(t)
	m.selected = "first"
	m.View()
	y := lineOf(m.lineIDs, "second")
	for _, msg := range []tea.MouseMsg{
		{Action: tea.MouseActionPress, Button: tea.MouseButtonRight, Y: y},
		{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, Y: y},
		{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, Y: y},
	} {
		next, _ := m.Update(msg)
		if next.(*Model).selected != "first" {
			t.Fatalf("%+v changed the selection", msg)
		}
	}
	if len(cl.windows) != 0 {
		t.Fatalf("non-press events jumped: %v", cl.windows)
	}
}

func TestClickOutsideTheMapIsIgnored(t *testing.T) {
	m, cl := mouseFixture(t)
	m.selected = "first"
	m.View()
	for _, y := range []int{-1, len(m.lineIDs), 999} {
		next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: y})
		if next.(*Model).selected != "first" {
			t.Fatalf("click at y=%d changed the selection", y)
		}
	}
	if len(cl.windows) != 0 {
		t.Fatalf("click outside the map jumped: %v", cl.windows)
	}
}

func TestJumpWindowTargetFallsBackToThePaneID(t *testing.T) {
	r := row{id: "x", bind: &model.Binding{Kind: model.BindPane, PaneID: "%9"}}
	if got := paneWindowTarget(r); got != "%9" {
		t.Fatalf("target = %q, want the pane id", got)
	}
	r.pane = &tmux.Pane{Session: "work", WindowIndex: 3}
	if got := paneWindowTarget(r); got != "work:3" {
		t.Fatalf("target = %q, want work:3", got)
	}
}
