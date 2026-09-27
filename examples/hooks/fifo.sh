#!/bin/sh
# Transition hook, low-latency pattern: write one line to a FIFO that a
# long-lived listener reads. The hook itself costs one tiny shell; the
# listener (AutoHotkey bridge, kanata, a script) stays warm, so a
# transition reaches it in about a millisecond instead of paying a process
# start (tens to hundreds of ms, more across a WSL/Windows boundary).
#
#   set -g @modal_transition_hook '~/.config/tmux/plugins/tmux-modal/examples/hooks/fifo.sh'
#   ~/.config/tmux/plugins/tmux-modal/examples/listener.sh     # in any terminal
#
# Line format: <typing 0|1> <bucket> <mode> <app> <pane> <event>
fifo="${MODAL_FIFO:-${XDG_RUNTIME_DIR:-/tmp}/tmux-modal.fifo}"
[ -p "$fifo" ] || exit 0 # nobody listening
# Opening read-write never blocks, even when no reader is attached.
exec 3<>"$fifo"
printf '%s %s %s %s %s %s\n' "$MODAL_TYPING" "$MODAL_BUCKET" "$MODAL_MODE_TO" \
	"${MODAL_APP:--}" "$MODAL_PANE" "$MODAL_EVENT" >&3
