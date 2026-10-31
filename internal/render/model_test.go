package render

import (
	"testing"

	"github.com/AlienResidents/sideline/internal/model"
)

func TestPruneBindsDropsOtherServerLifetime(t *testing.T) {
	m := &Model{boot: "100"}
	st := &model.State{Tasks: []model.Task{
		{ID: "same", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "100"}},
		{ID: "stale", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%2", Boot: "99"}},
		{ID: "unversioned", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%3"}},
		{ID: "window", Bind: &model.Binding{Kind: model.BindWindow, Window: "main:1"}},
	}}
	m.pruneBinds(st)

	if st.Tasks[0].Bind == nil {
		t.Fatal("binding from the live server was dropped")
	}
	if st.Tasks[1].Bind != nil {
		t.Fatal("binding from a dead server survived")
	}
	// A binding written before boot ids existed is kept: it may still be
	// valid, and dropping it would lose a working row on upgrade.
	if st.Tasks[2].Bind == nil {
		t.Fatal("unversioned pane binding was dropped")
	}
	if st.Tasks[3].Bind == nil {
		t.Fatal("window binding was dropped")
	}
}

func TestPruneBindsNoServerIsNoop(t *testing.T) {
	m := &Model{}
	st := &model.State{Tasks: []model.Task{
		{ID: "a", Bind: &model.Binding{Kind: model.BindPane, PaneID: "%1", Boot: "1"}},
	}}
	m.pruneBinds(st)
	if st.Tasks[0].Bind == nil {
		t.Fatal("pruned without a known server start")
	}
}
