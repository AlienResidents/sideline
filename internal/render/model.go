package render

import (
	"os"
	"time"
	"unicode/utf8"

	"github.com/AlienResidents/sideline/internal/config"
	"github.com/AlienResidents/sideline/internal/model"
	"github.com/AlienResidents/sideline/internal/store"
	"github.com/AlienResidents/sideline/internal/theme"
	"github.com/AlienResidents/sideline/internal/tmux"
	"github.com/charmbracelet/bubbletea"
)

// pruneBinds clears pane bindings that belong to a different tmux server
// lifetime. Pane ids are reused after a restart, so a stale id can otherwise
// send the cursor to an unrelated pane.
func (m *Model) pruneBinds(st *model.State) {
	if st == nil || m.boot == "" {
		return
	}
	for i := range st.Tasks {
		b := st.Tasks[i].Bind
		if b != nil && b.Kind == model.BindPane && b.Boot != "" && b.Boot != m.boot {
			st.Tasks[i].Bind = nil
		}
	}
}

// reloadConfig re-reads the config when the file changed. A broken edit keeps
// the previous config so a half-written file does not blank the pane.
func (m *Model) reloadConfig() {
	if m.cfgPath == "" {
		return
	}
	fi, err := os.Stat(m.cfgPath)
	if err != nil || fi.ModTime().Equal(m.cfgMod) {
		return
	}
	cfg, err := config.Load(m.cfgPath)
	if err != nil {
		return
	}
	m.cfg = cfg
	m.pal = palette(cfg)
	m.cfgMod = fi.ModTime()
}

// tickMsg drives the one-second refresh: elapsed stamps age, panes and state
// reload, and the frame is redrawn.
type tickMsg struct{}

// stateMsg carries one state snapshot from the file watch.
type stateMsg struct {
	state *model.State
}

// Model is the interactive sidebar. It is read-only over the state file: it
// watches and reloads, it never writes.
type Model struct {
	cfg    *config.Config
	pal    theme.Palette
	client tmux.Client

	state *model.State
	panes []tmux.Pane

	stateCh   <-chan *model.State
	stopWatch func()

	width, height int
	selected      string
	layout        string
	filter        string
	filtering     bool
	now           int64

	// cfgPath and cfgMod let a theme edit repaint without restarting the
	// pane: the config is re-read when the file's mtime moves.
	cfgPath string
	cfgMod  time.Time
	// boot is the tmux server start time this renderer is attached to. A
	// pane binding recorded against another server lifetime is dropped.
	boot string
}

// NewModel builds the interactive model and starts the state watch. It never
// fails: a missing or unreadable state file renders as an empty roster.
func NewModel(o Options) tea.Model {
	cfg := o.Config
	if cfg == nil {
		cfg = config.Default()
	}
	p, _ := config.Path()
	// The pane may belong to a tmux server other than the default one; the
	// daemon passes its socket through the pane environment.
	client := tmux.Client{Socket: os.Getenv("SIDELINE_TMUX_SOCKET")}
	m := &Model{
		cfg:     cfg,
		pal:     palette(cfg),
		client:  client,
		boot:    client.ServerStart(),
		width:   o.Width,
		height:  o.Height,
		layout:  o.layout(cfg),
		filter:  o.Filter,
		now:     time.Now().Unix(),
		cfgPath: p,
	}
	if fi, err := os.Stat(p); err == nil {
		m.cfgMod = fi.ModTime()
	}
	if st, err := store.Load(store.StateFile()); err == nil {
		m.pruneBinds(st)
		m.state = st
	}
	if ch, stop, err := store.Watch(store.StateFile()); err == nil {
		m.stateCh, m.stopWatch = ch, stop
	}
	return m
}

// Init starts the tick and the state wait.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.tick(), waitForState(m.stateCh))
}

func (m *Model) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// waitForState turns the next state watch value into a message. A closed
// channel yields a nil message: the model keeps the last state rather than
// exiting, so a stopped watch does not kill the sidebar.
func waitForState(ch <-chan *model.State) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		st, ok := <-ch
		if !ok {
			return nil
		}
		return stateMsg{st}
	}
}

// Update refreshes the frame on ticks, window sizes and state changes, and
// routes keys to the roster navigation and filter.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.now = time.Now().Unix()
		// A failed pane list keeps the last one: panes disappear transiently
		// while windows are closing.
		if panes, err := m.client.ListPanes(); err == nil {
			m.panes = panes
		}
		m.reloadConfig()
		// The state watch is re-armed from stateMsg, never from the tick:
		// arming again here would leave one blocked reader per second.
		return m, m.tick()
	case stateMsg:
		m.pruneBinds(msg.state)
		m.state = msg.state
		return m, waitForState(m.stateCh)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey applies one keypress. Keys that do not apply are ignored.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filter = ""
		m.filtering = false
	case "/":
		m.filtering = true
	case "enter":
		if m.filtering {
			m.filtering = false
		} else {
			m.jump()
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "g":
		m.moveToFirst()
	case "G":
		m.moveToLast()
	case "v":
		// Toggle between the two vertical layouts.
		if m.layout == "compact" {
			m.layout = "tiles"
		} else {
			m.layout = "compact"
		}
	case "backspace":
		if m.filtering && m.filter != "" {
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
		}
	default:
		if m.filtering && len(msg.Runes) > 0 {
			m.filter += string(msg.Runes)
		}
	}
	return m, nil
}

// rows is the filtered, sorted roster the navigation moves through.
func (m *Model) rows() []row {
	return roster(m.cfg, m.state, m.panes, m.filter)
}

// visibleIDs is the id sequence of the visible roster.
func (m *Model) visibleIDs() []string {
	rows := m.rows()
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].id
	}
	return ids
}

// move shifts the selection by delta rows, clamped at the ends.
func (m *Model) move(delta int) {
	ids := m.visibleIDs()
	if len(ids) == 0 {
		return
	}
	i := 0
	for j, id := range ids {
		if id == m.selected {
			i = j
			break
		}
	}
	i += delta
	if i < 0 {
		i = 0
	}
	if i >= len(ids) {
		i = len(ids) - 1
	}
	m.selected = ids[i]
}

func (m *Model) moveToFirst() {
	if ids := m.visibleIDs(); len(ids) > 0 {
		m.selected = ids[0]
	}
}

func (m *Model) moveToLast() {
	if ids := m.visibleIDs(); len(ids) > 0 {
		m.selected = ids[len(ids)-1]
	}
}

// jump focuses whatever the selected task is bound to. Errors are races with
// a closing pane or window, so they are ignored.
func (m *Model) jump() {
	for _, r := range m.rows() {
		if r.id != m.selected || r.bind == nil {
			continue
		}
		switch r.bind.Kind {
		case model.BindPane:
			_ = m.client.SelectPane(r.bind.PaneID)
		case model.BindWindow:
			_ = m.client.SelectWindow(r.bind.Window)
		}
		return
	}
}

// SelectedID is the task id under the cursor.
func (m *Model) SelectedID() string {
	return m.selected
}

// View renders the current frame.
func (m *Model) View() string {
	return Frame(Options{
		Config:   m.cfg,
		State:    m.state,
		Panes:    m.panes,
		Width:    m.width,
		Height:   m.height,
		Selected: m.selected,
		Filter:   m.filter,
		Layout:   m.layout,
	})
}
