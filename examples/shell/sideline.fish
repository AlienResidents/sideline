# sideline helpers for fish.
#
# Copy to ~/.config/fish/functions/ (one file per function, named after it),
# or source it from ~/.config/fish/config.fish:
#
#   source ~/.config/sideline/sideline.fish
#
# These mirror examples/shell/sideline.bash. Every pane already gets its own
# row from the daemon; these helpers are for naming a task that outlives the
# command, such as a build or a deploy.

# sideline_status <id> <status> [note]
function sideline_status --description 'Patch a sideline row status'
    if test (count $argv) -lt 2
        echo 'usage: sideline_status <id> <status> [note]' >&2
        return 2
    end
    if test -n "$argv[3]"
        sideline set $argv[1] --status $argv[2] --note "$argv[3]"
    else
        sideline set $argv[1] --status $argv[2]
    end
end

# sideline_run <id> -- <command> [args...]
function sideline_run --description 'Run a command as a sideline task row'
    if test (count $argv) -lt 2
        echo 'usage: sideline_run <id> -- <command> [args...]' >&2
        return 2
    end
    set -l id $argv[1]
    set -l rest $argv[2..-1]
    if test "$rest[1]" = "--"
        set rest $rest[2..-1]
    end
    # A missing sideline must not get in the way of the command.
    if not command -q sideline
        $rest
        return $status
    end
    if set -q TMUX_PANE
        sideline set $id --status working --bind-pane $TMUX_PANE >/dev/null
    else
        sideline set $id --status working >/dev/null
    end
    $rest
    set -l rc $status
    if test $rc -eq 0
        sideline_status $id done
    else
        sideline_status $id blocked "exit $rc"
    end
    return $rc
end
