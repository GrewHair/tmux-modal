#!/usr/bin/env bash
# Capture nested-multiplexer fixtures (tests/fixtures/nested/...): htop
# running inside a tmux that itself runs "remotely" in a container, as seen
# from the local pane. Every state must classify as unknown, never as a
# mode that would remap keys.
# usage: nested.sh [image]   (default tmux-modal-sshd, tests/docker/sshd.Dockerfile)
# shellcheck source=scripts/fixtures/lib.sh
source "$(dirname "$0")/lib.sh"

image=${1:-tmux-modal-sshd}
OUT=$ROOT/tests/fixtures/nested
trap stop EXIT

# remote W H INNER-TMUX-ARGS...: the inner tmux in a throwaway container.
# --hostname: the inner status line shows the host name; keep it neutral.
# $lang: with a UTF-8 locale the inner tmux draws borders with box-drawing
# characters, without one with the VT100 line-drawing charset.
remote() {
	local w=$1 h=$2
	shift 2
	stop
	start "$w" "$h" docker run --rm -it --hostname remote --entrypoint "" -u demo \
		-e HTOPRC=/dev/null -e TERM=xterm-256color -e LANG="$lang" "$image" tmux "$@"
	settle 1.5
}
inner() { keys C-b "$@"; settle; }
# The inner command prompt opens asynchronously: text sent together with
# the prefix would reach the application instead.
inner_cmd() { keys C-b :; settle 0.3; keys "$1" Enter; settle; }

for combo in 120x36:C.UTF-8 80x24:C; do
	size=${combo%:*} lang=${combo#*:}
	w=${size%x*} h=${size#*x}
	borders=utf8
	[ "$lang" = C ] && borders=acs
	dir=$OUT/$size-$borders

	remote "$w" "$h" new-session htop
	snap "$dir/status-bottom-normal.txt" htop unknown "htop in a remote tmux, default status line at the bottom"
	keys /; settle; keys abc; settle
	snap "$dir/status-bottom-search.txt" htop unknown "search open one row above the inner status line"
	keys Escape; settle 0.6

	inner_cmd 'set status-position top'
	snap "$dir/status-top-normal.txt" htop unknown "inner status line on top: htop's bottom bar is on our last row"
	keys /; settle
	snap "$dir/status-top-search.txt" htop unknown "search, inner status on top"
	keys Escape; settle 0.6

	inner_cmd 'set status off'
	inner %
	snap "$dir/split-left-htop.txt" htop unknown "inner window split, no status: htop on the left, a shell on the right has the keyboard"
	inner_cmd 'set status on'
	inner_cmd 'set status-position bottom'
	snap "$dir/split-status.txt" htop unknown "inner split with status line"
	inner '"'
	snap "$dir/split-three.txt" htop unknown "three inner panes"

	remote "$w" "$h" new-session
	snap "$dir/shell.txt" "" unknown "a shell in the remote tmux: alternate screen held by the inner tmux"
done
