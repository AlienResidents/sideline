package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/AlienResidents/sideline/internal/model"
)

// maxJSONDoc caps the state file we are willing to read: 16 MiB.
const maxJSONDoc = 16 << 20

// nowSec is the clock Apply stamps rows with. Tests replace it to pin
// status_ts and activity_ts values.
var nowSec = func() int64 { return time.Now().Unix() }

// Store is the owner of the state file at Path. All mutations go through
// Apply, which serialises in-process callers and takes an exclusive flock
// so daemonless writers in other processes cannot interleave.
type Store struct {
	mu    sync.Mutex
	path  string
	lock  string
	state *model.State
}

// New returns a store for the state file at path, loading the current
// contents when the file exists.
func New(path string) (*Store, error) {
	s := &Store{
		path: path,
		lock: LockPath(path),
	}
	st, err := load(path)
	if err != nil {
		return nil, err
	}
	s.state = st
	return s, nil
}

// Path returns the state file path, for reload messages.
func (s *Store) Path() string {
	return s.path
}

// BumpReload increments the reload counter and persists it. Renderers watch
// the state file, so this is the signal that asks them to re-read the config.
func (s *Store) BumpReload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bumpErr error
	err := withFlock(s.lock, func() error {
		if cur, err := load(s.path); err == nil {
			s.state = cur
		}
		s.state.ReloadSeq++
		bumpErr = save(s.path, s.state)
		return bumpErr
	})
	if err != nil {
		return err
	}
	return bumpErr
}

// State returns a deep copy of the current state.
func (s *Store) State() *model.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyState(s.state)
}

// Load reads the state file at path. A missing file yields a fresh empty
// state, so a first run needs no special casing.
func Load(path string) (*model.State, error) {
	return load(path)
}

func load(path string) (*model.State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return freshState(), nil
		}
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}
	if len(data) > maxJSONDoc {
		return nil, fmt.Errorf("state %s is %d bytes, over the %d byte cap", path, len(data), maxJSONDoc)
	}
	if len(data) == 0 {
		return freshState(), nil
	}
	st := &model.State{}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if st.Version == 0 {
		st.Version = model.StateVersion
	}
	if st.Tasks == nil {
		st.Tasks = []model.Task{}
	}
	return st, nil
}

func freshState() *model.State {
	return &model.State{Version: model.StateVersion, Tasks: []model.Task{}}
}

func copyState(s *model.State) *model.State {
	if s == nil {
		return freshState()
	}
	c := *s
	c.Tasks = make([]model.Task, len(s.Tasks))
	copy(c.Tasks, s.Tasks)
	return &c
}

// Save writes s to path atomically: a temp file in the same directory,
// fsync, then rename. A failure anywhere leaves the previous file intact.
func Save(path string, s *model.State) error {
	return withFlock(LockPath(path), func() error {
		return save(path, s)
	})
}

func save(path string, s *model.State) error {
	s.UpdatedTS = nowSec()
	s.Version = model.StateVersion
	if s.Tasks == nil {
		s.Tasks = []model.Task{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sideline-state-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

// withFlock runs fn while holding an exclusive lock on the file at path,
// so multiple processes take turns mutating the state file.
func withFlock(path string, fn func() error) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file %s: %w", path, err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("flock %s: %w", path, err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

// Apply applies one request to the state and persists the result. On
// success the response carries the new state; on failure the file is
// untouched.
func (s *Store) Apply(req model.Request) (model.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var applyErr error
	err := withFlock(s.lock, func() error {
		// Re-read in case a daemonless writer in another process
		// changed the file since our last write.
		if cur, err := load(s.path); err == nil {
			s.state = cur
		}
		applyErr = s.apply(req, nowSec())
		if applyErr != nil {
			return nil
		}
		return save(s.path, s.state)
	})
	if err != nil {
		return model.Response{OK: false, Error: err.Error()}, err
	}
	if applyErr != nil {
		return model.Response{OK: false, Error: applyErr.Error()}, applyErr
	}
	return model.Response{OK: true, State: copyState(s.state)}, nil
}

// apply mutates the in-memory state. status_ts moves only when the status
// changes; activity_ts moves on every set.
func (s *Store) apply(req model.Request, now int64) error {
	switch req.Op {
	case model.OpSet:
		if req.Task != nil {
			s.set(*req.Task, now)
			return nil
		}
		return s.patch(req.ID, req.Fields, now)
	case model.OpClear:
		if i := s.state.Find(req.ID); i >= 0 {
			s.state.Tasks = append(s.state.Tasks[:i], s.state.Tasks[i+1:]...)
		}
		return nil
	case model.OpReload:
		if cur, err := load(s.path); err == nil {
			s.state = cur
		}
		return nil
	case model.OpList, model.OpPing, model.OpPoke:
		return nil
	default:
		return fmt.Errorf("unknown op %q", req.Op)
	}
}

func (s *Store) set(t model.Task, now int64) {
	if t.ID == "" {
		t.ID = "task"
	}
	if t.Status == "" {
		t.Status = model.DefaultStatus
	}
	if i := s.state.Find(t.ID); i >= 0 {
		if s.state.Tasks[i].Status != t.Status {
			// A status change always gets a fresh stamp.
			t.StatusTS = now
		} else {
			// An unchanged status keeps its stamp: copying a row and
			// re-sending it must not make status_ts stale.
			t.StatusTS = s.state.Tasks[i].StatusTS
		}
	} else if t.StatusTS == 0 {
		t.StatusTS = now
	}
	t.ActivityTS = now
	if i := s.state.Find(t.ID); i >= 0 {
		s.state.Tasks[i] = t
	} else {
		s.state.Tasks = append(s.state.Tasks, t)
	}
	s.state.UpdatedTS = now
}

// patch applies recognised field keys to the row with id, creating the row
// when it does not exist yet. An upsert keeps `sideline set x --status done`
// working on a task that only exists in the config file.
func (s *Store) patch(id string, fields map[string]string, now int64) error {
	if id == "" {
		return fmt.Errorf("patch needs a task id")
	}
	i := s.state.Find(id)
	if i < 0 {
		s.state.Tasks = append(s.state.Tasks, model.Task{
			ID:     id,
			Title:  id,
			Status: model.DefaultStatus,
		})
		i = len(s.state.Tasks) - 1
	}
	t := &s.state.Tasks[i]
	if v, ok := fields["title"]; ok {
		t.Title = v
	}
	if v, ok := fields["status"]; ok && v != t.Status {
		t.Status = v
		t.StatusTS = now
	}
	if v, ok := fields["group"]; ok {
		t.Group = v
	}
	if v, ok := fields["note"]; ok {
		t.Note = v
	}
	if v, ok := fields["bind_pane"]; ok {
		t.Bind = &model.Binding{Kind: model.BindPane, PaneID: v, Boot: fields["bind_boot"]}
	}
	if v, ok := fields["bind_window"]; ok {
		t.Bind = &model.Binding{Kind: model.BindWindow, Window: v}
	}
	if v, ok := fields["bind_external"]; ok {
		t.Bind = &model.Binding{Kind: model.BindExternal, Target: v}
	}
	t.ActivityTS = now
	s.state.UpdatedTS = now
	return nil
}
