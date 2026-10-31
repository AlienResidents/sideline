package main

import (
	"strings"
	"testing"
)

func TestParseSetArgsFull(t *testing.T) {
	id, fields, err := parseSetArgs([]string{
		"t1", "--title", "Write daemon", "--status", "working",
		"--group", "platform", "--note", "see plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "t1" {
		t.Fatalf("id = %q", id)
	}
	want := map[string]string{
		"title": "Write daemon", "status": "working",
		"group": "platform", "note": "see plan",
	}
	for k, v := range want {
		if fields[k] != v {
			t.Errorf("fields[%q] = %q, want %q", k, fields[k], v)
		}
	}
	if len(fields) != len(want) {
		t.Fatalf("unexpected extra fields: %v", fields)
	}
}

func TestParseSetArgsBareID(t *testing.T) {
	// No flags means no field changes: the row is created or touched, and
	// its title must survive. This is the status-ping shape.
	id, fields, err := parseSetArgs([]string{"t1"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "t1" || len(fields) != 0 {
		t.Fatalf("id = %q fields = %v", id, fields)
	}
}

func TestParseSetArgsBindings(t *testing.T) {
	cases := map[string]string{
		"--bind-pane":     "bind_pane",
		"--bind-window":   "bind_window",
		"--bind-external": "bind_external",
	}
	for flag, key := range cases {
		_, fields, err := parseSetArgs([]string{"a1", flag, "v1"})
		if err != nil {
			t.Fatalf("%s: %v", flag, err)
		}
		if fields[key] != "v1" {
			t.Fatalf("%s -> %v", flag, fields)
		}
	}
}

func TestParseSetArgsRepeatKeepsLast(t *testing.T) {
	_, fields, err := parseSetArgs([]string{"a1", "--title", "first", "--title", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if fields["title"] != "second" {
		t.Fatalf("title = %q, want second", fields["title"])
	}
}

func TestParseSetArgsEmptyValueClears(t *testing.T) {
	// A flag with an empty value is present, so it clears the field rather
	// than being ignored.
	_, fields, err := parseSetArgs([]string{"a1", "--note", ""})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := fields["note"]
	if !ok || v != "" {
		t.Fatalf("note = %q present=%v", v, ok)
	}
}

func TestParseSetArgsErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"a1", "--title"},
		{"a1", "--bogus", "x"},
	}
	for _, args := range cases {
		if _, _, err := parseSetArgs(args); err == nil {
			t.Errorf("parseSetArgs(%q) must error", args)
		}
	}
}

func TestWantsHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"--state", "p", "--help"}} {
		if !wantsHelp(args) {
			t.Errorf("wantsHelp(%q) = false", args)
		}
	}
	if wantsHelp([]string{"--socket"}) {
		t.Error("wantsHelp(--socket) must be false")
	}
}

func TestEverySubcommandHasHelp(t *testing.T) {
	for cmd := range commandUsage {
		u, ok := commandUsage[cmd]
		if !ok || !strings.Contains(u, "Usage:") {
			t.Errorf("command %q has no usage text", cmd)
		}
	}
}

func TestPathsFlags(t *testing.T) {
	state, socket := paths([]string{"--state", "/tmp/s.json", "--socket", "/tmp/s.sock"})
	if state != "/tmp/s.json" || socket != "/tmp/s.sock" {
		t.Fatalf("state=%q socket=%q", state, socket)
	}
}
