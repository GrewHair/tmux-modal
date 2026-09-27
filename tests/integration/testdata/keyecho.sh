#!/usr/bin/env bash
# keyecho: a deterministic full-screen test application.
#
# Shows the name of the last key it received. '/' opens a PROMPT> text
# prompt on the last row (cursor visible there); Enter or Escape closes
# it. 'q' in normal mode quits. The first line is the identity banner,
# "$KEYECHO_NAME TEST APPLICATION" (default KEYECHO).
NAME=${KEYECHO_NAME:-KEYECHO}
mode=normal last=none buf=""
cleanup() { printf '\e[?25h\e[?1049l'; stty sane; }
trap cleanup EXIT
trap draw WINCH
printf '\e[?1049h'
stty -echo -icanon min 1 time 0

draw() {
	local lines
	lines=$(tput lines)
	printf '\e[?25l\e[H\e[2J%s TEST APPLICATION v1\r\n' "$NAME"
	printf 'last=[%s]\r\n' "$last"
	if [ "$mode" = prompt ]; then
		printf '\e[%d;1HPROMPT> %s' "$lines" "$buf"
		printf '\e[?25h'
	else
		printf '\e[%d;1Hkeyecho normal mode status bar' "$lines"
	fi
}

keyname() {
	local k=$1 rest
	if [ "$k" = $'\e' ]; then
		IFS= read -rsn2 -t 0.02 rest || true
		case "$rest" in
		'[A') echo Up ;; '[B') echo Down ;; '[C') echo Right ;; '[D') echo Left ;;
		'[H') echo Home ;; '[F') echo End ;;
		'[5') read -rsn1 -t 0.02 _; echo PageUp ;;
		'[6') read -rsn1 -t 0.02 _; echo PageDown ;;
		'[1') read -rsn1 -t 0.02 _; echo Home ;;
		'[4') read -rsn1 -t 0.02 _; echo End ;;
		'') echo Escape ;;
		*) echo "ESC$rest" ;;
		esac
		return
	fi
	case "$k" in
	'') echo Enter ;;
	$'\x04') echo C-d ;; $'\x15') echo C-u ;;
	*) printf '%s\n' "$k" ;;
	esac
}

draw
# Poll-read so the WINCH trap can run (bash defers traps while a read
# blocks); real TUIs redraw on resize, and so must this one.
while true; do
	IFS= read -rsn1 -t 0.1 k
	rc=$?
	if [ $rc -gt 128 ]; then continue; fi # timeout
	if [ $rc -ne 0 ]; then exit 0; fi     # EOF
	name=$(keyname "$k")
	last=$name
	if [ "$mode" = prompt ]; then
		case "$name" in
		Enter | Escape) mode=normal buf="" ;;
		*) buf+=$name ;;
		esac
	else
		case "$name" in
		/) mode=prompt ;;
		q) exit 0 ;;
		esac
	fi
	draw
done
