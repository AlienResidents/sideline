package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AlienResidents/sideline/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestApplySetClearPatch(t *testing.T) {
	// Pin the clock so a status_ts move is observable within one run:
	// real seconds tick too slowly to separate consecutive applies.
	sec := int64(1_000)
	old := nowSec
	nowSec = func() int64 { sec++; return sec }
	t.Cleanup(func() { nowSec = old })
	s := newTestStore(t)

	resp, err := s.Apply(model.Request{Op: model.OpSet, Task: &model.Task{ID: "webapp", Title: "web app", Status: "todo"}})
	if err != nil || !resp.OK {
		t.Fatalf("set: err=%v resp=%+v", err, resp)
	}
	i := resp.State.Find("webapp")
	if i < 0 {
		t.Fatalf("row missing after set: %+v", resp.State)
	}
	got := resp.State.Tasks[i]
	if got.Status != "todo" || got.StatusTS == 0 || got.ActivityTS == 0 {
		t.Fatalf("fresh row timestamps: %+v", got)
	}
	statusTS, activityTS := got.StatusTS, got.ActivityTS

	// A patch to a non-status field keeps status_ts but moves activity_ts.
	if _, err := s.Apply(model.Request{Op: model.OpSet, ID: "webapp", Fields: map[string]string{"title": "web app 2"}}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	st := s.State()
	row := st.Tasks[st.Find("webapp")]
	if row.Title != "web app 2" {
		t.Fatalf("title not patched: %+v", row)
	}
	if row.StatusTS != statusTS {
		t.Fatalf("status_ts moved on title patch: %d -> %d", statusTS, row.StatusTS)
	}
	if row.ActivityTS < activityTS {
		t.Fatalf("activity_ts stale: %d < %d", row.ActivityTS, activityTS)
	}
	activityTS = row.ActivityTS

	// A status change moves status_ts; the same status again does not.
	if _, err := s.Apply(model.Request{Op: model.OpSet, ID: "webapp", Fields: map[string]string{"status": "working"}}); err != nil {
		t.Fatalf("patch status: %v", err)
	}
	st = s.State()
	row = st.Tasks[st.Find("webapp")]
	if row.Status != "working" || row.StatusTS <= statusTS {
		t.Fatalf("status change: %+v", row)
	}
	statusTS = row.StatusTS
	if _, err := s.Apply(model.Request{Op: model.OpSet, ID: "webapp", Fields: map[string]string{"status": "working"}}); err != nil {
		t.Fatalf("patch status again: %v", err)
	}
	if row2 := s.State().Tasks[st.Find("webapp")]; row2.StatusTS != statusTS {
		t.Fatalf("status_ts moved without status change: %d -> %d", statusTS, row2.StatusTS)
	}

	// Bind fields patch the binding.
	if _, err := s.Apply(model.Request{Op: model.OpSet, ID: "webapp", Fields: map[string]string{"bind_pane": "%12"}}); err != nil {
		t.Fatalf("patch bind: %v", err)
	}
	row = s.State().Tasks[s.State().Find("webapp")]
	if row.Bind == nil || row.Bind.Kind != model.BindPane || row.Bind.PaneID != "%12" {
		t.Fatalf("bind not patched: %+v", row.Bind)
	}

	// A set of a full row replaces it, moving status_ts only on change.
	if _, err := s.Apply(model.Request{Op: model.OpSet, Task: &model.Task{ID: "webapp", Title: "replaced", Status: "working"}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	row = s.State().Tasks[s.State().Find("webapp")]
	if row.Title != "replaced" || row.StatusTS != statusTS || row.Bind != nil {
		t.Fatalf("replace semantics: %+v", row)
	}

	// Clear removes the row; clearing a missing id is a no-op success.
	if _, err := s.Apply(model.Request{Op: model.OpClear, ID: "webapp"}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if s.State().Find("webapp") >= 0 {
		t.Fatalf("row survived clear")
	}
	if resp, err := s.Apply(model.Request{Op: model.OpClear, ID: "nope"}); err != nil || !resp.OK {
		t.Fatalf("clear missing id: err=%v resp=%+v", err, resp)
	}

	// A patch to a missing id creates the row, so a status ping works on a
	// task that only exists in the config file.
	resp, err = s.Apply(model.Request{Op: model.OpSet, ID: "nope", Fields: map[string]string{"status": "working"}})
	if err != nil || !resp.OK {
		t.Fatalf("patch missing id should upsert: err=%v resp=%+v", err, resp)
	}
	i = s.State().Find("nope")
	if i < 0 {
		t.Fatal("upserted row missing")
	}
	up := s.State().Tasks[i]
	if up.Title != "nope" || up.Status != "working" {
		t.Fatalf("upserted row: %+v", up)
	}

	// A status-only patch on an existing row must not touch its title: this
	// is the lossless-ping property the agent extension relies on.
	if _, err := s.Apply(model.Request{Op: model.OpSet, Task: &model.Task{ID: "docs", Title: "Docs", Status: "todo"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := s.Apply(model.Request{Op: model.OpSet, ID: "docs", Fields: map[string]string{"status": "done"}}); err != nil {
		t.Fatalf("ping: %v", err)
	}
	docs := s.State().Tasks[s.State().Find("docs")]
	if docs.Title != "Docs" || docs.Status != "done" {
		t.Fatalf("status ping damaged the row: %+v", docs)
	}
}

func TestApplyUnknownOpAndMissingStatusDefault(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Apply(model.Request{Op: "explode"}); err == nil {
		t.Fatalf("unknown op accepted")
	}
	if _, err := s.Apply(model.Request{Op: model.OpSet, Task: &model.Task{ID: "docs"}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	row := s.State().Tasks[s.State().Find("docs")]
	if row.Status != model.DefaultStatus {
		t.Fatalf("empty status not defaulted: %+v", row)
	}
}

func TestSaveFailureLeavesFileIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Apply(model.Request{Op: model.OpSet, Task: &model.Task{ID: "inbox", Title: "before"}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// A read-only directory makes the temp-file write fail, which must
	// leave the previous file untouched.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(dir, 0o700)
	if _, err := s.Apply(model.Request{Op: model.OpSet, Task: &model.Task{ID: "should-not-exist"}}); err == nil {
		t.Fatalf("save in read-only dir succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("state file changed on failed save")
	}
	var st model.State
	if err := json.Unmarshal(after, &st); err != nil {
		t.Fatalf("file no longer valid JSON: %v", err)
	}
	if st.Find("should-not-exist") >= 0 {
		t.Fatalf("failed write leaked into state")
	}
}

func TestConcurrentApplyDistinctIDs(t *testing.T) {
	s := newTestStore(t)
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := s.Apply(model.Request{Op: model.OpSet, Task: &model.Task{
				ID:     fmt.Sprintf("task-%02d", i),
				Title:  fmt.Sprintf("row %d", i),
				Status: "working",
			}})
			if err != nil || !resp.OK {
				t.Errorf("apply %d: err=%v resp=%+v", i, err, resp)
			}
		}(i)
	}
	wg.Wait()

	st, err := Load(s.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(st.Tasks) != n {
		t.Fatalf("want %d rows, got %d: %+v", n, len(st.Tasks), st.Tasks)
	}
	seen := map[string]bool{}
	for _, row := range st.Tasks {
		if seen[row.ID] {
			t.Fatalf("duplicate id %q", row.ID)
		}
		seen[row.ID] = true
	}
}

func TestLoadMissingAndEmpty(t *testing.T) {
	dir := t.TempDir()
	st, err := Load(filepath.Join(dir, "absent.json"))
	if err != nil || st.Version != model.StateVersion || len(st.Tasks) != 0 {
		t.Fatalf("missing file: st=%+v err=%v", st, err)
	}
	if _, err := Load(filepath.Join(dir, "bad.json")); err != nil {
		t.Fatalf("empty dir load: %v", err)
	}
}
