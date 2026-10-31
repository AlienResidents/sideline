# Decisions

Settled choices, with the evidence behind them. Proposals do not belong here.

## D1: Go, not Rust

Evidence: `go version go1.27.2 darwin/arm64` is installed. `cargo` and `rustc` are not. A Go build with `CGO_ENABLED=0` produces darwin/arm64, linux/amd64, and linux/arm64 binaries from one machine with no extra toolchain. The daemon does one tmux poll loop and one socket read loop, so the memory and latency arguments for Rust do not apply.

## D2: One daemon owns the state file

Producers (CLI, editor hooks, agent extensions) send a JSON line over a unix socket. The daemon applies the change, writes the file atomically, and notifies renderers. Two writers never race on one file, and the pane-title fight that workmux has (daemon rewrites, extension re-asserts every 5 s) cannot happen because only the extension owns pane titles.

## D3: Tasks, not agents

The roster is a list of tasks. A task may bind to a tmux pane, a window, or an external target, but binding is optional. A task with no binding is a standing row. This is the property workmux cannot express, because every workmux row is an agent process.

## D4: Config grammar stays close to workmux

`sidebar`, `theme`, `status`, and `tasks` blocks, plus the `#[fg=...]` template directive subset and `{fill}`. Keeping the directive grammar means existing themes and templates translate without a rewrite, and it costs about 80 lines of parser.

## D5: Per-window sidebar pane

One renderer pane per tmux window, placed before the content pane. This matches the current look. The daemon reconciles the panes: it adds a missing sidebar, resizes an existing one, and removes sidebars for windows that no longer exist.

## D6: Placement is configurable

`sidebar.position` takes `left`, `right`, `top`, `bottom`. Default `left`. `left` and `right` use `sidebar.width` and a horizontal split, `top` and `bottom` use `sidebar.height` and a vertical split. Top and bottom use the `horizontal` template set, because a tile body does not fit in eight rows.

## D7: The daemon never writes a pane title

Pane titles belong to whatever runs in the pane. The sidebar reads titles, it does not set them.

## D8: Public repository hygiene

No employer names, internal hostnames, ticket prefixes, or private URLs in code, tests, fixtures, or example config. Personal configuration lives outside this repository.

## D9: `set` patches, and patches upsert

`sideline set <id> --status working` sends only the named fields. The store
creates the row when it does not exist yet and defaults the title to the id.
Reason: a status ping from an editor or an agent happens constantly, and a
full-row replace would erase a title or note that the ping never mentioned.
The store test asserts a status-only patch leaves the title intact.

## D10: The daemon passes resolved paths to the renderer

A pane spawned by the tmux server does not inherit the daemon's environment,
so a non-default state directory or config path would be invisible to the
renderer. The daemon passes `SIDELINE_STATE_DIR` and `SIDELINE_CONFIG` to the
pane with `split-window -e`, which sets them on that pane only.
