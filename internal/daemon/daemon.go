// Package daemon owns the single sideline daemon process: the unix socket
// that applies every state mutation, the tmux reconcile loop that keeps one
// sidebar pane per window, and the enabled flag that on/off/toggle write.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/ipc"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/store"
	"github.com/AlienResidents/sideline/internal/tmux"
)

// tickPeriod is how often the daemon reconciles tmux panes. One second would
// be chatty; tmux windows change on human timescales.
const tickPeriod = 5 * time.Second

// Options is everything the daemon needs to run. Zero fields fall back to
// the defaults from store and config, so a plain New(Options{}) runs.
type Options struct {
	// Config is the loaded config. Nil means config.Default().
	Config *config.Config
	// StatePath is the state file the daemon owns.
	StatePath string
	// SocketPath is the daemon socket.
	SocketPath string
	// EnabledPath is the enabled flag file.
	EnabledPath string
	// TmuxSocket is the tmux socket to talk to ("" for default).
	TmuxSocket string
	// RenderCmd is the command the sidebar pane runs.
	RenderCmd string
}

func (o Options) statePath() string {
	if o.StatePath != "" {
		return o.StatePath
	}
	return store.StateFile()
}

func (o Options) socketPath() string {
	if o.SocketPath != "" {
		return o.SocketPath
	}
	return store.SocketPath()
}

func (o Options) enabledPath() string {
	if o.EnabledPath != "" {
		return o.EnabledPath
	}
	return EnabledPath(store.Dir())
}

func (o Options) renderCmd() string {
	if o.RenderCmd != "" {
		return o.RenderCmd
	}
	// The sidebar pane runs this same binary in render mode, resolved
	// through PATH so a renamed or relocated install keeps working.
	return "sideline render"
}

// ErrRunning means a daemon is already answering on the socket.
var ErrRunning = errors.New("sideline daemon already running")

// Daemon is one running daemon: the store, the socket server and the tmux
// reconcile loop. Run blocks until a shutdown signal arrives.
type Daemon struct {
	cfg    *config.Config
	st     *store.Store
	client tmux.Client
	opts   Options
	srv    *ipc.Server

	// cfgPath and cfgMod drive hot reload: the config is re-read when the
	// file's modification time moves, so editing a theme does not need a
	// daemon restart.
	cfgPath string
	cfgMod  time.Time
	// dir is the placement the live panes were created with. A position
	// change cannot be applied by resizing, so the panes are torn down once
	// and rebuilt on the following tick.
	dir        tmux.Direction
	reposition bool
}

