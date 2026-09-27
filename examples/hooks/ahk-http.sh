#!/bin/sh
# Transition hook for an AutoHotkey v2 script on the Windows side of WSL
# that serves HTTP on localhost: each transition of the focused pane calls
#
#   <MODAL_AHK_URL>?<MODAL_AHK_FUNC>;;;;<app>/<mode>/<typing|commanding>
#
# e.g. .../send/F?OSD;;;;vim/insert/typing. The last field is from
# MODAL_TYPING: "commanding" only when the daemon is sure (a shell or an
# unrecognised app is "typing"); "-" stands for no app.
#
#   set -g @modal_transition_hook '~/.config/tmux/plugins/tmux-modal/examples/hooks/ahk-http.sh'
#   set -g @modal_transition_hook 'MODAL_AHK_FUNC=Layer ~/.config/tmux/plugins/tmux-modal/examples/hooks/ahk-http.sh'
#
# Windows curl.exe, because under WSL's default (NAT) networking the
# Windows localhost is not reachable from Linux. Starting it costs ~80 ms
# per call (the request itself ~3 ms).
url="${MODAL_AHK_URL:-http://localhost:42800/send/F}"
func="${MODAL_AHK_FUNC:-OSD}"
curl="${MODAL_AHK_CURL:-/mnt/c/Windows/System32/curl.exe}"

# Only the pane the keyboard goes to (other panes, with a wider
# @modal_scope, arrive with MODAL_PANE_ACTIVE=0).
[ "$MODAL_PANE_ACTIVE" = 1 ] || exit 0

if [ "$MODAL_TYPING" = 0 ]; then kind=commanding; else kind=typing; fi
payload="${MODAL_APP:--}/${MODAL_MODE_TO}/${kind}"
exec "$curl" -s -o /dev/null -m 1 "${url}?${func};;;;${payload}"
