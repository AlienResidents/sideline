# sideline

A task roster sidebar for tmux.

Most tmux sidebars show agents. sideline shows tasks. A task exists because a
person, a script, or an editor put it there. It may point at a tmux pane, at a
whole window, at a URL, or at nothing at all. That last case is the point: you
can track a job that has no process attached to it.

```
o Inbox
personal

* Deploy pipeline                    18s
ops
hold release
```

## How it works

One daemon owns the state file and keeps one sidebar pane in every tmux
window. Any number of writers send it a one-line JSON request over a unix
socket, so two processes never race on the same file. Each sidebar pane runs a
renderer that reads a snapshot and repaints when it changes.

```
omp extension ─┐
sideline set ──┼─> daemon ─> state.json ─> renderer panes
shell hook ────┘      │
                      └─> tmux (place and resize panes)
```

The daemon never writes pane titles. Titles belong to the program in the pane,
which is why a title does not get fought over every few seconds.

## Requirements

- macOS or Linux, amd64 or arm64.
- tmux 3.0 or newer. The daemon uses `split-window -e` and pane options
  (`set-option -p`, for the pane marker); both landed in the tmux 3.0 release
  block, per tmux's `CHANGES`. Check with `tmux -V`. Tested on tmux 3.8.
- Go 1.24 or newer, to build from source.

`~/.local/bin` is the recommended install location: no root, no package
manager, and the same path on macOS and Linux.

## Install

### macOS

```
brew install go tmux          # skip whichever you already have
git clone https://github.com/AlienResidents/sideline
cd sideline
make install                  # builds and copies to ~/.local/bin/sideline
```

If `~/.local/bin` is not on your `PATH`, add it to `~/.zshrc` (the default
shell on macOS) and open a new terminal:

```
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

Verify:

```
sideline --help
```

### Linux

```
# Go 1.24+ from your distribution, or from https://go.dev/dl/
go version

git clone https://github.com/AlienResidents/sideline
cd sideline
make install                  # builds and copies to ~/.local/bin/sideline
```

If `~/.local/bin` is not on your `PATH`, add it to `~/.profile` (login shells)
or `~/.bashrc`, then open a new terminal:

```
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.profile
```

Verify:

```
sideline --help
```

Distributions shipping tmux older than 3.0 need a newer tmux first; the
sidebar will not place panes without `split-window -e`.

### Without cloning

```
go install github.com/AlienResidents/sideline/cmd/sideline@latest
```

That writes to `$(go env GOPATH)/bin`, usually `~/go/bin`. Add that directory
to `PATH` if it is not there already.

### Prebuilt binaries

```
make build-all
```

`bin/` then holds `sideline-darwin-arm64`, `sideline-darwin-amd64`,
`sideline-linux-arm64`, and `sideline-linux-amd64`. Copy the one you need to
`~/.local/bin/sideline` and `chmod +x` it. This is the way to install on a
machine without a Go toolchain.

### System-wide install

```
sudo make install PREFIX=/usr/local/bin
```

### Upgrading

```
cd sideline && git pull && make install
sideline restart              # replace the daemon and rebuild the panes
```

The `off`/`on` pair matters: a sidebar pane keeps the renderer process it
started with, so existing panes would keep drawing with the old binary.

The daemon spawns the renderer by absolute path, so the tmux server does not
need your shell `PATH`.

## Quick start

```
sideline on                    # start the daemon and place the sidebar
sideline set inbox --title "Inbox" --group personal
sideline set inbox --status working
sideline list                  # JSON snapshot
sideline off                   # remove every sidebar pane
```

Start it automatically with your session:

```tmux
set-hook -g session-created 'run-shell "sideline on"'
```

Do not run two sidebar tools at once. If you are replacing another sidebar,
stop it with its own command and remove its `session-created` hook before
starting sideline, or both will fight for pane space.

### Upgrading the binary

A sidebar pane keeps the renderer process it started with. After replacing the
binary, run `sideline restart` to rebuild the panes; until then the old
renderer keeps drawing with the old code.

## Configuration

Unknown keys are an error, not a silent no-op: a misspelled `postion:` fails
to load with the line number. The shipped `config.example.yaml` is covered by
a test, so it always parses.

Config lives at `$SIDELINE_CONFIG`, else
`$XDG_CONFIG_HOME/sideline/config.yaml`, else `~/.config/sideline/config.yaml`.
A missing file is fine: the defaults are complete. See
[`config.example.yaml`](config.example.yaml).

### Placement

```yaml
sidebar:
  position: left   # left | right | top | bottom
  width: 43        # columns, used by left and right
  height: 8        # rows, used by top and bottom
