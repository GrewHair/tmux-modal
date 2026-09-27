# shellcheck shell=bash
# Resolve (and if necessary fetch or build) the tmux-modal binary.
#
# Order: an existing bin/tmux-modal whose version matches VERSION; a
# prebuilt release binary for this OS/arch; a local `go build` when a Go
# toolchain is present. Prints the path, or fails.

TMUX_MODAL_REPO="${TMUX_MODAL_REPO:-GrewHair/tmux-modal}"

tmux_modal_platform() {
	local os arch
	os="$(uname -s | tr '[:upper:]' '[:lower:]')"
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	armv7l | armv6l) arch=arm ;;
	*) return 1 ;;
	esac
	case "$os" in linux | darwin | freebsd) ;; *) return 1 ;; esac
	echo "${os}-${arch}"
}

tmux_modal_download() {
	local dir=$1 version=$2 platform url tmp
	platform="$(tmux_modal_platform)" || return 1
	url="https://github.com/${TMUX_MODAL_REPO}/releases/download/v${version}/tmux-modal-${platform}"
	tmp="$(mktemp "${dir}/bin/.dl.XXXXXX")" || return 1
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --max-time 30 -o "$tmp" "$url" || { rm -f "$tmp"; return 1; }
	elif command -v wget >/dev/null 2>&1; then
		wget -q -T 30 -O "$tmp" "$url" || { rm -f "$tmp"; return 1; }
	else
		rm -f "$tmp"
		return 1
	fi
	chmod +x "$tmp" && mv "$tmp" "${dir}/bin/tmux-modal"
}

tmux_modal_build() {
	local dir=$1
	command -v go >/dev/null 2>&1 || return 1
	(cd "$dir" && go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o bin/tmux-modal ./cmd/tmux-modal) >/dev/null 2>&1
}

tmux_modal_binary() {
	local dir=$1 version bin
	version="$(cat "$dir/VERSION" 2>/dev/null || echo dev)"
	bin="$dir/bin/tmux-modal"
	mkdir -p "$dir/bin"
	if [ -x "$bin" ] && [ "$("$bin" version 2>/dev/null)" = "tmux-modal $version" ]; then
		echo "$bin"
		return 0
	fi
	if tmux_modal_download "$dir" "$version" || tmux_modal_build "$dir"; then
		echo "$bin"
		return 0
	fi
	# A stale binary is better than none.
	if [ -x "$bin" ]; then
		echo "$bin"
		return 0
	fi
	return 1
}
