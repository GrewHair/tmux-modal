#!/usr/bin/env bash
# What tmux tells about the terminal's focus (F48), per version. Run inside
# the tmux-src images:
#   for v in 3.0a 3.2a 3.3a 3.7c; do echo "== $v"
#     docker run --rm -v "$PWD/scripts/probes:/w:ro" "tmux-modal-tmuxsrc:$v" bash /w/focus.sh; done
# An outer server's pane runs the inner client, so writing ESC [ I / ESC [ O
# into that pane is the terminal reporting focus in / out. Shows the
# client's flags, whether the client-focus-in/out hooks exist (and take an
# array index), and that run-shell expands a user option in a hook.
flags() { echo "$1: $(tmux -L in list-clients -F '#{client_flags}')"; }
focus() { tmux -L out send-keys -H 1b 5b "$1"; sleep 0.5; } # 49 = in, 4f = out
for fe in off on; do
	tmux -L in kill-server 2>/dev/null
	tmux -L out kill-server 2>/dev/null
	sleep 0.2
	rm -f /tmp/focus.log
	tmux -L in -f /dev/null new-session -d -s a
	tmux -L in set -g focus-events "$fe"
	tmux -L in set -g @pid 4242
	tmux -L in set-hook -g 'client-focus-in[7171]' "run-shell -b 'echo in #{@pid} >> /tmp/focus.log'" 2>&1 | sed 's/^/set-hook: /'
	tmux -L in set-hook -g 'client-focus-out[7171]' "run-shell -b 'echo out >> /tmp/focus.log'" 2>/dev/null
	tmux -L out -f /dev/null new-session -d -x 80 -y 24 "tmux -L in attach -t a"
	sleep 1
	echo "-- focus-events $fe"
	flags attached
	focus 4f
	flags "after ESC[O"
	focus 49
	flags "after ESC[I"
	echo "hooks ran: $(tr '\n' ' ' 2>/dev/null </tmp/focus.log)"
done
