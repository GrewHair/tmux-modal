# Shared helpers for fixture capture scripts. Source, don't run.
#
# Every capture uses a dedicated -L socket and -f /dev/null so it never
# touches the developer's own tmux server or config.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
BIN=${TMUX_MODAL_BIN:-$ROOT/bin/tmux-modal}
SOCK=fx-$$-$RANDOM

t() { tmux -L "$SOCK" "$@"; }

# start W H CMD...: new headless server running CMD in pane %0
start() {
	local w=$1 h=$2
	shift 2
	t -f /dev/null new-session -d -x "$w" -y "$h" "$@"
	t set-option -g status off
	t resize-window -x "$w" -y "$h" 2>/dev/null || true
}

stop() { t kill-server 2>/dev/null || true; }

keys() { t send-keys -t %0 "$@"; }

settle() { sleep "${1:-0.4}"; }

# snap FILE APP MODE [NOTE]: save pane %0 as a fixture
snap() {
	local file=$1 app=$2 mode=$3 note=${4:-}
	mkdir -p "$(dirname "$file")"
	TMUX="$(t display -p '#{socket_path}'),0,0" "$BIN" capture \
		-o "$file" -expect-app "$app" -expect-mode "$mode" -note "$note" %0
	echo "captured $file ($app/$mode)"
}
