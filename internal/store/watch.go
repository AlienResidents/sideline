package store

import (
	"path/filepath"

	"github.com/AlienResidents/sideline/internal/model"
	"github.com/fsnotify/fsnotify"
)

// Watch reports the state loaded from path after every change to it,
// including Save's atomic rename (a Create event on the path). The stop
// func ends the watch; the channel is closed after stop.
func Watch(path string) (<-chan *model.State, func(), error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	// Watch the directory: an atomic rename surfaces as an event on the
	// path itself, which a watch on the old inode would miss.
	if err := w.Add(filepath.Dir(path)); err != nil {
		w.Close()
		return nil, nil, err
	}
	ch := make(chan *model.State, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(ch)
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				// Rename swaps the inode, so match by name and reload.
				if ev.Name != path {
					continue
				}
				if st, err := load(path); err == nil {
					select {
					case ch <- st:
					case <-stopped:
						return
					}
				}
			case <-w.Errors:
				// Errors carry no new state; keep watching.
			case <-stopped:
				return
			}
		}
	}()
	stop := func() {
		close(stopped)
		w.Close()
	}
	return ch, stop, nil
}
