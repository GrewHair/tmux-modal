#!/bin/sh
# ucf-probe: the real "Modified configuration file" prompt that package
# upgrades show (ucf, through debconf's dialog frontend): install a config
# file, edit it locally, then offer a new version. Run as root (sudo).
set -e
DEBIAN_FRONTEND=${DEBIAN_FRONTEND:-dialog}
export DEBIAN_FRONTEND
dir=$(mktemp -d)
conf=/etc/tmux-modal-probe.conf
ucf --purge "$conf" 2>/dev/null || true
rm -f "$conf"
printf 'setting = 1\n' >"$dir/v1"
ucf "$dir/v1" "$conf"
printf 'local = yes\n' >>"$conf"
printf 'setting = 2\n' >"$dir/v2"
ucf "$dir/v2" "$conf"
rm -rf "$dir"
