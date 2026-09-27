#!/usr/bin/env bash
# Example long-lived listener for examples/hooks/fifo.sh. Replace the body
# of the loop with whatever should react (toggle a keyboard layer, etc.).
fifo="${MODAL_FIFO:-${XDG_RUNTIME_DIR:-/tmp}/tmux-modal.fifo}"
[ -p "$fifo" ] || mkfifo "$fifo"
echo "listening on $fifo" >&2
while :; do
	while read -r typing bucket mode app pane event; do
		echo "$(date +%T.%3N) typing=$typing bucket=$bucket mode=$mode app=$app pane=$pane ($event)"
	done <"$fifo"
done
