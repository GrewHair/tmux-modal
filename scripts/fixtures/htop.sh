#!/usr/bin/env bash
# Capture htop fixtures (tests/fixtures/htop/<version>/...).
# usage: htop.sh [label] [htop command...]
#   label defaults to the local htop version; the command can be e.g.
#   `docker run --rm -it img htop` to capture another version.
# shellcheck source=scripts/fixtures/lib.sh
source "$(dirname "$0")/lib.sh"

label=${1:-$(htop --version | awk 'NR==1{print $2}')}
shift || true
cmd=("${@:-htop}")
OUT=$ROOT/tests/fixtures/htop/$label
export HTOPRC=/dev/null
trap stop EXIT

capture_states() {
	local dir=$1 w=$2 h=$3
	start "$w" "$h" "${cmd[@]}"
	settle "${BOOT:-1.2}"
	snap "$dir/normal.txt" htop normal "default layout"
	keys /; settle
	snap "$dir/search-empty.txt" htop insert "search prompt just opened"
	keys abc; settle
	snap "$dir/search-typed.txt" htop insert "typing in search"
	keys Escape; settle 0.6
	snap "$dir/search-cancelled.txt" htop normal "search cancelled with Esc"
	keys "\\"; settle
	snap "$dir/filter-empty.txt" htop insert "filter prompt just opened"
	keys xyz; settle
	snap "$dir/filter-typed.txt" htop insert "typing in filter"
	keys Enter; settle 0.6
	snap "$dir/filter-applied.txt" htop normal "filter confirmed with Enter; filter stays active"
	keys "\\"; settle; keys BSpace BSpace BSpace Escape; settle 0.6
	keys F9; settle
	snap "$dir/kill-panel.txt" htop normal "F9 signal list: navigable"
	keys Escape; settle 0.6
	keys u; settle
	snap "$dir/user-panel.txt" htop normal "u user list: navigable"
	keys Escape; settle 0.6
	keys F2; settle
	snap "$dir/setup.txt" htop normal "F2 setup: navigable menu"
	keys Escape; settle 0.6
	keys h; settle
	snap "$dir/help.txt" htop unknown "help screen: no anchors, press any key"
	keys q; settle 0.6
	keys '#'; settle
	snap "$dir/header-hidden.txt" htop normal "header meters hidden with #"
	keys /; settle
	snap "$dir/header-hidden-search.txt" htop insert "search with header hidden"
	keys Escape; settle 0.6
	stop
}

capture_states "$OUT/200x50" 200 50
capture_states "$OUT/80x24" 80 24
