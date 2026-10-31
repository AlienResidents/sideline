# sideline shell helpers for bash and zsh.
#
# Copy next to your config and source it from the shell's startup file:
#
#   bash:  [ -f ~/.config/sideline/sideline.bash ] && . ~/.config/sideline/sideline.bash
#   zsh:   [ -f ~/.config/sideline/sideline.bash ] && . ~/.config/sideline/sideline.bash
#
# Nothing here needs a running daemon. `sideline` writes the state file
# directly when none answers, and the daemon picks that file up when it
# starts, so a row written in a plain shell is not lost.
#
# Every pane already gets a row of its own: the daemon creates one and shows
# the pane's live foreground command. These helpers are for naming a *task*
# that outlives the command, such as a build or a deploy.

# Capability probes discard stderr on purpose: sideline being absent is a
# normal state (a machine that has not installed it yet), not an error worth
# printing on every shell start.
__sideline_available() {
    command -v sideline >/dev/null 2>&1
}

# sideline_status <id> <status> [note]
#
# Patch one field group on a row. A status with no note leaves the stored note
# alone, so pings stay lossless.
sideline_status() {
    if [ "$#" -lt 2 ]; then
        printf 'usage: sideline_status <id> <status> [note]\n' >&2
        return 2
    fi
    if [ -n "${3:-}" ]; then
        sideline set "$1" --status "$2" --note "$3"
    else
        sideline set "$1" --status "$2"
    fi
}

# sideline_run <id> -- <command> [args...]
#
# Run a command under a task row: working while it runs, done when it exits
# zero, blocked with the exit code otherwise. The command's own exit status is
# returned, so this works under `set -e` and in a pipeline.
#
# Inside tmux the row is bound to the current pane, so Enter in the sidebar
# jumps back to the window that ran it.
sideline_run() {
    if [ "$#" -lt 2 ]; then
        printf 'usage: sideline_run <id> -- <command> [args...]\n' >&2
        return 2
    fi
    local id=$1
    shift
    if [ "$1" = "--" ]; then
        shift
    fi
    if ! __sideline_available; then
        "$@"
        return $?
    fi
    if [ -n "${TMUX_PANE:-}" ]; then
        sideline set "$id" --status working --bind-pane "$TMUX_PANE" >/dev/null
    else
        sideline set "$id" --status working >/dev/null
    fi
    "$@"
    local rc=$?
    if [ "$rc" -eq 0 ]; then
        sideline_status "$id" "done"
    else
        sideline_status "$id" blocked "exit $rc"
    fi
    return "$rc"
}
