#!/usr/bin/env bash
# Capture fixtures for the bundled specs (other than htop) inside the
# clean-room image, so no host data ends up in them:
#   tests/fixtures/<app>/<version>/<WxH>/<state>.txt (+ .ansi)
#
# usage: apps.sh [image-tag] [app...]
#   image-tag defaults to ubuntu-24.04 (image tmux-modal-fx:<tag>, built
#   from tests/docker/apps.Dockerfile); apps default to all of them.
#
# Most pane commands are `docker`, so the fixtures double as the remote
# (SSH) case. Where a state carries no identity anchor, identity only comes
# from the local command or from an earlier capture (sticky), and the
# fixture says so with expect_mode_remote/expect_mode_mono=unknown.
# shellcheck source=scripts/fixtures/lib.sh
source "$(dirname "$0")/lib.sh"

tag=${1:-ubuntu-24.04}
shift || true
IMG=tmux-modal-fx:$tag
apps=("$@")
[ ${#apps[@]} -gt 0 ] || apps=(less man tig ncdu btop k9s lazygit ranger lf nnn mc fzf vim nvim debconf whiptail)
trap stop EXIT

# ncurses waits ESCDELAY (1 s) after Escape to tell it from a sequence.
ESC_SETTLE=1.3
# States with no identity anchor on screen.
ANCHORLESS=(expect_mode_remote=unknown expect_mode_mono=unknown)

# app W H DIR CMD...: start CMD in the image, in DIR
app() {
	local w=$1 h=$2 dir=$3
	shift 3
	start "$w" "$h" docker run --rm -it --hostname fx -w "$dir" "$IMG" "$@"
	settle "${BOOT:-2}"
}

# version CMD FIELD: FIELD of the first line CMD prints in the image
version() { docker run --rm "$IMG" sh -c "$1" 2>/dev/null | awk -v f="$2" 'NR==1{print $f}'; }

cap_less() {
	local out
	out=$ROOT/tests/fixtures/less/$(version 'less --version' 2)
	for size in 100x30 80x24; do
		local d=$out/$size
		app "${size%x*}" "${size#*x}" /home/demo less /home/demo/sample.txt
		snap "$d/first.txt" less normal "file name prompt in reverse video; starts with / like a search" \
			"${ANCHORLESS[@]}"
		keys j; settle
		snap "$d/idle.txt" less normal "idle prompt ':' after moving" "${ANCHORLESS[@]}"
		keys /; settle
		snap "$d/search-empty.txt" less insert "search prompt just opened" "${ANCHORLESS[@]}"
		keys -l 'line 5'; settle
		snap "$d/search-typed.txt" less insert "typing a search" "${ANCHORLESS[@]}"
		keys Enter; settle
		snap "$d/search-found.txt" less normal "search done" "${ANCHORLESS[@]}"
		keys '?'; settle
		snap "$d/search-backward.txt" less insert "backward search prompt" "${ANCHORLESS[@]}"
		keys C-c; settle
		keys C-n; settle
		keys /; settle
		keys C-n; settle
		snap "$d/search-modifier.txt" less insert "search with the Non-match modifier" "${ANCHORLESS[@]}"
		keys C-c; settle
		keys /; keys -l zzz; keys Enter; settle
		snap "$d/not-found.txt" less normal "Pattern not found (press RETURN)"
		keys Enter; settle
		keys '&'; settle
		keys -l line; settle
		snap "$d/filter-typed.txt" less insert "filter prompt &/" "${ANCHORLESS[@]}"
		keys Enter; settle
		snap "$d/filter-applied.txt" less normal "filter on: '& :'" "${ANCHORLESS[@]}"
		keys '&'; keys Enter; settle
		keys -; settle
		snap "$d/option.txt" less insert "option prompt '-'" "${ANCHORLESS[@]}"
		keys C-c; settle
		keys :; keys e; settle
		snap "$d/examine.txt" less insert "Examine: prompt (:e)" "${ANCHORLESS[@]}"
		keys C-c; settle
		keys '!'; settle
		snap "$d/shell.txt" less insert "shell command prompt !" "${ANCHORLESS[@]}"
		keys C-c; settle
		keys G; settle
		snap "$d/end.txt" less normal "(END)"
		keys h; settle
		snap "$d/help.txt" less normal "help screen"
		keys q; settle
		stop
	done
	# A pipe: no file name, the idle prompt from the start; and a relative
	# file name, which does not look like a search.
	app 80 24 /home/demo bash -c 'seq 1000 | less'
	snap "$out/80x24/pipe.txt" less normal "reading a pipe: ':' from the start" "${ANCHORLESS[@]}"
	stop
	app 80 24 /home/demo less sample.txt
	snap "$out/80x24/first-relative.txt" less normal "relative file name prompt" "${ANCHORLESS[@]}"
	stop
}

cap_man() {
	local out
	out=$ROOT/tests/fixtures/man/$(version 'man --version' 2)
	for size in 100x30 80x24; do
		local d=$out/$size
		app "${size%x*}" "${size#*x}" /home/demo man ls
		snap "$d/first.txt" man normal "man bar"
		keys j j; settle
		snap "$d/scrolled.txt" man normal "man bar after moving"
		keys /; settle
		snap "$d/search-empty.txt" man insert "search prompt just opened" "${ANCHORLESS[@]}"
		keys -l sort; settle
		snap "$d/search-typed.txt" man insert "typing a search" "${ANCHORLESS[@]}"
		keys Enter; settle
		snap "$d/search-found.txt" man normal "search done"
		keys G; settle
		snap "$d/end.txt" man normal "at the end"
		stop
	done
}

cap_tig() {
	local out
	out=$ROOT/tests/fixtures/tig/$(version 'tig --version' 3)
	for size in 120x36 80x24; do
		local d=$out/$size
		app "${size%x*}" "${size#*x}" /home/demo/repo tig
		snap "$d/main.txt" tig normal "main view"
		keys /; settle
		snap "$d/search-empty.txt" tig insert "search prompt"
		keys -l file; settle
		snap "$d/search-typed.txt" tig insert "typing a search"
		keys Enter; settle
		snap "$d/search-done.txt" tig normal "search done, message on the last row"
		keys '?'; settle
		snap "$d/search-backward.txt" tig insert "backward search prompt"
		keys C-c; settle
		keys :; settle
		snap "$d/command-empty.txt" tig command "command prompt"
		keys -l 'help'; settle
		snap "$d/command-typed.txt" tig command "typing a command"
		keys C-c; settle
		keys j; keys Enter; settle
		snap "$d/diff-split.txt" tig normal "commit diff in a split view"
		keys /; settle
		snap "$d/diff-split-search.txt" tig insert "search in the split diff"
		keys C-c; settle
		keys q; settle
		keys h; settle
		snap "$d/help.txt" tig normal "help view"
		keys q; settle
		keys s; settle
		snap "$d/status.txt" tig normal "status view"
		stop
	done
}

cap_ncdu() {
	local out
	out=$ROOT/tests/fixtures/ncdu/$(version 'ncdu -v' 2)
	for size in 100x30 80x24; do
		local d=$out/$size
		app "${size%x*}" "${size#*x}" /home/demo ncdu /home/demo/tree
		snap "$d/main.txt" ncdu normal "directory list"
		keys d; settle
		snap "$d/delete-confirm.txt" ncdu normal "delete confirmation: a yes/no selection"
		keys n; settle
		keys i; settle
		snap "$d/info.txt" ncdu normal "item info"
		keys i; settle
		keys l; settle
		snap "$d/subdir.txt" ncdu normal "inside a directory"
		stop
	done
}

# btop shows the host's CPU model in its CPU box title; replace it (in the
# .txt and .ansi files) with a neutral name of the same width. The name is
# learnt from whichever capture shows it whole; overlays can leave only its
# tail visible, so tails of 5+ characters are replaced too.
sanitize_btop() {
	python3 - "$@" <<'PY'
import re, sys
txts = sys.argv[1:]
names = set()
for txt in txts:
    for m in re.finditer(r"╭─┐([^┌│]+)┌─*┐[0-9.]+ GHz┌", open(txt, encoding="utf-8").read()):
        names.add(m.group(1))
subs = {}
for name in names:
    for i in range(len(name) - 4):
        tail = name[i:]
        subs.setdefault(tail, ("CPU" + " " * len(name))[i:i + len(tail)] if i == 0 else " " * len(tail))
for txt in txts:
    for f in (txt, txt[:-4] + ".ansi"):
        try:
            data = open(f, encoding="utf-8").read()
        except FileNotFoundError:
            continue
        for tail in sorted(subs, key=len, reverse=True):
            data = data.replace(tail, subs[tail])
        open(f, "w", encoding="utf-8").write(data)
PY
}

cap_btop() {
	local out
	out=$ROOT/tests/fixtures/btop/$(version 'btop -v' 3)
	for size in 120x40 80x24; do
		local d=$out/$size
		BOOT=3 app "${size%x*}" "${size#*x}" /home/demo btop
		snap "$d/normal.txt" btop normal "default layout"
		# btop drops keys that arrive in one burst: type one at a time.
		keys f; settle 2.5
		snap "$d/filter-empty.txt" btop insert "filter just opened in the proc box title"
		keys b; settle 0.6; keys t; settle 0.6; keys o; settle 2.5
		snap "$d/filter-typed.txt" btop insert "typing the filter"
		keys Enter; settle 2.5
		snap "$d/filter-applied.txt" btop normal "filter applied: 'del' instead of the Enter mark"
		keys Delete; settle 2.5
		keys s; settle 2.5
		snap "$d/signals.txt" btop normal "signal list: arrows choose a signal"
		keys Escape; settle 2.5
		keys m; settle 2.5
		snap "$d/menu.txt" btop normal "main menu"
		keys o; settle 2.5
		snap "$d/options.txt" btop insert "options: editable fields, never remapped; covers the proc box" \
			"${ANCHORLESS[@]}"
		keys Escape; settle 2.5
		keys '?'; settle 2.5
		snap "$d/help.txt" btop normal "help overlay; covers the proc box" "${ANCHORLESS[@]}"
		keys Escape; settle 2.5
		stop
		sanitize_btop "$d"/*.txt
	done
}

cap_k9s() {
	local out
	out=$ROOT/tests/fixtures/k9s/$(version 'k9s version -s' 2)
	for size in 120x30 80x24; do
		local d=$out/$size
		BOOT=5 app "${size%x*}" "${size#*x}" /home/demo k9s
		snap "$d/main.txt" k9s normal "no cluster: contexts view"
		keys :; settle
		snap "$d/command-empty.txt" k9s command "command prompt"
		keys -l po; settle
		snap "$d/command-typed.txt" k9s command "typing a command (with suggestion)"
		keys Escape; settle "$ESC_SETTLE"
		keys /; settle
		snap "$d/filter-empty.txt" k9s insert "filter prompt"
		keys -l abc; settle
		snap "$d/filter-typed.txt" k9s insert "typing a filter"
		keys Enter; settle
		snap "$d/filter-applied.txt" k9s normal "filter applied, shown in the table title"
		keys Escape; settle "$ESC_SETTLE"
		keys '?'; settle
		snap "$d/help.txt" k9s normal "help"
		keys Escape; settle "$ESC_SETTLE"
		stop
	done
}

cap_lazygit() {
	local out
	out=$ROOT/tests/fixtures/lazygit/$(docker run --rm "$IMG" lazygit --version | sed 's/.*version=\([^,]*\),.*/\1/')
	for size in 120x30 80x24; do
		local d=$out/$size
		BOOT=3 app "${size%x*}" "${size#*x}" /home/demo/repo lazygit
		snap "$d/welcome.txt" lazygit normal "first-run popup"
		keys Enter; settle
		snap "$d/files.txt" lazygit normal "files panel"
		keys /; settle
		snap "$d/filter-empty.txt" lazygit insert "filter prompt"
		keys -l ab; settle
		snap "$d/filter-typed.txt" lazygit insert "typing a filter"
		keys Enter; settle
		snap "$d/filter-applied.txt" lazygit normal "filter applied"
		keys Escape; settle "$ESC_SETTLE"
		keys 2; settle
		keys Space; settle
		keys c; settle
		snap "$d/commit-summary.txt" lazygit insert "commit message summary"
		keys -l 'Summary'; settle
		keys Tab; settle
		snap "$d/commit-description.txt" lazygit insert "commit description"
		keys Escape; settle "$ESC_SETTLE"
		keys '?'; settle
		snap "$d/keybindings-menu.txt" lazygit normal "keybindings menu"
		keys Escape; settle "$ESC_SETTLE"
		keys :; settle
		snap "$d/shell-command.txt" lazygit insert "shell command prompt"
		keys Escape; settle "$ESC_SETTLE"
		keys 4; settle
		keys d; settle
		snap "$d/confirm.txt" lazygit normal "confirmation popup: no text field"
		keys Escape; settle "$ESC_SETTLE"
		keys 3; settle
		keys n; settle
		snap "$d/new-branch.txt" lazygit insert "new branch name prompt"
		keys Escape; settle "$ESC_SETTLE"
		stop
	done
}

cap_ranger() {
	local out
	out=$ROOT/tests/fixtures/ranger/$(docker run --rm "$IMG" ranger --version | awk 'NR==1{print $NF}')
	for size in 100x30 80x24; do
		local d=$out/$size
		BOOT=3 app "${size%x*}" "${size#*x}" /home/demo/tree ranger
		snap "$d/main.txt" ranger normal "browsing"
		keys :; settle
		snap "$d/console-empty.txt" ranger command "console ':'" "${ANCHORLESS[@]}"
		keys -l 'abc'; settle
		snap "$d/console-typed.txt" ranger command "typing in the console" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys /; settle
		snap "$d/search.txt" ranger command "search: ':search '" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys a; settle
		snap "$d/rename.txt" ranger command "rename: ':rename name'" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys '?'; settle
		snap "$d/help-question.txt" ranger unknown "one-key question over the status bar: identity not confirmed"
		keys q; settle
		stop
	done
}

cap_lf() {
	local out
	out=$ROOT/tests/fixtures/lf/r$(version 'lf -version' 1 | sed 's/+.*//')
	for size in 100x30 80x24; do
		local d=$out/$size
		BOOT=3 app "${size%x*}" "${size#*x}" /home/demo/tree lf
		snap "$d/main.txt" lf normal "browsing"
		keys :; settle
		snap "$d/command-empty.txt" lf command "command line ':'" "${ANCHORLESS[@]}"
		keys -l 'abc'; settle
		snap "$d/command-typed.txt" lf command "typing a command" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys /; settle
		snap "$d/search.txt" lf insert "search '/'" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys r; settle
		snap "$d/rename.txt" lf insert "rename: prompt" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys '!'; settle
		snap "$d/shell.txt" lf insert "shell command '!'" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys f; settle
		snap "$d/find.txt" lf insert "find: one-key prompt" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		stop
	done
}

cap_nnn() {
	local out
	out=$ROOT/tests/fixtures/nnn/$(version 'nnn -V' 1)
	for size in 100x30 80x24; do
		local d=$out/$size
		BOOT=3 app "${size%x*}" "${size#*x}" /home/demo/tree nnn
		snap "$d/main.txt" nnn normal "browsing"
		keys /; settle
		snap "$d/filter-empty.txt" nnn insert "filter prompt '/'" "${ANCHORLESS[@]}"
		keys -l s; settle
		snap "$d/filter-typed.txt" nnn insert "typing a filter" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys C-r; settle
		snap "$d/rename.txt" nnn insert "rename prompt" "${ANCHORLESS[@]}"
		keys Escape; settle "$ESC_SETTLE"
		keys n; settle
		snap "$d/new-question.txt" nnn unknown "new: one-key question over the status bar; not text entry"
		keys Escape; settle "$ESC_SETTLE"
		stop
	done
}

cap_mc() {
	local out
	out=$ROOT/tests/fixtures/mc/$(version 'mc -V' 4)
	for size in 100x30 80x24; do
		local d=$out/$size
		BOOT=3 app "${size%x*}" "${size#*x}" /home/demo mc
		snap "$d/panels.txt" mc insert "panels: letters go to the command line"
		keys F9; settle
		snap "$d/menu.txt" mc insert "pull-down menu"
		keys Escape; settle "$ESC_SETTLE"
		keys F7; settle
		snap "$d/mkdir.txt" mc insert "mkdir dialog"
		keys Escape; settle "$ESC_SETTLE"
		stop
	done
}

cap_fzf() {
	local out
	out=$ROOT/tests/fixtures/fzf/$(version 'fzf --version' 1)
	for size in 100x30 80x24; do
		local d=$out/$size
		app "${size%x*}" "${size#*x}" /home/demo bash -c 'seq 1000 | fzf'
		snap "$d/empty.txt" fzf insert "empty query"
		keys -l 12; settle
		snap "$d/typed.txt" fzf insert "typing a query"
		stop
		# --reverse (as fzf-tmux users often pass it): the query on top.
		app "${size%x*}" "${size#*x}" /home/demo bash -c 'seq 1000 | fzf --reverse'
		snap "$d/reverse-empty.txt" fzf insert "--reverse, empty query"
		keys -l 12; settle
		snap "$d/reverse-typed.txt" fzf insert "--reverse, typing a query"
		stop
	done
}

# vim and neovim (hooks-only, D30). EDITOR_BIN picks the binary (nvim for
# the distro's, nvim-upstream for the release build in ubuntu-24.04).
NOTES='printf "alpha\nalpine\nalps\nbeta\n" >notes.txt'
cap_editor() {
	local name=$1 bin=$2 ver=$3 out
	out=$ROOT/tests/fixtures/$name/$ver
	# vim's command line replaces the ruler: only tildes are left, so a
	# pane not yet identified can't be. nvim keeps its status line.
	local cmdline=()
	[ "$name" = vim ] && cmdline=("${ANCHORLESS[@]}")
	# vim with two windows has nvim's layout (the ruler in a status line
	# on row -2); over SSH only that tells them apart.
	local layout=()
	[ "$name" = vim ] && layout=(expect_app_remote=nvim expect_app_mono=nvim)
	# nvim's prompts scroll its status line away: vim's layout.
	local prompt=()
	[ "$name" = nvim ] && prompt=(expect_app_remote=vim expect_app_mono=vim)
	for size in 120x40 80x24; do
		local d=$out/$size
		app "${size%x*}" "${size#*x}" /home/demo bash -c "$NOTES; $bin notes.txt"
		snap "$d/open.txt" "$name" normal "just opened: file message, ruler, tildes"
		keys j; settle
		snap "$d/normal.txt" "$name" normal "after moving"
		keys i; settle
		snap "$d/insert.txt" "$name" insert "-- INSERT --"
		keys End Enter; keys -l al; keys C-n; settle
		snap "$d/insert-completion.txt" "$name" insert "completion popup, the mode line shows the completion submode"
		keys Escape; settle
		keys v; settle
		snap "$d/visual.txt" "$name" visual "-- VISUAL --"
		keys Escape V; settle
		snap "$d/visual-line.txt" "$name" visual "-- VISUAL LINE --"
		keys Escape C-v; settle
		snap "$d/visual-block.txt" "$name" visual "-- VISUAL BLOCK --"
		keys Escape; settle
		keys R; settle
		snap "$d/replace.txt" "$name" replace "-- REPLACE --"
		keys Escape; settle
		keys g h; settle
		snap "$d/select.txt" "$name" select "-- SELECT --: a letter replaces the selection"
		keys Escape; settle
		keys i C-o; settle
		snap "$d/insert-pending.txt" "$name" normal "-- (insert) --: one normal command from insert"
		keys Escape Escape; settle
		keys :; settle
		snap "$d/command-empty.txt" "$name" command "command line just opened" "${cmdline[@]}"
		keys -l 'set nu'; settle
		snap "$d/command-typed.txt" "$name" command "typing a command" "${cmdline[@]}"
		keys Escape; settle
		keys /; keys -l alp; settle
		snap "$d/search-typed.txt" "$name" command "typing a search" "${cmdline[@]}"
		keys Enter; settle
		keys -l ':w'; keys Enter; settle
		snap "$d/written.txt" "$name" normal "written message"
		keys -l '/zzzz'; keys Enter; settle
		snap "$d/not-found.txt" "$name" normal "E486: Pattern not found"
		keys q q; settle
		snap "$d/recording.txt" "$name" normal "recording @q"
		keys q; settle
		keys -l ':ls'; keys Enter; settle
		snap "$d/hit-enter.txt" "$name" normal "hit-enter prompt" "${prompt[@]}"
		keys Enter; settle
		keys -l ':set all'; keys Enter; settle
		snap "$d/more.txt" "$name" normal "-- More -- prompt" "${prompt[@]}"
		keys q; settle
		keys -l ':set cmdheight=2'; keys Enter; settle
		keys i; settle
		snap "$d/cmdheight2-insert.txt" "$name" insert "cmdheight=2: the marker stays on the last row"
		keys Escape :; settle
		snap "$d/cmdheight2-command.txt" "$name" command "cmdheight=2: the command line opens on row -2" "${cmdline[@]}"
		keys Escape; keys -l ':set cmdheight=1'; keys Enter; settle
		keys -l ':set noshowmode'; keys Enter; keys i; settle
		snap "$d/noshowmode-insert.txt" "$name" normal \
			"KNOWN GAP: really insert; with showmode off nothing on screen says so"
		keys Escape; keys -l ':set showmode'; keys Enter; settle
		keys -l ':split'; keys Enter; settle
		snap "$d/split.txt" "$name" normal "two windows: the ruler is in the status lines" "${layout[@]}"
		keys i; settle
		snap "$d/split-insert.txt" "$name" insert "two windows, insert" "${layout[@]}"
		keys Escape; keys -l ':only'; keys Enter; settle
		keys -l ':e /home/demo/sample.txt'; keys Enter; settle
		keys 5 0 G; settle
		snap "$d/full-buffer.txt" "$name" normal "the buffer fills the window: no tildes, the ruler only"
		keys -l ':term'; keys Enter; settle 1
		if [ "$name" = nvim ]; then
			keys i; settle
			snap "$d/terminal.txt" "$name" terminal "-- TERMINAL --"
		else
			snap "$d/terminal.txt" "$name" normal \
				"KNOWN GAP: vim's terminal window takes typing but shows no marker" "${layout[@]}"
		fi
		stop
	done
	# Translated markers (the full list is checked by TestVimMarkers).
	for lang in de_DE ru_RU ja_JP; do
		local d=$out/80x24/$lang
		app 80 24 /home/demo bash -c "$NOTES; LANG=$lang.UTF-8 $bin notes.txt"
		keys j; settle
		snap "$d/normal.txt" "$name" normal "LANG=$lang.UTF-8"
		keys i; settle
		snap "$d/insert.txt" "$name" insert "LANG=$lang.UTF-8"
		keys Escape V; settle
		snap "$d/visual-line.txt" "$name" visual "LANG=$lang.UTF-8"
		keys Escape; keys -l ':ls'; keys Enter; settle
		snap "$d/hit-enter.txt" "$name" normal "LANG=$lang.UTF-8" "${prompt[@]}"
		stop
	done
}

cap_vim() { cap_editor vim vim "$(version 'vim --version' 5)"; }
cap_nvim() {
	cap_editor nvim nvim "$(version 'nvim --version' 2 | sed 's/^v//')"
	local up
	up=$(version 'nvim-upstream --version 2>/dev/null' 2 | sed 's/^v//')
	if [ -n "$up" ]; then
		cap_editor nvim nvim-upstream "$up"
	fi
}

# debconf's questions through its whiptail frontend: the probe questions
# (tests/docker/debconf-probe.sh), ucf's real modified-conffile prompt and
# dpkg-reconfigure tzdata. The cursor shows only in a text entry.
cap_debconf() {
	local out
	out=$ROOT/tests/fixtures/debconf/$(version "dpkg-query -W -f='\${Version}\n' debconf" 1)
	for size in 100x30 80x24; do
		local d=$out/$size
		app "${size%x*}" "${size#*x}" /home/demo sudo debconf-probe
		snap "$d/restart.txt" debconf normal "needrestart-style checklist with a title"
		keys Down; settle
		snap "$d/restart-moved.txt" debconf normal "checklist, second item focused"
		keys Tab; settle
		snap "$d/restart-buttons.txt" debconf normal "checklist, focus on <Ok>"
		keys Enter; settle
		snap "$d/kernel.txt" debconf normal "note: pending kernel upgrade"
		keys Enter; settle
		snap "$d/area.txt" debconf normal "select question: a menu (letters jump to items)"
		keys End; settle
		snap "$d/area-end.txt" debconf normal "menu scrolled to the end"
		keys Enter; settle
		snap "$d/mailname.txt" debconf insert "string question: the cursor is in the text entry"
		keys End; keys -l .net; settle
		snap "$d/mailname-typed.txt" debconf insert "typing in the text entry"
		keys Tab; settle
		snap "$d/mailname-buttons.txt" debconf normal "string question, focus on <Ok>: the cursor is hidden"
		keys Enter; settle
		snap "$d/password.txt" debconf insert "password question"
		keys -l secret; settle
		snap "$d/password-typed.txt" debconf insert "typing a password"
		keys Enter; settle
		snap "$d/save.txt" debconf normal "boolean question: <Yes> <No>"
		keys Enter; settle
		stop
		app "${size%x*}" "${size#*x}" /home/demo sudo ucf-probe
		snap "$d/ucf.txt" debconf normal "ucf: modified configuration file"
		keys Down Down Enter; settle
		snap "$d/ucf-diff.txt" debconf normal "ucf: the differences, as a note"
		keys Enter; settle
		keys Enter; settle
		stop
		BOOT=3 app "${size%x*}" "${size#*x}" /home/demo sudo dpkg-reconfigure tzdata
		snap "$d/tzdata-area.txt" debconf normal "dpkg-reconfigure tzdata: geographic area"
		keys Enter; settle
		snap "$d/tzdata-city.txt" debconf normal "dpkg-reconfigure tzdata: a long scrolling menu"
		stop
	done
	# No UTF-8 locale: the frame in VT100 line-drawing letters.
	local d=$out/80x24/C
	app 80 24 /home/demo sudo env LANG=C debconf-probe restart mailname
	snap "$d/restart.txt" debconf normal "LANG=C: frame in VT100 letters"
	keys Enter; settle
	snap "$d/mailname.txt" debconf insert "LANG=C: text entry"
	stop
	# The backtitle is translated (the full list is checked by TestDebconfTitles).
	for lang in de_DE ru_RU ja_JP; do
		d=$out/80x24/$lang
		app 80 24 /home/demo sudo env LANG=$lang.UTF-8 ucf-probe
		snap "$d/ucf.txt" debconf normal "LANG=$lang.UTF-8"
		stop
		app 80 24 /home/demo sudo env LANG=$lang.UTF-8 debconf-probe mailname save
		snap "$d/mailname.txt" debconf insert "LANG=$lang.UTF-8"
		keys Enter; settle
		snap "$d/save.txt" debconf normal "LANG=$lang.UTF-8"
		stop
	done
}

# whiptail from scripts, the way raspi-config and installers use it.
WT_MENU=(whiptail --title "Software Configuration Tool (raspi-config)" --menu "Setup Options" 20 70 8
	--cancel-button Finish --ok-button Select
	"1 System Options" "Configure system settings"
	"2 Display Options" "Configure display settings"
	"3 Interface Options" "Configure connections to peripherals"
	"4 Performance Options" "Configure performance settings"
	"5 Localisation Options" "Configure language and regional settings"
	"8 Update" "Update this tool to the latest version")
cap_whiptail() {
	local out
	out=$ROOT/tests/fixtures/whiptail/$(version "dpkg-query -W -f='\${Version}\n' whiptail" 1 | cut -d- -f1)
	for size in 100x30 80x24; do
		local d=$out/$size w=${size%x*} h=${size#*x}
		app "$w" "$h" /home/demo "${WT_MENU[@]}"
		snap "$d/menu.txt" whiptail normal "raspi-config style menu, <Select> <Finish>"
		keys Down Down; settle
		snap "$d/menu-moved.txt" whiptail normal "menu, third item"
		keys Tab Tab; settle
		snap "$d/menu-buttons.txt" whiptail normal "menu, focus on <Finish>"
		stop
		app "$w" "$h" /home/demo whiptail --title "Pick features" --checklist "Choose:" 15 60 4 \
			ssh "OpenSSH server" ON web "Web server" OFF db "Database" OFF
		snap "$d/checklist.txt" whiptail normal "checklist"
		stop
		app "$w" "$h" /home/demo whiptail --radiolist "Interface:" 12 50 3 \
			eth0 "Wired" ON wlan0 "Wireless" OFF
		snap "$d/radiolist.txt" whiptail normal "radiolist, no title"
		stop
		app "$w" "$h" /home/demo whiptail --title "Reboot" --yesno "Would you like to reboot now?" 8 50
		snap "$d/yesno.txt" whiptail normal "yes/no"
		stop
		app "$w" "$h" /home/demo whiptail --msgbox "The setup is complete." 8 40
		snap "$d/msgbox.txt" whiptail normal "message box, <Ok> only"
		stop
		app "$w" "$h" /home/demo whiptail --scrolltext --textbox /home/demo/sample.txt 20 60
		snap "$d/textbox.txt" whiptail normal "text box with a scroll bar"
		stop
		app "$w" "$h" /home/demo whiptail --title "Hostname" --inputbox "Please enter a hostname" 10 60 raspberrypi
		snap "$d/inputbox.txt" whiptail insert "input box: the cursor is in the entry"
		keys -l .lan; settle
		snap "$d/inputbox-typed.txt" whiptail insert "typing in the input box"
		keys Tab; settle
		snap "$d/inputbox-buttons.txt" whiptail normal "input box, focus on <Ok>"
		stop
		app "$w" "$h" /home/demo whiptail --passwordbox "Enter a new password" 8 50
		snap "$d/passwordbox.txt" whiptail insert "password box"
		stop
		app "$w" "$h" /home/demo bash -c 'sleep 600 | whiptail --gauge "Installing ..." 6 50 40'
		snap "$d/gauge.txt" "" unknown "progress gauge: no buttons, not recognised"
		stop
	done
	local d=$out/80x24/C
	app 80 24 /home/demo env LANG=C "${WT_MENU[@]}"
	snap "$d/menu.txt" whiptail normal "LANG=C: frame in VT100 letters"
	stop
	app 80 24 /home/demo env LANG=C whiptail --inputbox "Please enter a hostname" 10 60
	snap "$d/inputbox.txt" whiptail insert "LANG=C: input box"
	stop
}

for a in "${apps[@]}"; do
	"cap_$a"
done
