package model

import "testing"

func TestFindReturnsTheRowIndex(t *testing.T) {
	s := &State{Tasks: []Task{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	if got := s.Find("b"); got != 1 {
		t.Fatalf("Find(b) = %d, want 1", got)
	}
	if got := s.Find("a"); got != 0 {
		t.Fatalf("Find(a) = %d, want 0", got)
	}
}

func TestFindMissingIsMinusOne(t *testing.T) {
	s := &State{Tasks: []Task{{ID: "a"}}}
	if got := s.Find("nope"); got != -1 {
		t.Fatalf("Find(nope) = %d, want -1", got)
	}
	if got := (&State{}).Find("a"); got != -1 {
		t.Fatalf("Find on an empty state = %d, want -1", got)
	}
}

func TestFindIsCaseSensitive(t *testing.T) {
	// Ids are the identity of a row, so a case variation is a different row
	// rather than the same one written differently.
	s := &State{Tasks: []Task{{ID: "Build"}}}
	if got := s.Find("build"); got != -1 {
		t.Fatalf("Find(build) = %d, want -1", got)
	}
}

func TestDeleteRemovesOnlyTheNamedRow(t *testing.T) {
	s := &State{Tasks: []Task{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}}
	i := s.Find("a")
	s.Tasks = append(s.Tasks[:i], s.Tasks[i+1:]...)
	if len(s.Tasks) != 1 || s.Tasks[0].ID != "b" {
		t.Fatalf("after delete: %+v", s.Tasks)
	}
}
