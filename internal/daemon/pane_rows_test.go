package daemon

import (
	"testing"

	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/tmux"
)

func TestPlanPaneRowsCreatesOnePerBarePane(t *testing.T) {
	panes := []tmux.Pane{
		{PaneID: "%1", Command: "bash", Path: "/Users/x/repo", WindowName: "w0"},
		{PaneID: "%2", Command: "vim", Path: "/Users/x/repo"},
		// The sidebar pane is not a task and never gets a row.
		{PaneID: "%9", Command: "sideline", Sidebar: true},
	}
	creates, clears := planPaneRows(panes, nil, "100")
	if len(clears) != 0 {
		t.Fatalf("clears = %v", clears)
	}
	if len(creates) != 2 {
		t.Fatalf("creates = %d, want 2", len(creates))
	}
	byID := map[string]model.Task{}
	for _, c := range creates {
		byID[c.ID] = c
	}
	r, ok := byID["pane-1"]
	if !ok {
		t.Fatalf("no row for %%1: %v", byID)
	}
	if r.Owner != model.OwnerShell {
		t.Errorf("owner = %q", r.Owner)
	}
	if r.Bind == nil || r.Bind.Kind != model.BindPane || r.Bind.PaneID != "%1" || r.Bind.Boot != "100" {
		t.Errorf("bind = %+v", r.Bind)
	}
	if r.Title != "repo" || r.Group != "repo" {
		t.Errorf("title/group = %q/%q", r.Title, r.Group)
	}
}

func TestPlanPaneRowsCoversHarnessWithNoWriter(t *testing.T) {
	// A claude or codex pane has no status writer, but it is still a pane in
	// the roster. The renderer reads the live command off the pane.
	panes := []tmux.Pane{{PaneID: "%4", Command: "claude", Path: "/x/app"}}
	creates, _ := planPaneRows(panes, nil, "1")
	if len(creates) != 1 || creates[0].ID != "pane-4" {
		t.Fatalf("creates = %+v", creates)
	}
}

func TestPlanPaneRowsLeavesOwnedPaneAlone(t *testing.T) {
	panes := []tmux.Pane{{PaneID: "%1", Command: "omp", Path: "/x/repo"}}
	tasks := []model.Task{{
		ID:    "repo-1",
		Title: "Build the thing",
		Bind:  &model.Binding{Kind: model.BindPane, PaneID: "%1"},
	}}
	creates, clears := planPaneRows(panes, tasks, "100")
	if len(creates) != 0 || len(clears) != 0 {
		t.Fatalf("creates = %v clears = %v", creates, clears)
	}
}

func TestPlanPaneRowsKeepsExistingShellRow(t *testing.T) {
	panes := []tmux.Pane{{PaneID: "%1", Command: "bash", Path: "/x/repo"}}
	tasks := []model.Task{{
		ID:    "pane-1",
		Owner: model.OwnerShell,
		Bind:  &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "100"},
	}}
	creates, clears := planPaneRows(panes, tasks, "100")
	// Re-creating the row every tick would reset its activity time.
	if len(creates) != 0 || len(clears) != 0 {
		t.Fatalf("creates = %v clears = %v", creates, clears)
	}
}

func TestPlanPaneRowsClearsDeadPane(t *testing.T) {
	tasks := []model.Task{{
		ID:    "pane-1",
		Owner: model.OwnerShell,
		Bind:  &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "100"},
	}}
	creates, clears := planPaneRows(nil, tasks, "100")
	if len(creates) != 0 || len(clears) != 1 || clears[0] != "pane-1" {
		t.Fatalf("creates = %v clears = %v", creates, clears)
	}
}

func TestPlanPaneRowsClearsOnTakeover(t *testing.T) {
	// An agent claimed the pane after the shell row was written.
	panes := []tmux.Pane{{PaneID: "%1", Command: "omp", Path: "/x/repo"}}
	tasks := []model.Task{
		{ID: "pane-1", Owner: model.OwnerShell, Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "100"}},
		{ID: "repo-1", Title: "Agent task", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1"}},
	}
	creates, clears := planPaneRows(panes, tasks, "100")
	if len(creates) != 0 {
		t.Fatalf("creates = %v", creates)
	}
	if len(clears) != 1 || clears[0] != "pane-1" {
		t.Fatalf("clears = %v", clears)
	}
}

func TestPlanPaneRowsClearsOnBootChange(t *testing.T) {
	// Same pane id, new tmux server: the id points at a different pane.
	panes := []tmux.Pane{{PaneID: "%1", Command: "bash", Path: "/x/repo"}}
	tasks := []model.Task{{
		ID:    "pane-1",
		Owner: model.OwnerShell,
		Bind:  &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "100"},
	}}
	_, clears := planPaneRows(panes, tasks, "200")
	if len(clears) != 1 || clears[0] != "pane-1" {
		t.Fatalf("clears = %v", clears)
	}
}

func TestPlanPaneRowsClearsUnboundShellRow(t *testing.T) {
	tasks := []model.Task{{ID: "pane-1", Owner: model.OwnerShell}}
	_, clears := planPaneRows(nil, tasks, "100")
	if len(clears) != 1 || clears[0] != "pane-1" {
		t.Fatalf("clears = %v", clears)
	}
}

func TestPlanPaneRowsKeepsUnboundUserRow(t *testing.T) {
	// A row with no pane binding is a task in its own right, so it is never
	// swept.
	tasks := []model.Task{{ID: "inbox", Title: "Keep me", Status: "working"}}
	_, clears := planPaneRows(nil, tasks, "100")
	if len(clears) != 0 {
		t.Fatalf("clears = %v", clears)
	}
}

func TestPlanPaneRowsKeepsBoundRowWhilePaneLives(t *testing.T) {
	panes := []tmux.Pane{{PaneID: "%1", Command: "omp", Path: "/x/repo", Boot: "100"}}
	tasks := []model.Task{{
		ID:    "repo-1",
		Title: "Agent task",
		Bind:  &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "100"},
	}}
	if _, clears := planPaneRows(panes, tasks, "100"); len(clears) != 0 {
		t.Fatalf("clears = %v", clears)
	}
}

func TestPlanPaneRowsClearsAgentRowForDeadPane(t *testing.T) {
	// Regression: a writer's row outlived its pane and sat in the roster.
	tasks := []model.Task{{
		ID:    "tmp-202",
		Title: "tmp-202",
		Bind:  &model.Binding{Kind: model.BindPane, PaneID: "%202", Boot: "100"},
	}}
	_, clears := planPaneRows(nil, tasks, "100")
	if len(clears) != 1 || clears[0] != "tmp-202" {
		t.Fatalf("clears = %v", clears)
	}
}
