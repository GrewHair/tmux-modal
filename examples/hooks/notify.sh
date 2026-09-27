#!/bin/sh
# Transition hook: a desktop notification per transition (Linux
# notify-send, or terminal-notifier on macOS). Simple, but it spawns a
# process per transition; see fifo.sh for the low-latency pattern.
msg="${MODAL_APP:-shell}: ${MODAL_MODE_TO} (typing=${MODAL_TYPING})"
if command -v notify-send >/dev/null 2>&1; then
	notify-send -t 1500 tmux-modal "$msg"
elif command -v terminal-notifier >/dev/null 2>&1; then
	terminal-notifier -title tmux-modal -message "$msg"
fi
