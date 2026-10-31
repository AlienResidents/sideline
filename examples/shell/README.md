# Shell integration

A pane does not need any of this to appear in the sidebar. The daemon gives
every pane a row of its own and shows the pane's live foreground command, so a
plain shell, `vim` or an agent with no integration all show up by themselves.

These helpers are for the other case: a **named task** that outlives the
command that produced it, such as a build, a deploy or a long test run.

| File | Shell | What it provides |
| --- | --- | --- |
| [`sideline.bash`](sideline.bash) | bash, zsh | `sideline_run`, `sideline_status` |
| [`sideline.fish`](sideline.fish) | fish | `sideline_run`, `sideline_status` |
| [`hooks.tmux`](hooks.tmux) | tmux | hooks that reconcile at once instead of on the tick |

## Install

Copy the file next to your config and source it from the shell's startup file.

```sh
cp sideline.bash ~/.config/sideline/sideline.bash
```

```sh
# ~/.bashrc or ~/.zshrc
[ -f ~/.config/sideline/sideline.bash ] && . ~/.config/sideline/sideline.bash
```

```fish
# ~/.config/fish/config.fish
source ~/.config/sideline/sideline.fish
```

The helpers need no running daemon. `sideline` writes the state file directly
when none answers, and the daemon adopts that file when it starts, so a row
written in a plain shell survives.

## Use

Wrap a command so the row follows it:

```sh
sideline_run deploy -- ./deploy.sh
```

The row goes `working` while the command runs, then `done`, or `blocked` with
the exit code. The command's own exit status is returned, so this is safe
under `set -e`:

```sh
set -e
sideline_run build -- make all
```

Inside tmux the row is bound to the current pane, so Enter in the sidebar jumps
back to the window that ran it.

Report a status by hand when the work is not a single command:

```sh
sideline_status inbox working
sideline_status inbox done
sideline_status deploy blocked "waiting on the database migration"
```

## Notes

- Tasks named this way have no pane binding unless they run inside tmux, so
  they stay in the roster until `sideline clear <id>`. Rows bound to a pane
  are removed when the pane closes.
- Any status string works. A name with no entry in the config's `status` block
  falls back to a plain icon and the theme text colour.
- The tmux hooks are the one part that needs tmux 3.1: `pane-exited` arrived
  there. On 3.0, leave the `hooks.tmux` lines out and sideline falls back to
  its one-second tick.
