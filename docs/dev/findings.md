# Verified findings

Empirical facts established while building, with how each was verified.
Where a finding contradicts or refines the brief, the brief section is
noted. Environment: WSL2 Ubuntu 24.04, tmux 3.4 locally; other tmux
versions built in Docker (`tests/docker/tmux-src.Dockerfile`,
`--build-arg TMUX_VERSION=...`).

## tmux behaviour

**F1. Batched tier-1 reads work.** `list-panes -a -F <fmt>` evaluates the
format per pane in one command (answers §13 "batched display-message").
Each pane of a linked window is listed once per session.

**F2. Unattached control clients don't persist.** `tmux -C <cmd>` runs the
command and exits; a persistent control connection must `attach-session`.
With `-f ignore-size` it does not change window sizes. It receives
`%output` for every pane in its session only.

**F3. Control-mode block framing.** Replies are `%begin T N F` … `%end T N F`
(or `%error`). `F=0` for blocks not caused by this client (the attach
itself); `F=1` for ours. Pane text inside a block can look like `%end`, so a
block ends only on a terminator with the same `T N`.
Notifications never appear inside a block.

**F4. `unbind-key -a -q -T <missing table>` returns an error in 3.4** (empty
message) despite `-q`. When several commands were joined with ` ; ` on one
control line, that error dropped the whole line — this is why commands are
now sent one per line (still pipelined).

**F5. Setting `key-table` does not move clients** — see decisions D3. A client
in a no-longer-default table discards the first unbound key.

**F6. Copy mode bypasses the session key table.** When the client is in its
default table and the pane is in a mode, tmux uses the mode's table
(`copy-mode-vi`), so modal tables never interfere with copy mode.

**F7. `display-message -p` escapes non-printable characters** (`\x1f` comes
back as the text `\037`). Tabs pass through. Use tab separators.

**F8. Option values print `$` as `\$`** — in `show-options -v` and in
`#{@option}` alike, however the value was set (config file or argv). Only
`$` is escaped; a stored `\$` prints as `\\$`. So unescaping `\$` → `$`
is exact (`unescapeOption`). Matters for `@modal_transition_hook`.
Setting via shell argv also drops a trailing ` ;` (tmux reparses argv).

**F9. `pane-border-format` redraws by itself** when a pane option changes;
the status line does not (needs `refresh-client -S`).

**F10. `escape-time` dominates Esc latency.** Default 500 ms: every Esc is
held that long by tmux before reaching the app. With the default, "leave
insert" measured ~590 ms; with `escape-time 5`, ~90 ms. The integration
harness sets it to 5. The owner already has 0.

**F11. `cursor_shape` availability** (§3.3, §13 — highest-value unknown):
absent in tmux 3.4 and 3.5a (empty; also no `cursor_very_visible` /
`cursor_colour`), present in **3.7c** (latest release at the time; 3.8-rc2
exists). On 3.7c: nvim reports `block`/`bar`/`underline` as the brief
predicts; stock vim reports `default` in every mode (confirmed). **It passes
through a nested tmux**, but the switch back to `block` after Esc took
~510 ms nested versus 4 ms (nvim `ttimeoutlen=0`) / 59 ms (default) direct,
even with `escape-time 0` on both servers. Cause not identified. Conclusion:
over nested tmux it is a slow corroborator only.

**F12. Run-shell / environment.** `run-shell` expands formats in its command,
so `modal.tmux` passes `--socket '#{socket_path}'` explicitly.
`list-keys` output is re-executable tmux syntax (keeps padding; `\;` stays).
`if-shell -F cond { a } { b }` brace syntax works over control mode.

## htop (fixtures: `tests/fixtures/htop/{2.2.0,3.0.5,3.2.2,3.3.0}/{200x50,80x24}`)

**F13. Prompts are not at column 0** (contradicts the §5.2/§6.1 example
`^Search:` `col=0`). The last row's function bar is replaced by a prompt bar:
- 3.x: `F3Next  S-F3Prev   EscCancel    Search: abc`
- 2.2: `F3Next  EscCancel    Search: abc` (no `S-F3Prev`)
- all: `EnterDone  EscClear    Filter: xy`
It is always the last row, including with header meters hidden (`#`) and at
80×24.

**F14. `cursor_flag` is 1 exactly while a text prompt is open**, in every
version, and 0 in normal mode and in the F9/u/F2 panels and help. Only 3.2+
parks the cursor on the prompt row; 2.2 and 3.0 leave it at 0,0.

**F15. Panels.** F9 (signal list: `EnterSend  EscCancel`), `u` (user list:
`EnterShow  EscCancel`), F2 (setup: bar ends `F10Done`) are navigable →
normal. Help (`h`) has no anchors → `unknown` (pass-through), which is fine:
any key leaves it. After Filter+Enter the filter stays active (`F4FILTER`
in the bar) and htop is back in normal.

**F16. htop key bindings (3.3 help):** `h` help, `k` kill, `l` lsof are
shadowed by hjkl; `j`, `g`, `G`, `C-d`, `C-u` and `_` are unbound, so `_`
is a safe leader for htop. Digits start an incremental PID search (no prompt
drawn; not handled — digits are not remapped, so harmless).

