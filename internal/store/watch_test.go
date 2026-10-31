package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/AlienResidents/sideline/internal/model"
)

func TestWatchDeliversAChangedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Save(path, &model.State{Tasks: []model.Task{{ID: "a"}}}); err != nil {
		t.Fatal(err)
	}
	ch, stop, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if err := Save(path, &model.State{Tasks: []model.Task{{ID: "a"}, {ID: "b"}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case st := <-ch:
		if st == nil || len(st.Tasks) != 2 {
			t.Fatalf("watched state = %+v", st)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no state delivered within 3s")
	}
}

func TestWatchIgnoresOtherFilesInTheDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := Save(path, &model.State{Tasks: []model.Task{{ID: "a"}}}); err != nil {
		t.Fatal(err)
	}
	ch, stop, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if err := Save(filepath.Join(dir, "unrelated.json"), &model.State{Tasks: []model.Task{{ID: "x"}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case st := <-ch:
		t.Fatalf("a write to another file was delivered: %+v", st)
	case <-time.After(400 * time.Millisecond):
	}
}

func TestWatchStops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Save(path, &model.State{}); err != nil {
		t.Fatal(err)
	}
	ch, stop, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	// The channel closes once the watcher exits, so a reader is released
	// rather than left blocked after a reload or a shutdown.
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("a state arrived after stop")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the channel did not close after stop")
	}
}
