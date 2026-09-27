#!/bin/sh
# Install every application a bundled spec covers, for fixture capture and
# the SSH integration tier. Packages missing from the base are skipped.
#   APPS             package list (default: all of them)
#   LAZYGIT_VERSION  K9S_VERSION  NVIM_VERSION  release to download
#                    (upstream nvim goes to /opt/nvim, as nvim-upstream);
#                    empty skips it
#   LOCALES          UTF-8 locales to generate (vim's translated markers)
set -eu
want=${APPS:-"htop less man-db vim-nox neovim tig ncdu btop ranger lf nnn mc fzf git
python3 sqlite3 gdb postgresql-client mariadb-client nodejs ruby weechat irssi
ca-certificates curl locales"}
# Minimised images strip man pages; man needs them.
rm -f /etc/dpkg/dpkg.cfg.d/excludes
if [ "$(dpkg-divert --truename /usr/bin/man)" = /usr/bin/man.REAL ]; then
	rm -f /usr/bin/man
	dpkg-divert --quiet --remove --rename /usr/bin/man
fi
apt-get update
have=""
for p in $want; do
	if apt-cache show "$p" >/dev/null 2>&1; then
		have="$have $p"
	else
		echo "apps-install: $p not available on this base, skipped"
	fi
done
# shellcheck disable=SC2086
apt-get install -y --no-install-recommends $have
apt-get install -y --no-install-recommends --reinstall coreutils >/dev/null
case $(uname -m) in
x86_64) arch=x86_64 k9arch=amd64 ;;
aarch64) arch=arm64 k9arch=arm64 ;;
*) arch="" ;;
esac
if [ -n "$arch" ] && [ -n "${LAZYGIT_VERSION:-}" ]; then
	curl -fsSL "https://github.com/jesseduffield/lazygit/releases/download/v${LAZYGIT_VERSION}/lazygit_${LAZYGIT_VERSION}_Linux_${arch}.tar.gz" |
		tar -xz -C /usr/local/bin lazygit
fi
if [ -n "$arch" ] && [ -n "${K9S_VERSION:-}" ]; then
	curl -fsSL "https://github.com/derailed/k9s/releases/download/v${K9S_VERSION}/k9s_Linux_${k9arch}.tar.gz" |
		tar -xz -C /usr/local/bin k9s
fi
if [ -n "$arch" ] && [ -n "${NVIM_VERSION:-}" ]; then
	mkdir -p /opt/nvim
	curl -fsSL "https://github.com/neovim/neovim/releases/download/v${NVIM_VERSION}/nvim-linux-${arch}.tar.gz" |
		tar -xz -C /opt/nvim --strip-components=1
	ln -sf /opt/nvim/bin/nvim /usr/local/bin/nvim-upstream
fi
if command -v localedef >/dev/null; then
	for l in ${LOCALES:-de_DE ru_RU ja_JP fr_FR}; do
		localedef -i "$l" -f UTF-8 "$l.UTF-8"
	done
fi
rm -rf /var/lib/apt/lists/*
if [ -x /usr/bin/mandb ]; then
	mandb -q || true
fi