**F17. htop colours** (default theme, all versions): keycap labels on the
bottom bar are fg black / bg cyan (e.g. cell (-1,2)); ` Search:` label too.
Same styling in normal and prompt bars → useful for identity only, as bonus.

## Spec-authoring pitfalls found

**F18. Lines are right-trimmed**, so a prompt regex ending in a space
(`'^PROMPT> '`, `'Search: '`) can never match an empty prompt. Use
`( |$)`. `validate` now lints this.

**F19. The brief's §5.2.1 weights are inconsistent:** 70/50/40 with
threshold 100, but "any two of those three clear the threshold" is false
(50+40=90). Use e.g. 70/60/50 when writing the real vim spec.

**F20. TOML placement:** a top-level key appended after a `[[table]]` header
belongs to that table (bit an integration test).

## SSH and nested tmux (M5; sshd container `tests/docker/sshd.Dockerfile`, tmux 3.4 both sides)

**F21. The brief's tier-1 nesting signals (§3.8) do not discriminate.**
Measured per 0.5 s: plain `ssh -t host htop` also shows `alternate_on=1`
from the first read with zero toggles (the command becomes `ssh` and htop
enters the alt screen between two reads), and `history_size` frozen.
`scroll_region_lower` stayed `pane_height-1` under a remote tmux even after
it scrolled shell output (`seq 100`) and ran htop. `pane_title` is the
local host name unless the inner tmux has `set-titles on`. Nesting is
therefore detected from the screen (status line, borders), the local
command (`tmux`) and the title shape only.

**F22. Under a remote tmux, htop's top anchors still match** (PID header,
meters), while the inner status line covers the last row. With the bottom
bar as optional evidence the identity was confirmed and the missing
`Search:` prompt (now one row up) read as **normal → remap while typing**.
Fixed twice over: the bottom bar is `required` in `specs/htop.toml`, and
remapping is disabled for nested panes (D20). A lint now flags the pattern
(D21). Fixtures: `tests/fixtures/nested/`.

**F23. Inner pane borders come in two alphabets.** With a UTF-8 locale on
the remote the inner tmux draws `│ ─ ├ …`; without one (no `LANG` in a
bare container) it uses the VT100 line-drawing charset, and the outer
grid stores — and `capture-pane` returns — the plain letters `x q t u w v n`.

**F24. The inner tmux's command prompt opens asynchronously.** Sending
`C-b : set … Enter` in one `send-keys` delivers the text to the
application (htop then started `strace` on `s`). Wait ~0.3 s after `C-b :`
(`scripts/fixtures/nested.sh`).

## Test-harness pitfalls

**F25. bash `read -t` loses bytes.** The bash test app polled with
`read -rsn1 -t 0.1` (so its SIGWINCH trap could redraw). Roughly one run in
four lost a key: `tmux -vv` showed `writing key 0x2f (/) to %0`, the app
never saw it. Blocking `read` defers the trap (no redraw on resize), and
`set -o posix` did not make SIGWINCH interrupt `read` in bash 5.2. The app
is now a small Go program (`tests/integration/keyecho/`). A
self-inflicted variant: typing a key less than 20 ms after Escape merges
the two into `ESC/` in any app that disambiguates Escape by timeout — wait
for the *app* (screen), not only for the daemon's published state, before
the next key.

## Performance (WSL2, tmux 3.4)