```

`top` and `bottom` use the `horizontal` template set, because a three-line
tile does not fit in eight rows.

### Gutter

```yaml
sidebar:
  gutter: 2    # columns, 0 removes it
```

A two-column indicator column sits left of every tile: a bright bar for the
row of the window you are looking at, a dim one for the rest. The same live row
gets a subtle background band across its full width, and its title takes the
`current_worktree_fg` colour. That is the row-level "you are here" marker; the
cursor selection adds the same treatment to whichever row you move onto.

### Reloading

```yaml
sidebar:
  auto_reload: true    # default
```

With `auto_reload` on, a changed config file is parsed and applied within a
second or two. If the new file does not parse, the previous config stays in
use, so a half-written theme never blanks the sidebar.

The edit that sets `auto_reload: false` is itself applied immediately,
because the running config still has it on. Only edits made after that are
held until `sideline reload` or `sideline restart`.

| Command | Effect |
| --- | --- |
| `sideline reload` | re-read the config now, even with `auto_reload: false`; rebuilds panes if `position` changed |
| `sideline restart` | replace the daemon, remove the panes and rebuild them, which is how a new binary takes effect |

`reload` never restarts renderers; they re-read the config when told to.
`restart` does, because it replaces them.

Changing `position` at runtime tears the panes down and rebuilds them on the
new edge. Changing `width` or `height` resizes in place.

### Templates

Templates use the tmux format grammar, so a template written for another
sidebar tool usually works unchanged. Directives are stateful until
`#[default]`.

```yaml
sidebar:
  templates:
    compact: "{status_icon} {primary} {fill} {elapsed}"
    tiles:
      - "{status_icon} #[bold]{primary}#[default] {fill} {elapsed}"
      - "{secondary} {fill} {pane_title}"
      - "{note}"
    horizontal:
      - "{status_icon} {primary} {pane_suffix} {fill} {elapsed}"
      - "{secondary} {fill} {note}"
```

Placeholders: `{command_icon}` (glyph for the process in the bound pane),
`{status_icon}`, `{primary}` (title, highlighted when selected),
`{secondary}` (group), `{title}`, `{group}`, `{note}`, `{status}`, `{elapsed}`,
`{id}`, `{pane}`, `{pane_title}`, `{pane_suffix}`, `{command}`,
`{window_index}`, `{fill}`. `{agent_icon}` is an alias for `{command_icon}`,
and `{agent_label}` is the process name from the `icons:` map (`Oh My Pi`,
`Bash`). The names `{git}`, `{git_stats}`, and `{pr_checks}` are accepted and
empty, because sideline has no git or PR integration.

`{command_icon}` comes from the `icons:` map, keyed by the pane's process
name: `omp` is `⌥`, `pi` is `π`, a shell is a terminal glyph.

When a line is too long for the pane, the text before `{fill}` is shortened
and marked with `…`, so the trailing `{elapsed}` column is never cut off.

`{fill}` expands to the spaces that reach the pane width. Every line is padded
or truncated to exactly the pane width, so a repaint never leaves stale text.

### Status vocabulary

Statuses are free strings. The `status` map only decides the icon and colour:

```yaml
status:
  todo:    { icon: "", colour: "#7aa2f7" }
  working: { icon: "", colour: "#7aa2f7" }
  waiting: { icon: "", colour: "#e0af68" }
  done:    { icon: "", colour: "#9ece6a" }
  blocked: { icon: "", colour: "#f7768e" }
```

