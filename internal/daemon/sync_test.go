package daemon

import (
	"path/filepath"
	"testing"

	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/store"
	"github.com/AlienResidents/sideline/internal/tmux"
)

func newTestDaemon(t *testing.T) (*Daemon, *store.Store) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &Daemon{st: st}, st
}

func TestSyncPaneRowsWritesARowForABarePane(t *testing.T) {
	d, st := newTestDaemon(t)
	d.syncPaneRows([]tmux.Pane{{PaneID: "%1", Command: "bash", Path: "/x/repo", Boot: "100"}})

	state := st.State()
	if len(state.Tasks) != 1 {
		t.Fatalf("tasks = %+v", state.Tasks)
	}
	got := state.Tasks[0]
	if got.ID != "pane-1" || got.Owner != model.OwnerShell {
		t.Fatalf("row = %+v", got)
	}
	if got.Bind == nil || got.Bind.PaneID != "%1" || got.Bind.Boot != "100" {
		t.Fatalf("bind = %+v", got.Bind)
	}
}

func TestSyncPaneRowsClearsWhenThePaneGoes(t *testing.T) {
	d, st := newTestDaemon(t)
	panes := []tmux.Pane{{PaneID: "%1", Command: "bash", Path: "/x/repo", Boot: "100"}}
	d.syncPaneRows(panes)
	if len(st.State().Tasks) != 1 {
		t.Fatal("the row was not written")
	}
	d.syncPaneRows(nil)
	if n := len(st.State().Tasks); n != 0 {
		t.Fatalf("tasks after the pane closed = %d, want 0", n)
	}
}

func TestSyncPaneRowsLeavesATaskWithNoPaneAlone(t *testing.T) {
	d, st := newTestDaemon(t)
	if _, err := st.Apply(model.Request{Op: model.OpSet, Task: &model.Task{ID: "inbox", Title: "Inbox"}}); err != nil {
		t.Fatal(err)
	}
	d.syncPaneRows(nil)
	if n := len(st.State().Tasks); n != 1 {
		t.Fatalf("an unbound task was swept: %+v", st.State().Tasks)
	}
}
