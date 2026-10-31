// Command sideline is the single binary: daemon, renderer, and client.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/daemon"
	"github.com/AlienResidents/sideline/internal/ipc"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/render"
	"github.com/AlienResidents/sideline/internal/store"
	"github.com/AlienResidents/sideline/internal/tmux"
	"github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if code := run(os.Args[1], os.Args[2:]); code != 0 {
		os.Exit(code)
	}
}

// run dispatches one subcommand and returns its exit code. Help comes first
// so every subcommand answers --help without touching tmux or the state
// file.
func run(cmd string, args []string) int {
	u, known := commandUsage[cmd]
	if !known {
		switch cmd {
		case "help", "--help", "-h":
			fmt.Print(usage)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "sideline: %q is not a sideline command\n\n%s", cmd, usage)
			return 2
		}
	}
	if wantsHelp(args) {
		fmt.Print(u)
		return 0
	}

	var err error
	switch cmd {
	case "daemon":
		err = cmdDaemon(args)
	case "render":
		err = cmdRender()
	case "on":
		err = cmdOn()
	case "off":
		err = cmdOff()
	case "toggle":
		err = cmdToggle()
	case "set":
		err = cmdSet(args)
	case "clear":
		err = cmdClear(args)
	case "list":
		err = cmdList(args)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sideline %s: %v\n", cmd, err)
		return 1
	}
	return 0
}

// cmdRender runs the sidebar renderer inside a tmux pane. The daemon starts
// this subcommand into the pane; a human typing it gets the same program.
func cmdRender() error {
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}
	m := render.NewModel(render.Options{Config: cfg})
	// WindowSizeMsg sets the pane size, so no initial guess is needed.
	if _, err := tea.NewProgram(m).Run(); err != nil {
		return fmt.Errorf("render: %w", err)
	}
	return nil
}

// paths resolves the state file and daemon socket paths for a subcommand,
// honouring --state/--socket over the environment defaults.
func paths(args []string) (statePath, socketPath string) {
	statePath, socketPath = store.StateFile(), store.SocketPath()
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state", "--socket":
			if i+1 >= len(args) {
				break
			}
			i++
			if args[i-1] == "--state" {
				statePath = args[i]
			} else {
				socketPath = args[i]
			}
		}
	}
	return statePath, socketPath
}

// flagValue returns the value of --name in args, or "" when absent. It is
// used for the flags that exist only on the daemon subcommand.
func flagValue(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

// cmdDaemon runs the daemon in the foreground until interrupted.
func cmdDaemon(args []string) error {
	statePath, socketPath := paths(args)
	tmuxSocket := flagValue(args, "--tmux-socket")
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}
	render, err := renderCmd()
	if err != nil {
		return err
	}
	d, err := daemon.New(daemon.Options{
		Config:     cfg,
		StatePath:  statePath,
		SocketPath: socketPath,
		TmuxSocket: tmuxSocket,
		RenderCmd:  render,
	})
	if err != nil {
		return err
	}
	return d.Run()
}

// cmdOn sets the enabled flag and starts the daemon if none answers.
func cmdOn() error {
	if err := daemon.SetEnabled(enabledPath(), true); err != nil {
		return err
	}
	if daemon.Running(store.SocketPath()) {
		return nil
	}
	return startDaemon()
}

// cmdOff clears the enabled flag. A running daemon tears the sidebar panes
// down on its next reconcile pass, so off never races tmux itself.
func cmdOff() error {
	return daemon.SetEnabled(enabledPath(), false)
}

// cmdToggle flips the flag.
func cmdToggle() error {
	on, err := daemon.IsEnabled(enabledPath())
	if err != nil {
		return err
	}
	if on {
		return cmdOff()
	}
	return cmdOn()
}

// cmdSet writes one task row. The daemon is preferred so the state file has
// one writer; the flock-backed store is the fallback on a cold machine.
func cmdSet(args []string) error {
	id, fields, err := parseSetArgs(args)
	if err != nil {
		return err
	}
	// A pane id only means something inside one tmux server lifetime, so a
	// pane binding carries the server's start time. Without it a row can
	// point at whatever pane inherits the id after a restart.
	if fields["bind_pane"] != "" {
		fields["bind_boot"] = (tmux.Client{}).ServerStart()
	}
	req := model.Request{Op: model.OpSet, ID: id, Fields: fields}
	if apply(req) {
		return nil
	}
	return applyDirect(req)
}

// cmdClear removes one task row, same preference order as cmdSet.
func cmdClear(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("clear requires exactly one task id")
	}
	req := model.Request{Op: model.OpClear, ID: args[0]}
	if apply(req) {
		return nil
	}
	return applyDirect(req)
}

// apply sends req to the daemon and reports whether it was applied there.
func apply(req model.Request) bool {
	_, socketPath := paths(nil)
	resp, err := ipc.Send(socketPath, req)
	return err == nil && resp.OK
}

// applyDirect mutates the state file without a daemon, under the store's
// flock, so a daemonless writer still cannot interleave with another one.
func applyDirect(req model.Request) error {
	if err := ensureStateDir(); err != nil {
		return err
	}
	st, err := store.New(store.StateFile())
	if err != nil {
		return err
	}
	_, err = st.Apply(req)
	return err
}

// cmdList prints the task state as JSON: through the daemon when one
// answers, straight from the file otherwise, so a cold machine still lists.
func cmdList(args []string) error {
	statePath, socketPath := paths(args)
	if resp, err := ipc.Send(socketPath, model.Request{Op: model.OpList}); err == nil && resp.OK && resp.State != nil {
		return printState(resp.State)
	}
	st, err := store.New(statePath)
	if err != nil {
		return err
	}
	return printState(st.State())
}

// ensureStateDir creates the state dir before the store opens its lock file.
// A first run on a fresh machine has no directory yet.
func ensureStateDir() error {
	if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	return nil
}

func printState(s *model.State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func enabledPath() string { return daemon.EnabledPath(store.Dir()) }

// startDaemon starts a detached daemon process running this same binary. A
// fresh process group keeps it alive when the calling shell exits.
func startDaemon() error {
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate sideline binary: %w", err)
	}
	c := exec.Command(bin, "daemon")
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// nil files give the child /dev/null: the daemon must not hold the
	// caller's tty open, which would keep a terminal alive after `on`.
	if err := c.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	// Reap in the background so the child is never a zombie in a long
	// running caller; the child is Setsid so it survives regardless.
	go c.Wait()
	// Return only once the socket answers, so `sideline on` means the
	// sidebar is coming up rather than merely that a fork happened.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if daemon.Running(store.SocketPath()) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not answer within 2s")
}

// renderCmd is the shell command a sidebar pane runs. The absolute path is
// used because a pane spawned by the tmux server does not necessarily share
// the caller's PATH.
func renderCmd() (string, error) {
	bin, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate sideline binary: %w", err)
	}
	return shellQuote(bin) + " render", nil
}

// shellQuote wraps s in single quotes for tmux's sh -c.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
