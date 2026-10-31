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
	"sync"
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
	// Version is reported by `sideline status`.
	Version string
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

// PIDPath returns the daemon pid file next to the socket, so a test server on
// another socket gets its own lock.
func PIDPath(socketPath string) string {
	return filepath.Join(filepath.Dir(socketPath), "daemon.pid")
}

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

	// shutdown is closed when a client asks the daemon to stop, which is how
	// `sideline restart` replaces the process without hunting its pid.
	shutdown chan struct{}
	stopOnce sync.Once

	started time.Time
}

// stop asks Run to shut down. It is idempotent: two clients asking at once
// must not close the channel twice.
func (d *Daemon) stop() {
	d.stopOnce.Do(func() { close(d.shutdown) })
}

// killPanes removes every sidebar pane this daemon placed. A restart uses it
// so the next daemon does not adopt renderers from the previous build.
func (d *Daemon) killPanes() {
	panes, err := d.client.ListPanes()
	if err != nil {
		return
	}
	for _, p := range panes {
		if p.Sidebar && p.PaneID != "" {
			_ = d.client.KillPane(p.PaneID)
		}
	}
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
		cfg:      cfg,
		st:       st,
		client:   tmux.Client{Socket: o.TmuxSocket},
		opts:     o,
		cfgPath:  p,
		dir:      dir,
		shutdown: make(chan struct{}),
		started:  time.Now(),
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
	if d.opts.TmuxSocket != "" {
		env = append(env, "SIDELINE_TMUX_SOCKET="+d.opts.TmuxSocket)
	}
	return env
}

// reload re-reads the config when the file changed since the last read. A
// broken edit keeps the previous config: a syntax error while editing a theme
// should not blank every sidebar. When force is false and
// sidebar.auto_reload is off, a file change is ignored until someone runs
// `sideline reload` or `sideline restart`.
func (d *Daemon) reload(force bool) {
	if d.cfgPath == "" {
		return
	}
	if !force && !d.cfg.AutoReloadOn() {
		return
	}
	fi, err := os.Stat(d.cfgPath)
	if err != nil {
		return
	}
	if !force && fi.ModTime().Equal(d.cfgMod) {
		return
	}
	cfg, err := config.Load(d.cfgPath)
	if err != nil {
		// Invalid config: keep drawing with the last good one.
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
	// The lock covers the window between the socket check and the listener,
	// which is where two `sideline on` calls can otherwise both bind.
	pid, err := lockPID(PIDPath(sock))
	if err != nil {
		return err
	}
	defer pid.Close()
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
				d.reload(true)
				continue
			}
			return nil
		case <-d.shutdown:
			// Remove the panes so the next daemon starts from a clean
			// window tree instead of adopting renderers from a dead build.
			d.killPanes()
			return nil
		case err := <-done:
			return err
		case <-ticker.C:
			d.reconcile()
		}
	}
}

// info reports the daemon's own state and its view of the world.
func (d *Daemon) info() *model.Info {
	enabled, _ := IsEnabled(d.opts.enabledPath())
	panes := 0
	if ps, err := d.client.ListPanes(); err == nil {
		for _, p := range ps {
			if p.Sidebar {
				panes++
			}
		}
	}
	return &model.Info{
		Running:    true,
		Version:    d.opts.Version,
		PID:        os.Getpid(),
		UptimeS:    int64(time.Since(d.started).Seconds()),
		ConfigPath: d.cfgPath,
		StatePath:  d.opts.statePath(),
		TmuxSocket: d.opts.TmuxSocket,
		Enabled:    enabled,
		Tasks:      len(d.st.State().Tasks),
		Panes:      panes,
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
		d.reload(true)
		// Renderers read the config themselves but only when auto_reload is
		// on. Bumping the sequence tells them to re-read either way.
		if err := d.st.BumpReload(); err != nil {
			return model.Response{OK: false, Error: err.Error()}
		}
		return model.Response{OK: true}
	case model.OpStatus:
		return model.Response{OK: true, Info: d.info()}
	case model.OpShutdown:
		// Reply first: Run closes the socket on its way out, and the client
		// must see OK before the connection drops.
		go d.stop()
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
	d.reload(false)
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
