#!/bin/sh
# debconf-probe [QUESTION...]: ask debconf questions the way packages do
# (debconf-probe.templates, loaded when the image is built), through
# debconf's dialog frontend (whiptail). Modelled on real prompts:
# needrestart's service list and kernel note, tzdata's area menu, postfix's
# mail name, a database password, iptables-persistent's yes/no.
# Default: every question once, in that order. Run as root (sudo).
if [ -z "${DEBIAN_HAS_FRONTEND:-}" ]; then
	DEBIAN_FRONTEND=${DEBIAN_FRONTEND:-dialog}
	export DEBIAN_FRONTEND
fi
# shellcheck disable=SC1091
. /usr/share/debconf/confmodule
db_capb backup
[ $# -gt 0 ] || set -- restart kernel area mailname password save
for q in "$@"; do
	db_fset "tmux-modal-probe/$q" seen false
	if [ "$q" = restart ] || [ "$q" = kernel ]; then
		db_settitle tmux-modal-probe/title
	else
		db_title "Configuring tmux-modal-probe"
	fi
	db_input critical "tmux-modal-probe/$q" || true
	db_go || true
done
