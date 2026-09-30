#!/usr/bin/env bash
# Manual check of the cursor badge on a tmux that reports #{cursor_shape}
# (>= 3.5), with the real daemon and nvim (F45). Run inside a tmux-src image:
#   CGO_ENABLED=0 go build -o /tmp/tm/tm ./cmd/tmux-modal && cp scripts/probes/cursor-shape.sh /tmp/tm/
#   docker run --rm -u root -v /tmp/tm:/w:ro tmux-modal-tmuxsrc:3.7c bash /w/cursor-shape.sh
# The inner server runs nvim; an outer server's pane is the attached "human"
# client the daemon needs to publish anything.
tmux -L in -f /dev/null new-session -d -s m -x 100 -y 30 nvim
tmux -L in set -g @modal_log_level debug
tmux -L out -f /dev/null new-session -d -x 100 -y 30 "tmux -L in attach -t m"
sleep 1
/w/tm daemon -L in --log /tmp/d.log &
sleep 3
show() { echo "$1: shape=[$(tmux -L in display -p -t %0 '#{cursor_shape}')] published=[$(tmux -L in display -p -t %0 '#{@modal_cursor_shape}')] badges=[$(tmux -L in display -p -t %0 '#{@modal_badges}' | sed 's/#\[[^]]*\]//g')]"; }
show normal
tmux -L out send-keys i; sleep 1.5
show insert
tmux -L out send-keys Escape; sleep 1.5
show "back to normal"
