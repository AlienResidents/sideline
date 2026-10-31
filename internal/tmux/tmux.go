// Package tmux is a thin wrapper over the tmux CLI. It only runs commands that
// answer questions or place the sidebar pane; it never mutates task state.
package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNoServer means no tmux server is running for this socket.
var ErrNoServer = errors.New("tmux: no server running")

// Client runs tmux commands against one socket. An empty Socket uses tmux's
// default resolution ($TMUX, then the default socket path).
type Client struct {
	Socket string
}

// Direction is where the sidebar pane sits relative to the content pane.
type Direction int

// Placement directions. The names match config's sidebar.position values.
const (
	Left Direction = iota
	Right
	Top
	Bottom
)

// ParseDirection maps a config position string onto a Direction.
func ParseDirection(s string) (Direction, error) {
	switch s {
	case "left", "":
		return Left, nil
	case "right":
		return Right, nil
	case "top":
		return Top, nil
	case "bottom":
		return Bottom, nil
	}
	return Left, fmt.Errorf("tmux: unknown position %q", s)
}

// IsVertical reports whether the direction splits along the horizontal axis,
// which is to say the sidebar is a full-height column.
func (d Direction) IsVertical() bool { return d == Left || d == Right }

// Axis is the resize axis for a pane.
type Axis int

// Resize axes.
const (
	AxisWidth Axis = iota
	AxisHeight
)

// roleOption marks the panes this tool created. It is a pane option so the
// marker survives a shell restart inside the pane and cannot collide with a
// user binary that happens to be named sideline.
const roleOption = "@sideline_role"

// roleValue is the marker value for a renderer pane.
const roleValue = "renderer"

// Pane is one tmux pane as reported by list-panes.
type Pane struct {
	Session      string
	WindowIndex  int
	WindowName   string
	PaneIndex    int
	PaneID       string
	Command      string
	StartCommand string
	Title        string
	Path         string
	Width        int
	Height       int
	Active       bool
	WindowActive bool
	// Sidebar is true when the pane carries this tool's renderer marker.
	Sidebar bool
	// Boot is the tmux server start time. Pane ids are reused when a server
	// restarts, so a binding is only valid while this value matches.
	Boot string
}

// listFormat must stay in sync with parsePane.
const listFormat = "#{session_name}\x1f#{window_index}\x1f#{window_name}\x1f#{pane_index}\x1f#{pane_id}\x1f#{pane_current_command}\x1f#{pane_start_command}\x1f#{pane_title}\x1f#{pane_current_path}\x1f#{pane_width}\x1f#{pane_height}\x1f#{pane_active}\x1f#{window_active}\x1f#{" + roleOption + "}\x1f#{start_time}"

// run executes tmux with args and returns stdout.
func (c Client) run(args ...string) (string, error) {
	full := args
	if c.Socket != "" {
		full = append([]string{"-S", c.Socket}, args...)
	}
	cmd := exec.Command("tmux", full...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := errb.String()
		if strings.Contains(msg, "no server running") || strings.Contains(msg, "error connecting to") {
			return "", ErrNoServer
		}
		return "", fmt.Errorf("tmux %s: %w: %s", args[0], err, strings.TrimSpace(msg))
	}
	return out.String(), nil
}

// Out runs a read-only tmux command and returns its trimmed stdout.
func (c Client) Out(args ...string) (string, error) {
	out, err := c.run(args...)
	return strings.TrimSpace(out), err
}

// ListPanes returns every pane in every session, in tmux order.
func (c Client) ListPanes() ([]Pane, error) {
	out, err := c.run("list-panes", "-a", "-F", listFormat)
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		p, err := parsePane(line)
		if err != nil {
			return nil, err
		}
		panes = append(panes, p)
	}
	return panes, nil
}

