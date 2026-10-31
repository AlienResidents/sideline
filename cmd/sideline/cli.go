package main

import (
	"fmt"
	"strings"
)

// parseSetArgs parses the arguments of `sideline set` after the subcommand
// name: the task id, then flags. It is pure so the flag grammar is testable
// without a store or a daemon.
//
//	sideline set <id> [--title T] [--status S] [--group G] [--note N]
//	              [--bind-pane %1] [--bind-window sess:1] [--bind-external URL]
//
// A missing id and a flag without a value are errors. Repeating a flag keeps
// the last value, which is how a human reads a repeated flag.
//
// Only the flags present are returned, so `sideline set x --status done`
// leaves the row's title and note alone. This is what makes status pings from
// an editor or an agent cheap and lossless.
func parseSetArgs(args []string) (string, map[string]string, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("set requires a task id")
	}
	id := args[0]
	fields := map[string]string{}

	for i := 1; i < len(args); i++ {
		flag := args[i]
		var key string
		switch flag {
		case "--title", "--status", "--group", "--note":
			key = strings.TrimPrefix(flag, "--")
		case "--bind-pane":
			key = "bind_pane"
		case "--bind-window":
			key = "bind_window"
		case "--bind-external":
			key = "bind_external"
		default:
			return "", nil, fmt.Errorf("unknown flag %q", flag)
		}
		if i+1 >= len(args) {
			return "", nil, fmt.Errorf("%s requires a value", flag)
		}
		i++
		fields[key] = args[i]
	}
	return id, fields, nil
}

// wantsHelp reports whether args ask for help.
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// Help texts, one per subcommand; every subcommand answers --help with exit 0.
const (
	usageDaemon = `sideline daemon - run the task-roster daemon in the foreground

The daemon owns the state file, serves the client socket, and keeps one
sidebar pane per tmux window. Only one daemon can run at a time.

Usage:
  sideline daemon [--state PATH] [--socket PATH] [--tmux-socket PATH]

--tmux-socket points the daemon at a non-default tmux server, which is how
the smoke tests drive an isolated server.
`
	usageRender = `sideline render - run the sidebar renderer

Runs the interactive roster renderer in a tmux pane. This is normally started
by the daemon inside the sidebar pane, not typed by hand.

Usage:
  sideline render
`
	usageOn = `sideline on - ensure the sidebar is running

Sets the enabled flag, starts the daemon if none is running, and lets the
daemon place a sidebar pane in every tmux window.

Usage:
  sideline on
`
	usageOff = `sideline off - stop the sidebar

Clears the enabled flag; the running daemon removes every sidebar pane on
its next reconcile pass.

Usage:
  sideline off
`
	usageToggle = `sideline toggle - flip the sidebar on or off

Usage:
  sideline toggle
`
	usageStatus = `sideline status - show the daemon, paths and counts

Prints the version, whether the daemon is running, the config and state
paths, the tmux socket, the enabled flag, and the task and pane counts.
The pane count needs a running daemon; without one the rest is still shown.

Usage:
  sideline status [--json]
  sideline --version
`
	usageReload = `sideline reload - re-read the config

Asks the running daemon to re-read its config file. Renderers watch that file
themselves, so templates and colours repaint on their own; a position change
makes the daemon rebuild the panes. Fails when no daemon is running.

Usage:
  sideline reload
`
	usageRestart = `sideline restart - replace the daemon and rebuild the panes

Stops the running daemon, removes every sidebar pane and starts a fresh
daemon. Use this after installing a new binary: a pane keeps the renderer
process it started with, so only a rebuild picks up new code.

Usage:
  sideline restart
`
	usageSet = `sideline set - write one task row

Creates or replaces the row with the given id. A set moves the row's activity
timestamp, so a fresh set sorts to the top under activity sorting.

Usage:
  sideline set <id> [--title T] [--status S] [--group G] [--note N]
                 [--bind-pane %1] [--bind-window sess:1] [--bind-external URL]

Statuses are free-form strings; the config file maps known ones to icons.
`
	usageClear = `sideline clear - remove one task row

Usage:
  sideline clear <id>
`
	usagePoke = `sideline poke - reconcile panes now

Asks the running daemon to reconcile tmux panes immediately instead of waiting
for its next tick. tmux hooks call this after a window or pane is created or
destroyed. With no daemon running it does nothing and exits 0, so a hook never
prints an error.

Usage:
  sideline poke
`
	usageList = `sideline list - print the current task state

Prints the task state as JSON. Talks to the daemon when one is running and
reads the state file directly when it is not.

Usage:
  sideline list [--state PATH] [--socket PATH]
`
)

// usage is the top-level help text.
const usage = `sideline - task roster sidebar for tmux

Usage:
  sideline <command> [flags]

Commands:
  daemon    run the daemon in the foreground
  render    run one sidebar renderer (used inside the pane)
  on        ensure the sidebar is running
  off       stop the sidebar
  toggle    flip the sidebar on or off
  status    show the daemon, paths and counts
  reload    re-read the config
  restart   replace the daemon and rebuild the panes
  set       write a task row
  clear     remove a task row
  list      print the current task state

Run "sideline <command> --help" for command details.
`

// commandUsage maps a subcommand name to its help text.
var commandUsage = map[string]string{
	"daemon":  usageDaemon,
	"render":  usageRender,
	"on":      usageOn,
	"off":     usageOff,
	"toggle":  usageToggle,
	"status":  usageStatus,
	"reload":  usageReload,
	"restart": usageRestart,
	"set":     usageSet,
	"clear":   usageClear,
	"list":    usageList,
	"poke":    usagePoke,
}
