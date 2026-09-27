#!/usr/bin/env bash
# Capture nested-multiplexer fixtures (tests/fixtures/nested/...): htop
# running inside a tmux that itself runs "remotely" in a container, as seen
# from the local pane. With the inner status line removed htop's modes read
# as usual. Across inner splits only the inner pane with the cursor is read
# (vim there); with no cursor on screen (htop has the keyboard) the mode is
# unknown.
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
	snap "$dir/status-bottom-normal.txt" htop normal "htop in a remote tmux, default status line at the bottom"
	keys /; settle; keys abc; settle
	snap "$dir/status-bottom-search.txt" htop insert "search open one row above the inner status line"
	keys Escape; settle 0.6

	inner_cmd 'set status-position top'
	snap "$dir/status-top-normal.txt" htop normal "inner status line on top: htop's bottom bar is on our last row"
	keys /; settle
	snap "$dir/status-top-search.txt" htop insert "search, inner status on top"
	keys Escape; settle 0.6

	inner_cmd 'set status off'
	inner %
	snap "$dir/split-left-htop.txt" "" unknown "inner window split, no status: htop on the left, a shell on the right has the keyboard (no spec reads a shell in an inner pane)"
	inner Left
	snap "$dir/split-htop-focused.txt" htop unknown "inner split, htop has the keyboard: its cursor is hidden, so nothing shows which inner pane is focused"
	inner Right
	inner_cmd 'set status on'
	inner_cmd 'set status-position bottom'
	snap "$dir/split-status.txt" "" unknown "inner split with status line, the shell has the keyboard"
	inner '"'
	snap "$dir/split-three.txt" "" unknown "three inner panes, the bottom-right shell has the keyboard"

	# vim beside a shell: the cursor is in whichever inner pane is focused.
	remote "$w" "$h" new-session sh -c "printf 'alpha\nalpine\nalps\n' >/tmp/notes.txt && exec vim /tmp/notes.txt"
	inner %
	inner Left
	snap "$dir/split-vim-normal.txt" vim normal "inner split: vim on the left has the keyboard, a shell on the right"
	keys i; settle
	snap "$dir/split-vim-insert.txt" vim insert "inner split: vim in insert mode beside a shell"
	keys Escape; settle 1
	inner Right
	snap "$dir/split-vim-shell-focused.txt" "" unknown "inner split: the shell beside vim has the keyboard"
	inner '"'
	inner_cmd "send-keys 'exec vim /tmp/second.txt' Enter"
	settle 1
	snap "$dir/split-three-vim.txt" vim normal "three inner panes: a second vim, bottom right, has the keyboard"
	keys v; settle
	snap "$dir/split-three-vim-visual.txt" vim visual "three inner panes: visual in the bottom-right vim"
	keys Escape; settle 1

	remote "$w" "$h" new-session
	snap "$dir/shell.txt" "" unknown "a shell in the remote tmux: alternate screen held by the inner tmux"
done
