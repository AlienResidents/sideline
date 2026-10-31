package daemon

import (
	"path/filepath"
	"strings"

	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/tmux"
)

// paneRowID is the id of the row the daemon keeps for one bare pane. Pane ids
// are unique inside a tmux server, and a row that outlives its pane is removed
// by the sweep, so no directory or session name goes into the id.
func paneRowID(paneID string) string {
	return "pane-" + strings.TrimPrefix(paneID, "%")
}

// planPaneRows decides which shell-owned rows to create and which to remove.
//
// A pane gets a row only when no other row binds it. That is what keeps a
// shell from duplicating an agent's row: the agent's writer owns the pane, and
// the pane stays in the roster through that row. When the agent's row is
// cleared by hand, the next tick creates the shell row again.
//
// A shell row is removed when its pane is gone, when the pane id has been
// reused by a new tmux server (the boot time no longer matches), or when a
// real writer has taken the pane over.
//
// The plan is pure so the ownership rules are testable without a daemon.
func planPaneRows(panes []tmux.Pane, tasks []model.Task, boot string) (creates []model.Task, clears []string) {
	claimed := make(map[string]bool, len(tasks))
	shell := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t.Bind == nil || t.Bind.Kind != model.BindPane || t.Bind.PaneID == "" {
			continue
		}
		if t.Owner == model.OwnerShell {
			shell[paneRowID(t.Bind.PaneID)] = true
			continue
		}
		claimed[t.Bind.PaneID] = true
	}

	live := make(map[string]bool, len(panes))
	for _, p := range panes {
		if p.Sidebar || p.PaneID == "" {
			continue
		}
		live[p.PaneID] = true
		if claimed[p.PaneID] || shell[paneRowID(p.PaneID)] {
			continue
		}
		creates = append(creates, model.Task{
			ID:     paneRowID(p.PaneID),
			Title:  paneName(p),
			Group:  paneName(p),
			Status: "waiting",
			Owner:  model.OwnerShell,
			Bind: &model.Binding{
				Kind:   model.BindPane,
				PaneID: p.PaneID,
				Boot:   boot,
			},
		})
	}

	for _, t := range tasks {
		if t.Owner != model.OwnerShell {
			continue
		}
		b := t.Bind
		switch {
		case b == nil || b.Kind != model.BindPane || b.PaneID == "":
			clears = append(clears, t.ID)
		case !live[b.PaneID]:
			clears = append(clears, t.ID)
		case claimed[b.PaneID]:
			clears = append(clears, t.ID)
		case b.Boot != "" && boot != "" && b.Boot != boot:
			clears = append(clears, t.ID)
		}
	}
	return creates, clears
}

// paneName is the name a shell row carries: the last path element of the
// pane's working directory, falling back to the window name and then the
// process. The renderer replaces it with the live foreground command while the
// row is shown.
func paneName(p tmux.Pane) string {
	if base := filepath.Base(p.Path); base != "" && base != "." && base != "/" {
		return base
	}
	if p.WindowName != "" {
		return p.WindowName
	}
	return p.PaneID
}
