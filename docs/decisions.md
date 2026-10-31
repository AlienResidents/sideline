# Decisions

Settled choices, with the evidence behind them. Proposals do not belong here.

## D1: Go, not Rust

Evidence: `go version go1.27.2 darwin/arm64` is installed. `cargo` and `rustc` are not. A Go build with `CGO_ENABLED=0` produces darwin/arm64, linux/amd64, and linux/arm64 binaries from one machine with no extra toolchain. The daemon does one tmux poll loop and one socket read loop, so the memory and latency arguments for Rust do not apply.

## D2: One daemon owns the state file

Producers (CLI, editor hooks, agent extensions) send a JSON line over a unix socket. The daemon applies the change, writes the file atomically, and notifies renderers. Two writers never race on one file, and the pane-title fight a sidebar daemon can start (daemon rewrites, extension re-asserts every 5 s) cannot happen because only the extension owns pane titles.

## D3: Tasks, not agents

The roster is a list of tasks. A task may bind to a tmux pane, a window, or an external target, but binding is optional. A task with no binding is a standing row. A roster where every row is an agent process cannot express that.

## D4: Config grammar follows an existing sidebar tool

`sidebar`, `theme`, `status`, and `tasks` blocks, plus the `#[fg=...]` template directive subset and `{fill}`. Keeping the directive grammar means existing themes and templates translate without a rewrite, and it costs about 80 lines of parser.

## D5: Per-window sidebar pane

One renderer pane per tmux window, placed before the content pane. This matches the current look. The daemon reconciles the panes: it adds a missing sidebar, resizes an existing one, and removes sidebars for windows that no longer exist.

## D6: Placement is configurable

`sidebar.position` takes `left`, `right`, `top`, `bottom`. Default `left`. `left` and `right` use `sidebar.width` and a horizontal split, `top` and `bottom` use `sidebar.height` and a vertical split. Top and bottom use the `horizontal` template set, because a tile body does not fit in eight rows. A position change cannot be applied by resizing, so the daemon tears the panes down once and rebuilds them on the next tick.

## D7: The daemon never writes a pane title

Pane titles belong to whatever runs in the pane. The sidebar reads titles, it does not set them.

## D8: Public repository hygiene

No employer names, internal hostnames, ticket prefixes, or private URLs in code, tests, fixtures, or example config. Personal configuration lives outside this repository.

## D9: `set` patches, and patches upsert

`sideline set <id> --status working` sends only the named fields. The store creates the row when it does not exist yet and defaults the title to the id. Reason: a status ping from an editor or an agent happens constantly, and a full-row replace would erase a title or note that the ping never mentioned. The store test asserts a status-only patch leaves the title intact.

## D10: The daemon passes resolved paths to the renderer

A pane spawned by the tmux server does not inherit the daemon's environment, so a non-default state directory or config path would be invisible to the renderer. The daemon passes `SIDELINE_STATE_DIR` and `SIDELINE_CONFIG` to the pane with `split-window -e`.

Verified on tmux 3.8 that `-e` reaches the pane. Note that tmux also writes those values into the session environment, so later panes in that session inherit them. That is harmless here: only sideline reads those names, and a `sideline` command run from such a pane then talks to the same daemon.

## D11: The renderer gets the tmux socket the same way

A renderer that talks to the default server while the daemon runs against `--tmux-socket` lists the wrong panes. The daemon passes `SIDELINE_TMUX_SOCKET` through the pane environment and the renderer builds its client from it.

## D12: A pane binding records the tmux server start time

tmux pane ids (`%12`) are unique only within one server lifetime, so a row that stores one can point at an unrelated pane after a restart. A pane binding carries `boot`, read from `#{start_time}`, and the renderer drops a binding whose boot does not match the live server. Bindings written before this field existed are kept: dropping them would lose working rows on upgrade.

## D13: Unknown config keys are an error

`yaml.Decoder.KnownFields(true)`. A silently ignored `postion:` or `task:` reads as "the setting does nothing", which is the hardest config bug to find. A test loads `config.example.yaml` so the shipped example always parses.

## D14: Linux is a build target, verified

`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...` and the arm64 equivalent succeed, as does `GOOS=linux go vet ./...`. The only platform-sensitive calls are `syscall.Flock` and `syscall.SysProcAttr{Setsid: true}`, both present on Darwin and Linux.

## D15: Auto-reload is on by default, and an invalid edit is ignored

`sidebar.auto_reload` defaults to true: the daemon and every renderer watch the config file's mtime, parse a change, and apply it only when the parse succeeds. A half-written theme therefore never blanks the sidebar, and the previous config stays in use.

The field is a `*bool` so an absent key means on. A plain `bool` would make "not mentioned" indistinguishable from "false".

## D16: `reload` is explicit, `restart` replaces the renderers

`sideline reload` forces a config re-read even with `auto_reload: false`. The daemon reloads and bumps `State.ReloadSeq`; renderers watch the state file, so the bump is how a forced reload reaches a renderer that is not watching for mtime changes. `sideline restart` stops the daemon over the same socket, removes every sidebar pane, and starts a fresh daemon, which is the only way a newly installed binary takes effect inside existing panes.

## D17: Version is 1.0.1, and the contract is now frozen

The version lives in exactly one place, `internal/version/VERSION`, and is embedded with `go:embed`. `make`, a plain `go build`, and `go install ...@latest` therefore all report the same string, and a release bumps one file. An earlier draft stamped it with `-X main.version` from the Makefile, which would have split from the source constant at the first tag bump.

Version policy:

| Range | Meaning |
| --- | --- |
| `0.0.x` | pre-alpha. Config keys, the CLI, and the state file may change without notice |
| `0.x.y` | the command set and config keys are settling; breaking changes get a note |
| `1.0.0` | config, CLI, and state file treated as a contract |