func parsePane(line string) (Pane, error) {
	f := strings.Split(line, "\x1f")
	if len(f) != 15 {
		return Pane{}, fmt.Errorf("tmux: unexpected pane field count %d", len(f))
	}
	wi, _ := strconv.Atoi(f[1])
	pi, _ := strconv.Atoi(f[3])
	w, _ := strconv.Atoi(f[9])
	h, _ := strconv.Atoi(f[10])
	return Pane{
		Session:      f[0],
		WindowIndex:  wi,
		WindowName:   f[2],
		PaneIndex:    pi,
		PaneID:       f[4],
		Command:      f[5],
		StartCommand: f[6],
		Title:        f[7],
		Path:         f[8],
		Width:        w,
		Height:       h,
		Active:       f[11] == "1",
		WindowActive: f[12] == "1",
		Sidebar:      f[13] == roleValue,
		Boot:         f[14],
	}, nil
}

// Target returns the "session:window_index" form for this pane's window.
func (p Pane) Target() string {
	return p.Session + ":" + strconv.Itoa(p.WindowIndex)
}

// ServerStart returns the tmux server's start time as a string. Pane ids are
// only unique within one server lifetime, so a binding records this value and
// is dropped when the live server reports a different one.
func (c Client) ServerStart() string {
	out, err := c.run("display-message", "-p", "#{start_time}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// CurrentSession returns the session of the calling client.
func (c Client) CurrentSession() (string, error) {
	out, err := c.run("display-message", "-p", "#{session_name}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// SplitSidebar creates a sidebar pane next to the active pane of target and
// returns the new pane id. size is columns for Left/Right and rows for
// Top/Bottom. cmd is the process the pane runs. env entries are "KEY=VALUE"
// and are set on the new pane only, so a non-default state dir or config path
// reaches the renderer without touching the tmux server environment.
func (c Client) SplitSidebar(target string, dir Direction, size int, cmd string, env []string) (string, error) {
	out, err := c.run(splitArgs(target, dir, size, cmd, env)...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if err := c.SetPaneOption(id, roleOption, roleValue); err != nil {
		return id, err
	}
	return id, nil
}

// splitArgs builds the tmux command that places a sidebar pane. It is pure so
// the flags are testable.
//
// -d is load-bearing: without it tmux makes the new pane active, so creating
// a sidebar would move the cursor out of the user's shell, and switching to
// that window would land in the sidebar.
func splitArgs(target string, dir Direction, size int, cmd string, env []string) []string {
	flag := "-hb"
	switch dir {
	case Right:
		flag = "-h"
	case Top:
		flag = "-vb"
	case Bottom:
		flag = "-v"
	}
	args := []string{"split-window", flag, "-d", "-l", strconv.Itoa(size), "-t", target}
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	return append(args, "-P", "-F", "#{pane_id}", cmd)
}

// SetPaneOption writes a pane-scoped tmux option.
func (c Client) SetPaneOption(paneID, name, value string) error {
	_, err := c.run("set-option", "-p", "-t", paneID, name, value)
	return err
}

// ResizePane sets the width or height of a pane.
func (c Client) ResizePane(paneID string, axis Axis, size int) error {
	flag := "-x"
	if axis == AxisHeight {
		flag = "-y"
	}
	_, err := c.run("resize-pane", "-t", paneID, flag, strconv.Itoa(size))
	return err
}

// SelectPane focuses a pane.
func (c Client) SelectPane(paneID string) error {
	_, err := c.run("select-pane", "-t", paneID)
	return err
}

// SelectWindow focuses "session:index".
func (c Client) SelectWindow(target string) error {
	_, err := c.run("select-window", "-t", target)
	return err
}

// HasSession reports whether session exists.
func (c Client) HasSession(session string) bool {
	_, err := c.run("has-session", "-t", session)
	return err == nil
}

// KillPane closes a pane. Used when turning the sidebar off.
func (c Client) KillPane(paneID string) error {
	_, err := c.run("kill-pane", "-t", paneID)
	return err
}
