// Package render draws the task roster sidebar: Frame is a pure function
// from config, state and pane list to the exact pane-sized text, and Model
// is the bubbletea program that keeps it fresh in the pane.
package render

import (
	"sort"
	"strings"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/tmux"
)

// row is one merged task row: the config declaration overlaid with the live
// state, plus the pane the row is bound to.
type row struct {
	id         string
	title      string
	group      string
	status     string
	note       string
	activityTS int64
	bind       *model.Binding
	pane       *tmux.Pane
	declared   bool
	order      int
}

// buildRows merges the config declarations with the live state. Declared rows
// come first in file order; undeclared state rows follow, sorted by id so the
// roster stays deterministic across writes.
func buildRows(cfg *config.Config, state *model.State, panes []tmux.Pane) []row {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if state == nil {
		state = &model.State{}
	}
	byID := make(map[string]int, len(state.Tasks))
	for i := range state.Tasks {
		byID[state.Tasks[i].ID] = i
	}
	var rows []row
	seen := make(map[string]bool, len(cfg.Tasks))
	for i, d := range cfg.Tasks {
		if d.ID == "" {
			continue
		}
		seen[d.ID] = true
		r := row{
			id:       d.ID,
			title:    d.Title,
			group:    d.Group,
			status:   d.Status,
			note:     d.Note,
			declared: true,
			order:    i,
		}
		if j, ok := byID[d.ID]; ok {
			mergeState(&r, &state.Tasks[j])
		}
		rows = append(rows, r)
	}
	var rest []model.Task
	for _, t := range state.Tasks {
		if t.ID != "" && !seen[t.ID] {
			rest = append(rest, t)
		}
	}
	sort.Slice(rest, func(a, b int) bool { return rest[a].ID < rest[b].ID })
	for _, t := range rest {
		r := row{id: t.ID}
		mergeState(&r, &t)
		rows = append(rows, r)
	}
	for i := range rows {
		if rows[i].status == "" {
			rows[i].status = model.DefaultStatus
		}
		rows[i].pane = paneFor(rows[i].bind, panes)
	}
	return rows
}

// mergeState overlays one live state row onto a declaration. Non-empty state
// fields win field by field; an empty status falls back to the declaration,
// and a still-empty one to model.DefaultStatus in buildRows.
func mergeState(r *row, t *model.Task) {
	if t == nil {
		return
	}
	if t.Title != "" {
		r.title = t.Title
	}
	if t.Group != "" {
		r.group = t.Group
	}
	if t.Status != "" {
		r.status = t.Status
	}
	if t.Note != "" {
		r.note = t.Note
	}
	if t.Bind != nil {
		r.bind = t.Bind
	}
	r.activityTS = t.ActivityTS
}

// paneFor resolves a pane binding against the current pane list. Only a pane
// binding with a pane id can resolve; anything else renders unbound.
func paneFor(bind *model.Binding, panes []tmux.Pane) *tmux.Pane {
	if bind == nil || bind.Kind != model.BindPane || bind.PaneID == "" {
		return nil
	}
	for i := range panes {
		if panes[i].PaneID == bind.PaneID {
			return &panes[i]
		}
	}
	return nil
}

// filterRows drops rows that do not match a case-insensitive substring over
// id, title or group. An empty filter keeps everything.
func filterRows(rows []row, filter string) []row {
	if filter == "" {
		return rows
	}
	needle := strings.ToLower(filter)
	out := rows[:0:0]
	for _, r := range rows {
		hay := strings.ToLower(r.id + " " + r.title + " " + r.group)
		if strings.Contains(hay, needle) {
			out = append(out, r)
		}
	}
	return out
}

// sortRows orders rows by the configured sort mode.
func sortRows(rows []row, mode string) {
	switch mode {
	case "activity":
		// Newest activity first; ties break on title so equal-stamped rows
		// (unstarted rows are all zero) still order stably.
		sort.SliceStable(rows, func(a, b int) bool {
			if rows[a].activityTS != rows[b].activityTS {
				return rows[a].activityTS > rows[b].activityTS
			}
			return strings.ToLower(rows[a].title) < strings.ToLower(rows[b].title)
		})
	case "title":
		sort.SliceStable(rows, func(a, b int) bool {
			return strings.ToLower(rows[a].title) < strings.ToLower(rows[b].title)
		})
	case "window":
		// Bound rows by the pane's window, unbound rows last.
		sort.SliceStable(rows, func(a, b int) bool {
			pa, pb := rows[a].pane, rows[b].pane
			if pa == nil && pb == nil {
				return strings.ToLower(rows[a].title) < strings.ToLower(rows[b].title)
			}
			if pa == nil {
				return false
			}
			if pb == nil {
				return true
			}
			if pa.WindowIndex != pb.WindowIndex {
				return pa.WindowIndex < pb.WindowIndex
			}
			if pa.Session != pb.Session {
				return pa.Session < pb.Session
			}
			return strings.ToLower(rows[a].title) < strings.ToLower(rows[b].title)
		})
	default:
		// "config": declared rows keep file order, undeclared follow by id.
		// buildRows already produced that order, so this only has to keep it.
		sort.SliceStable(rows, func(a, b int) bool {
			if rows[a].declared != rows[b].declared {
				return rows[a].declared
			}
			if rows[a].declared {
				return rows[a].order < rows[b].order
			}
			return rows[a].id < rows[b].id
		})
	}
}

// roster runs the full row pipeline: merge, filter, sort.
func roster(cfg *config.Config, state *model.State, panes []tmux.Pane, filter string) []row {
	rows := filterRows(buildRows(cfg, state, panes), filter)
	sortRows(rows, cfg.Sidebar.Sort)
	return rows
}