// New opens the state file and returns a daemon ready to Run.
func New(o Options) (*Daemon, error) {
	cfg := o.Config
	if cfg == nil {
		c, err := config.LoadDefault()
		if err != nil {
			return nil, err
		}
		cfg = c
	}
	// The store flocks a lock file next to the state file, so the directory
	// must exist before it opens; a first run on a fresh machine has none.
	if err := os.MkdirAll(filepath.Dir(o.statePath()), 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	st, err := store.New(o.statePath())
	if err != nil {
		return nil, err
	}
	p, _ := config.Path()
	dir, err := tmux.ParseDirection(cfg.Sidebar.Position)
	if err != nil {
		dir = tmux.Left
	}
	d := &Daemon{
		cfg:     cfg,
		st:      st,
		client:  tmux.Client{Socket: o.TmuxSocket},
		opts:    o,
		cfgPath: p,
		dir:     dir,
	}
	if fi, err := os.Stat(p); err == nil {
		d.cfgMod = fi.ModTime()
	}
	return d, nil
}

// renderEnv is the environment the renderer pane needs so it reads the same
// config and state the daemon does, even when either path is non-default.
func (d *Daemon) renderEnv() []string {
	env := []string{"SIDELINE_STATE_DIR=" + filepath.Dir(d.opts.statePath())}
	if d.cfgPath != "" {
		env = append(env, "SIDELINE_CONFIG="+d.cfgPath)
	}
	return env
}

// reload re-reads the config when the file changed since the last read. A
// broken edit keeps the previous config: a syntax error while editing a theme
// should not blank every sidebar.
func (d *Daemon) reload() {
	if d.cfgPath == "" {
		return
	}
	fi, err := os.Stat(d.cfgPath)
	if err != nil {
		return
	}
	if fi.ModTime().Equal(d.cfgMod) {
		return
	}
	cfg, err := config.Load(d.cfgPath)
	if err != nil {
		return
	}
	d.cfg = cfg
	d.cfgMod = fi.ModTime()
	if dir, err := tmux.ParseDirection(cfg.Sidebar.Position); err == nil && dir != d.dir {
		d.dir = dir
		d.reposition = true
	}
}

// State returns the current state, for the list fallback path.
func (d *Daemon) State() *model.State { return d.st.State() }

// Store returns the daemon's store.
func (d *Daemon) Store() *store.Store { return d.st }

// Running reports whether a daemon answers on the socket path.
func Running(socketPath string) bool {
	if socketPath == "" {
		socketPath = store.SocketPath()
	}
	if _, err := os.Stat(socketPath); err != nil {
		return false
	}
	_, err := ipc.Send(socketPath, model.Request{Op: model.OpPing})
	return err == nil
}

// Run serves the socket and reconciles tmux until interrupted. The socket
// listen also acts as the single-instance guard: a second daemon exits with
// ErrRunning rather than stealing the socket out from under the first.
func (d *Daemon) Run() error {
	sock := d.opts.socketPath()
	if Running(sock) {
		return ErrRunning
	}
	srv, err := ipc.Listen(sock, d.handle)
	if err != nil {
		return err
	}
	d.srv = srv
	defer srv.Close()

	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	// One reconcile before the loop so enabling the daemon draws sidebars
	// immediately instead of after the first tick.
	d.reconcile()

	ticker := time.NewTicker(tickPeriod)
	defer ticker.Stop()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()

	for {
		select {
		case sig := <-sigs:
			// SIGHUP is a reload request from a human, not a shutdown.
			if sig == syscall.SIGHUP {
				d.reload()
				continue
			}
			return nil
		case err := <-done:
			return err
		case <-ticker.C:
			d.reconcile()
		}
	}
}

// handle answers one client request through the store.
func (d *Daemon) handle(req model.Request) model.Response {
	switch req.Op {
	case model.OpPing:
		return model.Response{OK: true}
	case model.OpList:
		return model.Response{OK: true, State: d.st.State()}
	case model.OpReload:
		d.reload()
		return model.Response{OK: true}
	}
	resp, err := d.st.Apply(req)
	if err != nil {
		return resp
	}
	return resp
}

// reconcile reads the enabled flag, lists panes and applies the plan.
// Errors are logged-shaped and dropped: a closing tmux server or a half
// dead pane must never take the daemon down.
func (d *Daemon) reconcile() {
	d.reload()
	enabled, err := IsEnabled(d.opts.enabledPath())
	if err != nil {
		return
	}
	panes, err := d.client.ListPanes()
	if err != nil {
		// No tmux server: nothing to place, nothing to remove.
		return
	}
	if d.reposition {
		// A placement change is not a resize: drop the panes and let the
		// next tick build them on the new edge.
		for _, p := range panes {
			if p.Sidebar && p.PaneID != "" {
				_ = d.client.KillPane(p.PaneID)
			}
		}
		d.reposition = false
		return
	}
	for _, a := range plan(d.cfg, panes, enabled) {
		d.apply(a)
	}
}

// apply performs one reconciliation action. Every error is dropped: these
// are races with a closing pane or server, and the next tick retries.
func (d *Daemon) apply(a Action) {
	dir, err := tmux.ParseDirection(d.cfg.Sidebar.Position)
	if err != nil {
		dir = tmux.Left
	}
	switch a.Kind {
	case ActionAdd:
		id, err := d.client.SplitSidebar(a.Target, dir, a.Size, d.opts.renderCmd(), d.renderEnv())
		if err != nil {
			return
		}
		// Assert the configured size: split-window -l sizes against the
		// split edge, not the final sidebar pane.
		_ = d.client.ResizePane(id, axisFor(d.cfg), a.Size)
	case ActionResize:
		_ = d.client.ResizePane(a.PaneID, axisFor(d.cfg), a.Size)
	case ActionRemove:
		_ = d.client.KillPane(a.PaneID)
	}
}
