package render

import (
	"testing"

	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/tmux"
)

func TestSelectionStartsOnFirstRow(t *testing.T) {
	m := &Model{cfg: testConfig(), state: &model.State{Tasks: []model.Task{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
	}}}
	m.ensureSelection()
	if m.selected != "a" {
		t.Fatalf("selection = %q, want a", m.selected)
	}
	// The selected task disappearing must not leave a dead cursor.
	m.state = &model.State{Tasks: []model.Task{{ID: "b", Title: "B"}}}
	m.ensureSelection()
	if m.selected != "b" {
		t.Fatalf("selection = %q after removal, want b", m.selected)
	}
	// No rows at all means no selection, not a stale id.
	m.state = &model.State{}
	m.ensureSelection()
	if m.selected != "" {
		t.Fatalf("selection = %q with no rows", m.selected)
	}
}

func TestSelectionFollowsLiveWindowUntilMoved(t *testing.T) {
	// Two rows, the second bound to the window in front. The cursor must
	// land on that row so the band and the bright gutter bar agree.
	panes := []tmux.Pane{
		{PaneID: "%1", Active: true, WindowActive: false, Session: "s", WindowIndex: 0},
		{PaneID: "%2", Active: true, WindowActive: true, Session: "s", WindowIndex: 1},
	}
	m := &Model{cfg: testConfig(), panes: panes, state: &model.State{Tasks: []model.Task{
		{ID: "back", Title: "A", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
		{ID: "front", Title: "B", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2"}},
	}}}
	m.ensureSelection()
	if m.selected != "front" {
		t.Fatalf("selection = %q, want front", m.selected)
	}
	// Once the user moves, the cursor is theirs and must not be stolen back.
	m.userMoved = true
	m.selected = "back"
	m.ensureSelection()
	if m.selected != "back" {
		t.Fatalf("selection = %q after user move", m.selected)
	}
}

func TestPruneBindsDropsOtherServerLifetime(t *testing.T) {
	m := &Model{boot: "100"}
	st := &model.State{Tasks: []model.Task{
		{ID: "same", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "100"}},
		{ID: "stale", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2", Boot: "99"}},
		{ID: "unversioned", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%3"}},
		{ID: "window", Bind: &model.Binding{Kind: model.BindWindow, Window: "main:1"}},
	}}
	m.pruneBinds(st)

	if st.Tasks[0].Bind == nil {
		t.Fatal("binding from the live server was dropped")
	}
	if st.Tasks[1].Bind != nil {
		t.Fatal("binding from a dead server survived")
	}
	// A binding written before boot ids existed is kept: it may still be
	// valid, and dropping it would lose a working row on upgrade.
	if st.Tasks[2].Bind == nil {
		t.Fatal("unversioned pane binding was dropped")
	}
	if st.Tasks[3].Bind == nil {
		t.Fatal("window binding was dropped")
	}
}

func TestPruneBindsNoServerIsNoop(t *testing.T) {
	m := &Model{}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "1"}},
	}}
	m.pruneBinds(st)
	if st.Tasks[0].Bind == nil {
		t.Fatal("pruned without a known server start")
	}
}

func TestTickRefreshesPanesFromTmux(t *testing.T) {
	m, cl := mouseFixture(t)
	cl.panes = []tmux.Pane{{PaneID: "%9", Command: "vim"}}
	next, cmd := m.Update(tickMsg{})
	got := next.(*Model)
	if len(got.panes) != 1 || got.panes[0].PaneID != "%9" {
		t.Fatalf("panes after tick = %+v", got.panes)
	}
	// The next tick is scheduled from the tick itself.
	if cmd == nil {
		t.Fatal("tick did not schedule a follow-up")
	}
}

func TestStateMessageReplacesTheRoster(t *testing.T) {
	m, _ := mouseFixture(t)
	m.selected = "second"
	st := &model.State{Tasks: []model.Task{{ID: "first", Title: "First"}}}
	next, _ := m.Update(stateMsg{state: st})
	got := next.(*Model)
	if got.state != st {
		t.Fatal("state was not replaced")
	}
	// The selected row disappeared, so the cursor falls back to a live row.
	if got.selected != "first" {
		t.Fatalf("selection = %q, want first", got.selected)
	}
}
