# tmux hooks that make the sidebar react to the window tree at once.
#
# Add to ~/.tmux.conf after the autostart line, then reload with
#   tmux source-file ~/.tmux.conf
#
# Without these, sideline still works: it reconciles on a one-second tick.
# With them, a row appears or disappears in tens of milliseconds.
#
# `sideline poke` exits 0 when no daemon is running, so these lines are
# harmless while the sidebar is off.

# Start the sidebar when a session is created. `on` is idempotent: it starts
# the daemon only when none answers.
set-hook -g session-created 'run-shell "sideline on"'

# Reconcile now, rather than on the next tick.
set-hook -g after-new-window   'run-shell -b "sideline poke"'
set-hook -g after-split-window 'run-shell -b "sideline poke"'
set-hook -g after-kill-pane    'run-shell -b "sideline poke"'
set-hook -g pane-exited        'run-shell -b "sideline poke"'
set-hook -g window-unlinked    'run-shell -b "sideline poke"'

# ---- Mouse -------------------------------------------------------------------
# Clicking a row switches to its window, which needs tmux to forward mouse
# events into the sidebar pane. `set -g mouse on` is the usual setting.
#
# If you rebind MouseDown1Pane, keep the forward: without `send-keys -M` the
# click stops at tmux and never reaches sideline.
set -g mouse on
