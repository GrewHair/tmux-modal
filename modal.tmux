#!/usr/bin/env bash
# tmux-modal — TPM entry point.
#
# Resolves the tmux-modal binary and starts one detector daemon for this
# tmux server. Everything else (key tables, pane options) is managed by the
# daemon. If no binary can be obtained the plugin stays inert: keys pass
# through untouched, which is always the safe failure mode.
CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=scripts/binary.sh
source "$CURRENT_DIR/scripts/binary.sh"

main() {
	local bin
	if ! bin="$(tmux_modal_binary "$CURRENT_DIR")"; then
		tmux display-message "tmux-modal: no binary for $(uname -s)/$(uname -m) and no Go toolchain; plugin inactive (see README)"
		return 0
	fi
	tmux set-option -gq @modal_bin "$bin"
	if [ "$(tmux show-option -gqv @modal_enabled)" = "off" ]; then
		return 0
	fi
	# One daemon per server: a second start exits at once on the lock.
	# The socket is passed explicitly (run-shell expands the format).
	tmux run-shell -b "'$bin' daemon --socket '#{socket_path}' >/dev/null 2>&1 || true"
}

main
