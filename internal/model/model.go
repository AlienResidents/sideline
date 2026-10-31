// Package model holds the types shared by every sideline component: the
// on-disk task state, the binding of a task to a tmux target, and the line
// protocol spoken between clients and the daemon.
//
// This package has no dependencies outside the standard library. Every other
// package in sideline may import it; it imports none of them.
package model

// StateVersion is bumped only when an older reader would misread the file.
// Readers ignore unknown fields, so additive changes do not need a bump.
const StateVersion = 1

// DefaultStatus is applied to a declared task that has no state row yet.
const DefaultStatus = "todo"

// State is the whole on-disk document. The daemon is its only writer.
type State struct {
	Version   int    `json:"version"`
	UpdatedTS int64  `json:"updated_ts"`
	Tasks     []Task `json:"tasks"`
}

// Task is one row in the sidebar. A task is not an agent: it exists because a
// person or a tool put it there, and it may or may not point at a tmux pane.
type Task struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Group      string            `json:"group,omitempty"`
	Status     string            `json:"status,omitempty"`
	StatusTS   int64             `json:"status_ts,omitempty"`
	ActivityTS int64             `json:"activity_ts,omitempty"`
	Bind       *Binding          `json:"bind,omitempty"`
	Note       string            `json:"note,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
}

// Binding kinds.
const (
	BindPane     = "pane"
	BindWindow   = "window"
	BindExternal = "external"
)

// Binding points a task at something a human can jump to. All fields are
// optional: a binding with an empty value renders but does not navigate.
type Binding struct {
	// Kind is one of BindPane, BindWindow, BindExternal.
	Kind string `json:"kind"`
	// PaneID is a tmux pane id such as "%12".
	PaneID string `json:"pane_id,omitempty"`
	// Window is "session:window_index".
	Window string `json:"window,omitempty"`
	// Target is a URL or command for BindExternal.
	Target string `json:"target,omitempty"`
}

// Request is one line of JSON sent to the daemon.
type Request struct {
	// Op is one of: set, clear, list, reload, ping.
	Op string `json:"op"`
	// Task is the full task to write for set. When set, it replaces the row
	// with the same ID.
	Task *Task `json:"task,omitempty"`
	// ID selects the row for clear.
	ID string `json:"id,omitempty"`
	// Fields patches named fields on the row with ID for set when Task is nil.
	// Recognised keys: title, status, group, note, bind_pane, bind_window,
	// bind_external.
	Fields map[string]string `json:"fields,omitempty"`
}

// Response is one line of JSON returned by the daemon.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	State *State `json:"state,omitempty"`
}

// Request op names.
const (
	OpSet    = "set"
	OpClear  = "clear"
	OpList   = "list"
	OpReload = "reload"
	OpPing   = "ping"
)

// Find returns the index of the task with id, or -1.
func (s *State) Find(id string) int {
	for i := range s.Tasks {
		if s.Tasks[i].ID == id {
			return i
		}
	}
	return -1
}
