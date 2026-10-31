package daemon

import (
	"sort"
	"strconv"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/tmux"
)

// ActionKind names one reconciliation step.
type ActionKind string

const (
	// ActionAdd creates a missing sidebar pane in a window that has none.
	ActionAdd ActionKind = "add"
	// ActionResize corrects a sidebar pane whose size drifted.
	ActionResize ActionKind = "resize"
	// ActionRemove kills a sidebar pane for a window that has no content
	// pane left, or a duplicated sidebar in the same window.
	ActionRemove ActionKind = "remove"
)

// Action is one planned tmux operation. Target is the pane id for Resize and
// Remove, and the "session:window_index" target for Add.
type Action struct {
	Kind   ActionKind
	Target string
	PaneID string
	Size   int
}

// windowKey identifies the window a pane belongs to.
type windowKey struct {
	Session     string
	WindowIndex int
}

// sizeFor returns the sidebar pane size for this position.
func sizeFor(cfg *config.Config) int {
	if cfg.Sidebar.Position == "top" || cfg.Sidebar.Position == "bottom" {
		return cfg.Sidebar.Height
	}
	return cfg.Sidebar.Width
}

// axisFor returns the resize axis for this position.
func axisFor(cfg *config.Config) tmux.Axis {
	if cfg.Sidebar.Position == "top" || cfg.Sidebar.Position == "bottom" {
		return tmux.AxisHeight
	}
	return tmux.AxisWidth
}

// plan derives the tmux operations that bring the tmux window tree in line
// with the config. It is pure: it reads cfg and panes and returns actions,
// so the placement rules are testable without a tmux server.
//
// Rules, per window that has at least one non-sidebar pane:
//   - no sidebar pane -> add one;
//   - exactly one sidebar pane -> resize it if its size or the position's
//     axis size differs from the config;
//   - more than one sidebar pane -> keep the first, remove the rest.
//
// A window with no content pane (a sidebar-only window, typically the last
// gasp of a closing window) has its sidebar removed. Panes with empty ids
// are ignored: they cannot be targeted.
func plan(cfg *config.Config, panes []tmux.Pane, enabled bool) []Action {
	if cfg == nil || !enabled {
		// Disabled: every existing sidebar pane goes, so "off" is a full
		// teardown, not just "stop adding new ones".
		var acts []Action
		for _, p := range panes {
			if p.Sidebar && p.PaneID != "" {
				acts = append(acts, Action{Kind: ActionRemove, PaneID: p.PaneID})
			}
		}
		return acts
	}

	size := sizeFor(cfg)
	axis := axisFor(cfg)

	byWin := map[windowKey][]tmux.Pane{}
	var keys []windowKey
	for _, p := range panes {
		k := windowKey{p.Session, p.WindowIndex}
		if _, ok := byWin[k]; !ok {
			keys = append(keys, k)
		}
		byWin[k] = append(byWin[k], p)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Session != keys[j].Session {
			return keys[i].Session < keys[j].Session
		}
		return keys[i].WindowIndex < keys[j].WindowIndex
	})

	var acts []Action
	for _, k := range keys {
		panes := byWin[k]
		hasContent := false
		for _, p := range panes {
			if !p.Sidebar && p.PaneID != "" {
				hasContent = true
			}
		}
		if !hasContent {
			for _, p := range panes {
				if p.Sidebar && p.PaneID != "" {
					acts = append(acts, Action{Kind: ActionRemove, PaneID: p.PaneID})
				}
			}
			continue
		}

		// First sidebar pane wins: an existing pane is never killed in
		// favour of a new one, because it holds the running renderer.
		var side *tmux.Pane
		count := 0
		for i := range panes {
			if panes[i].Sidebar && panes[i].PaneID != "" {
				count++
				if side == nil {
					side = &panes[i]
				}
			}
		}
		switch {
		case count == 0:
			// A window too small for the sidebar plus usable content would
			// fail the split on every tick, so leave it alone instead.
			if !fits(panes, axis, size) {
				continue
			}
			target := panes[0].Session + ":" + strconv.Itoa(panes[0].WindowIndex)
			acts = append(acts, Action{Kind: ActionAdd, Target: target, Size: size})
		case count == 1:
			cur := sizeOf(*side, axis)
			if cur != size {
				acts = append(acts, Action{Kind: ActionResize, PaneID: side.PaneID, Size: size})
			}
		default:
			for _, p := range panes {
				if p.Sidebar && p.PaneID != "" && p.PaneID != side.PaneID {
					acts = append(acts, Action{Kind: ActionRemove, PaneID: p.PaneID})
				}
			}
		}
	}
	return acts
}

// minContent is the smallest content pane worth leaving beside a sidebar.
const minContent = 20

// fits reports whether a window has room for the sidebar plus usable content.
// tmux refuses a split that would leave a pane narrower than its minimum, and
// a failed split is retried on every tick, so the plan skips those windows.
func fits(panes []tmux.Pane, axis tmux.Axis, size int) bool {
	best := 0
	for _, p := range panes {
		if p.Sidebar {
			continue
		}
		v := p.Width
		if axis == tmux.AxisHeight {
			v = p.Height
		}
		if v > best {
			best = v
		}
	}
	return best-size >= minContent
}

func sizeOf(p tmux.Pane, axis tmux.Axis) int {
	if axis == tmux.AxisHeight {
		return p.Height
	}
	return p.Width
}