**CPU benchmark (M6, `TestBenchmarkCPU`)**, htop in 5 / 20 tiled panes,
30 s windows, % of one core, daemon / extra tmux server CPU. Before output
gating (D28), scope `active`: 5 panes 0.30 / +0.47 (balanced), 20 panes
0.67 / +1.60 (balanced), 1.33 / +4.27 (snappy) — the server's share grew
with every background pane. After: 5 panes 0.13 / +0.17, 20 panes
0.17 / +0.33 (balanced), 0.27 / +0.50 (snappy). Scope `all`, 20 panes:
2.20 / +5.59 → 1.63 / +4.16 (balanced; every pane is examined on each
redraw, full-screen captures because htop's identity reads the top rows).
Full table in the README.

**One pane:**

Keypress → published mode (`TestDetectionLatency`): into typing 37 ms
(balanced) / 28 ms (snappy); back to commanding 94 / 76 ms (includes the
2-capture confirmation and ~20 ms of the test app's own Esc handling).
Daemon CPU ~0 % idle, ~1.8 % of a core while toggling modes every ~150 ms.
`powershell.exe` from WSL takes ~0.47 s just to start (hence the detached
toast hook).

## Applications (M6, probed in containers)

**F26. Most "navigation TUIs" already have vim keys.** less (j/k/g/G,
C-d/C-u; h = help), tig (j/k; h = help), lazygit (hjkl), k9s (j/k, g/G),
ranger, lf, nnn (hjkl), ncdu (hjkl). htop and btop do not (btop has an
opt-in `vim_keys`). Hence D23.

**F27. Aligned text looks like VT100-letter borders.** less showing
"Sample text line N" puts `t` and `x` in the same columns on every row;
with the file name `sample.txt` on the last row, column 9 was all
border letters and the pane read as a split nested tmux (N/A). mc's
full-width panel separator `├───┤├───┤` did the same with UTF-8 glyphs.
Fixed by the edge and junction rules in D27.

**F28. Escape does not close less or tig prompts.** less treats Esc as
the start of a line-editing sequence; tig's prompts are readline (Esc is
a meta prefix). Enter runs the prompt, C-c (or backspacing past its start
in less) cancels it. k9s, lazygit, ranger, lf, nnn close on Esc.

**F29. ncurses apps wait `ESCDELAY` (1 s) after Esc**, so scripted
captures must settle ~1.3 s after Escape (tig, ranger, lf, nnn, ncdu, mc).
tcell apps (k9s, lazygit) read Esc followed at once by a key as Alt+key.

**F30. Cursor visibility is a near-perfect prompt signal** for tig,
ranger, lf, nnn (cursor on the last row only while a prompt is open) and
lazygit (cursor visible exactly while a text field has focus, anywhere on
screen; menus and confirmations keep it hidden). less keeps it visible on
the last row always; btop and k9s hide it always (btop draws its own `█`).

**F31. btop specifics.** The filter is edited in the proc box title:
`⁴proc┌┐f bto█ ↵┌` (`↲` in 1.2.3), `f bto del` once applied. btop drops
keys that arrive in one write (`send-keys -l abc` loses all but one; a
human typing is fine). Overlays (options, help in 1.3) cover the proc box,
so identity is not confirmed there; the options tab row is a positive
marker. btop shows the host's CPU model, which the fixture script
replaces (including tails left visible by overlays).

**F32. less under git runs on the primary screen** (`LESS=FRX`, pane
command `git`); `man` reports pane command `man` with the alternate
screen on. less's idle prompt is `:` (not a command line); `& :` while a
filter is on; messages end in `(press RETURN)`; prompts are right-trimmed,
so `Examine: ` must be matched as `Examine:( |$)`.

**F33. Minimised Ubuntu images divert `man`** to a stub and exclude man
pages; `apps-install.sh` removes the exclusion and the diversion.

**F34. `refresh-client -A '%N:off'` is safe and needs quoting.** From a
control client it stops that client's `%output` stream for the pane; tmux
keeps reading the pane for human clients (the pane went on updating while
visible and while in a hidden window; the manual's "stops reading when all
clients have turned it off" did not apply with a human attached). Unquoted,
`refresh-client -A %0:off` is a parse error (a word starting with `%` other
than a bare pane id), so `tmux.Quote` quotes such words.

## vim and neovim (M7; vim 8.2/9.0/9.1, nvim 0.6.1/0.7.2/0.9.5 from the distros, nvim 0.12.5 release build)

**F35. Screen layout.** The showmode marker is always on the **last row**,
column 0, whatever the layout (cmdheight 1 or 2, status line or not); in
vim without a status line row -2 is buffer text (a Lua/SQL `-- ` comment
can sit there). The ruler (`12,5-8   All`) starts exactly **18 columns from
the right edge**: on the last row in vim (laststatus=1), in the status line
on row -2 in nvim (laststatus=2 by default) and in split windows. A `:`
command line replaces the ruler on the last row (vim then shows only
tildes), or with cmdheight=2 opens on row -2 with the last row cleared.
Completion replaces `-- INSERT --` with `-- Keyword completion (^N^P)
match 1 of 3` (and no ruler). `C-o` in insert shows `-- (insert) --`,
visual from it `-- (insert) VISUAL --`. The hit-enter and `-- More --`
prompts take the last row with the cursor on it. nvim's terminal mode
shows `-- TERMINAL --`; plain vim's terminal window shows nothing.
`recording @q` follows the marker (or stands alone in normal mode).

**F36. Translations.** vim ships 30 catalogues, the distros' neovim 27;
**neovim's release tarballs ship none** (always English). Translated:
every showmode word, the ruler's position word (`Alles`, `Весь текст`,
`全て`), the hit-enter and More prompts. Traditional Chinese uses one word
(選取) for both VISUAL and SELECT. The Hungarian catalogue is mis-declared
(UTF-8 bytes labelled ISO-8859-1), so vim shows mojibake, and the markers
list keeps what is displayed. `scripts/fixtures/vim-markers.py` extracts
all of it from the images into `tests/fixtures/vim/markers.json`.

**F37. No distro vim sets `t_SI`/`t_EI` by default** (vim 8.2, 9.0, 9.1
with defaults.vim, TERM tmux-256color and xterm-256color): stock vim never
changes the cursor shape, as the brief expected, so cursor shape helps
neovim only, and only on a tmux that reports it (F11).

**F38. Hook latency, keypress → transition hook → a warm FIFO listener**
(`TestVimHookLatency`, vim with defaults, WSL2): entering insert p50/p90
39/55 ms local, 62/70 ms over SSH (balanced); 31/45 and 40/76 ms (snappy).
Leaving insert is ~130 ms more, almost all of it vim's own Escape timeout
(`ttimeoutlen=100` in defaults.vim). Until M7 the hook debounce was
trailing and added its full 30 ms to every call (D31).
