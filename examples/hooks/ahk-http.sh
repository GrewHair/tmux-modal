#!/bin/sh
# Transition hook for an AutoHotkey v2 script on the Windows side of WSL
# that serves HTTP on localhost: each transition of the focused pane calls
#
#   <MODAL_AHK_URL>?<MODAL_AHK_FUNC>;;;;<app>/<mode>/<typing|commanding>/<ms>
#
# e.g. .../send/F?OSD;;;;vim/insert/typing/1759012345678. The third field
# is from MODAL_TYPING: "commanding" only when the daemon is sure (a shell
# or an unrecognised app is "typing"); "-" stands for no app. The last is
# the transition's time in milliseconds (MODAL_TIMESTAMP_MS).
#
# The pane is the one you type into: with tmux's focus-events on, the call
# is repeated whenever a terminal regains focus (back from another window)
# and a terminal you are not looking at is left out. With
# `set -g @modal_hook_blur on`, leaving the terminal sends "-/none/typing".
#
# Fire and forget: curl.exe is started detached and the hook returns at
# once, never waiting for a response. So two quick transitions may arrive
# out of order (process start varies by tens of ms): the receiver should
# ignore a request older than the last one it acted on. A server that is
# not running refuses at once; one that hangs is given up on after 2 s,
# in the background.
#
#   set -g @modal_transition_hook '~/.config/tmux/plugins/tmux-modal/examples/hooks/ahk-http.sh'
#   set -g @modal_transition_hook 'MODAL_AHK_FUNC=Layer ~/.config/tmux/plugins/tmux-modal/examples/hooks/ahk-http.sh'
#
# Windows curl.exe, because under WSL's default (NAT) networking the
# Windows localhost is not reachable from Linux. Starting it takes ~80 ms
# (the request itself ~3 ms), after this hook has already returned.
url="${MODAL_AHK_URL:-http://localhost:42800/send/F}"
func="${MODAL_AHK_FUNC:-OSD}"
curl="${MODAL_AHK_CURL:-/mnt/c/Windows/System32/curl.exe}"

# Only the pane the keyboard goes to (other panes, with a wider
# @modal_scope, arrive with MODAL_PANE_ACTIVE=0).
[ "$MODAL_PANE_ACTIVE" = 1 ] || exit 0

if [ "$MODAL_TYPING" = 0 ]; then kind=commanding; else kind=typing; fi
payload="${MODAL_APP:--}/${MODAL_MODE_TO}/${kind}/${MODAL_TIMESTAMP_MS}"
setsid "$curl" -s -o /dev/null --connect-timeout 1 -m 2 "${url}?${func};;;;${payload}" \
	>/dev/null 2>&1 </dev/null &
