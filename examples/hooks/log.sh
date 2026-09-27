#!/bin/sh
# Transition hook: append one line per transition to a log you can tail.
#
#   set -g @modal_transition_hook '~/.config/tmux/plugins/tmux-modal/examples/hooks/log.sh'
#   tail -f ~/.local/state/tmux-modal/transitions.log
dir="${XDG_STATE_HOME:-$HOME/.local/state}/tmux-modal"
mkdir -p "$dir"
printf '%s %-5s %-4s session=%s app=%s %s -> %s typing=%s confidence=%s active=%s\n' \
	"$(date +%H:%M:%S)" "$MODAL_EVENT" "$MODAL_PANE" "$MODAL_SESSION" "${MODAL_APP:--}" \
	"${MODAL_MODE_FROM:--}" "$MODAL_MODE_TO" "$MODAL_TYPING" "$MODAL_CONFIDENCE" "$MODAL_PANE_ACTIVE" \
	>>"$dir/transitions.log"