A status with no entry renders the default status style.

### Tasks declared in config

```yaml
tasks:
  - id: inbox
    title: Inbox
    group: personal
```

A declared task always has a row. `sideline set inbox --status working` fills
in its runtime state without changing the declaration. Declarations are
overridden field by field by state rows, so a `set --title` wins until the
state row is cleared.

## Commands

| Command | Purpose |
| --- | --- |
| `sideline on` | set the enabled flag, start the daemon, place panes |
| `sideline off` | clear the flag, remove every sidebar pane |
| `sideline toggle` | flip the flag |
| `sideline status` | version, daemon state, paths, counts |
| `sideline reload` | re-read the config now |
| `sideline restart` | replace the daemon and rebuild the panes |
| `sideline daemon` | run the daemon in the foreground |
| `sideline render` | run one renderer (the pane does this) |
| `sideline set <id> [flags]` | create or patch one task |
| `sideline clear <id>` | remove one task |
| `sideline list` | print the state as JSON |

`set` flags: `--title`, `--status`, `--group`, `--note`, `--bind-pane`,
`--bind-window`, `--bind-external`. Only the flags you pass are written, so a
status ping never erases a title.

### Status

```
$ sideline status
sideline 0.0.1
daemon     running, pid 4211, up 2h04m
config     ~/.config/sideline/config.yaml
state      ~/.local/state/sideline/state.json
tmux       default socket
enabled    yes
tasks      3
panes      2
```

`sideline status --json` prints the same as one object. `sideline --version`
prints just the version. Without a running daemon the block still shows the
paths, the enabled flag and the task count, and reports the daemon as stopped.

A daemon from an older build answers the socket but rejects `status`; the
block then says `running, older build` instead of claiming it is stopped.
`sideline restart` fixes that by replacing the process.

`set` and `clear` talk to the daemon when it is running and fall back to a
flock-guarded direct write when it is not.

## Keyboard

The sidebar never takes focus: panes are created detached, so switching to a
window lands in the pane you left. The sidebar only receives keys when you
move to it yourself (`ctrl+x` prefix then an arrow, or a click).

Inside the sidebar pane: `j`/`k` or arrows move, `g`/`G` jump to the ends,
`enter` focuses the bound pane or window, `v` toggles tiles and compact,
`/` filters, `esc` clears the filter, `ctrl+c` exits the renderer (the daemon
starts a new one on its next pass).

## Files

| Path | Content |
| --- | --- |
| `$XDG_STATE_HOME/sideline/state.json` | task state, owned by the daemon |
| `$XDG_STATE_HOME/sideline/sideline.sock` | client socket, mode 0600 |
| `$XDG_STATE_HOME/sideline/enabled` | the on/off flag |
| `$XDG_STATE_HOME/sideline/daemon.pid` | daemon pid, locked while it runs |

`SIDELINE_STATE_DIR` moves all three. The daemon passes the resolved paths to
each renderer pane, so a non-default directory works end to end.

## Editor and agent integration

[`examples/omp/sideline-status.ts`](examples/omp/sideline-status.ts) is an
oh-my-pi extension that reports the session status as a task. Copy it to
`~/.omp/agent/extensions/`.

The same pattern works for anything that can run a command:

```
sideline set build --status working
make all && sideline set build --status done
```

## Design notes

[`docs/decisions.md`](docs/decisions.md) records the settled choices and the
evidence behind them.

## Versioning

The major stays 0 while the config keys, the CLI, and the state file can still
change. `0.0.x` is pre-alpha and makes no compatibility promise. Bump the
release by editing `internal/version/VERSION`; the value is embedded, so every
build path reports it.

## Development

```
go test ./...
go vet ./...
gofmt -l .
```

The daemon accepts `--tmux-socket PATH`, which is how the smoke tests drive an
isolated tmux server without touching a live session.

## License

MIT.