1.0.0 is reached once the contract covered what the tool does end to end:
placement, templates, pane rows, event-driven reconcile, click and keyboard
navigation, and the sweep rules, each with tests. From here a breaking change
to the config keys, the CLI or the state file needs a major bump.

Note on the earlier belief: this entry read "the major stays 0 while the
contract moves" until the surface was frozen. The version-history table is
kept above because it still describes what a pre-1.0 release meant.

`sideline --version` prints it. `sideline status` prints the daemon's version and names a mismatch, so a daemon left over from an older install is visible as `note daemon is X, binary is Y; run sideline restart`.

## D18: The socket check is not the only single-instance guard

Two `sideline on` calls can both pass the "is anything listening" check before either binds, and unlinking a socket file lets a second daemon take the path from the first. The daemon therefore takes an exclusive `flock` on `daemon.pid` next to the socket before listening, and a second daemon exits with `ErrRunning`. The OS releases the lock on crash, so a stale pid file never blocks a restart. The pid file also gives `status` a pid to report.

## D19: The sidebar never takes focus

`split-window` makes the new pane active by default, so placing a sidebar would move the cursor out of the user's shell and every later switch into that window would land in the sidebar. The pane is created with `-d` (detached), which leaves the previous active pane in place. `splitArgs` is a pure function and a test asserts the flag is present for all four placements.

## D20: Every pane gets a row, with no shell hook

A pane with no writer was invisible, so plain shells and harnesses without an
extension never appeared. The daemon now keeps one row per unclaimed pane
(id `pane-<n>`, owner `shell`) on the tick it already runs, and the renderer
reads the pane's live foreground command and derives the status from it:
`waiting` at a shell prompt, `working` otherwise.

This qualifies D3 rather than reversing it. Rows are still tasks: a pane row
is a task the daemon maintains on the user's behalf, marked with
`owner: shell`, and it disappears with the pane. A row a person or an agent
wrote still has no owner and still outlives its pane binding.

The alternative, a `.bashrc` hook that binds the pane, was rejected: it forks
a client on every prompt, and prompt-time writes still cannot follow a
foreground command that changes without a prompt. The old tool needed a
one-second refresher per pane for exactly that reason.

## D21: Reconcile on tmux events, with a one-second fallback

The five-second tick made a new window wait up to five seconds for its sidebar
and a closed pane keep its row as long. `sideline poke` asks the daemon to
reconcile immediately, and tmux hooks call it:

```
after-new-window  after-split-window  after-kill-pane  pane-exited  window-unlinked
```

`kill-window` fires neither `after-kill-pane` nor `pane-exited`, which is why
`window-unlinked` is in the list. Pokes coalesce, so a burst costs one pass.

Measured on tmux 3.8: a pane row appears in 32 to 52 ms and a sidebar pane in
52 to 83 ms, against a second or more on the tick alone. A typed `exit` adds
five milliseconds after the pane dies; the rest of that pause is the shell
itself, measured at 253 to 282 ms with sideline out of the picture.

`pane-exited` is queried with `show-hooks -g pane-exited`, not with
`show-hooks -g`, which does not list it even when set. A timestamp probe
proved it fires before the hook was relied on.

## D22: A row bound to a dead pane is swept, whoever wrote it

Agent rows carry no owner, so the first sweep only removed `shell` rows and an
agent row outlived its pane: the sidebar showed `tmp-202` bound to a pane that
no longer existed. The sweep now clears any row whose pane binding no longer
resolves, and any binding that belongs to an older tmux server. A row with no
pane binding is never swept: that is a task in its own right.

## D23: A click selects the window, then the pane

Two facts make click-to-switch work, and both would regress silently:

1. `select-pane -t <id>` activates a pane inside its own window only. From
   another window it exits 0 and changes nothing visible. Probed on an
   isolated server: window 0 current, `select-pane -t %1` (a pane in window 1)
   returned 0 and `#{window_index}` stayed 0; `select-window -t %1` moved it
   to 1. `jump` therefore selects the window first, using the resolved pane's
   `session:window_index`, and falls back to the pane id when the pane is not
   in the list.
2. The renderer must ask for mouse reporting (`tea.WithMouseCellMotion()`)
   before tmux forwards clicks into the pane at all. With it, only presses are
   acted on, so a held button does not repaint. Measured on the live pane:
   `#{mouse_any_flag}` and `#{mouse_button_flag}` are both 1 for the renderer,
   against 0 for a content pane beside it, so the request really reaches tmux.

The hit test uses the line-to-row map produced by the same render pass that
draws the frame, so clipped rows, blank separators and padding cannot
disagree with what is on screen.

Proved end to end on an isolated server: two windows, a click sent into the
sidebar moved the session's current window from 0 to 1 and made the bound pane
active.

## D24: Releases are built from the tag by CI, never on a laptop

A tag push is the only trigger. The workflow checks that the tag equals
`internal/version/VERSION` before anything builds, so a release cannot ship a
binary that reports a different version, then runs the tests and calls
GoReleaser.

GoReleaser cross-compiles the matrix from one Ubuntu runner because sideline is
pure Go and the build sets `CGO_ENABLED=0`: darwin and linux on amd64 and
arm64, four archives plus `checksums.txt`. With cgo each target would need its
own native runner and C toolchain.

Two config details were wrong on the first pass and only `goreleaser check`
found them: the top-level key is `checksum`, not `checksums`, and
`archives.format` is deprecated in v2 in favour of `formats`.

Verified locally with GoReleaser 2.19.0: `check` reports a valid config, and
`release --snapshot --clean` produced the four archives, each holding
`sideline`, `LICENSE` and `README.md`. The extracted binary reports 1.0.0 and
`shasum -a 256 -c checksums.txt` verifies all four.
