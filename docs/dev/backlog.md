# Backlog

Ideas and known gaps that are **not** scheduled in a milestone yet. Each
entry says why it exists, what was already decided, and how to build it.
Promote an entry into [milestones.md](milestones.md) when it gets
scheduled; delete it when done (record the outcome in decisions/findings).

## B1. Split remote tmux — done (cursor D32 in 1.0, border colour D34 in 1.2)

Left, by the owner's call (default configs only): a remote tmux with a
themed border colour falls back to the cursor alone (htop/btop N/A
there).

## B2. Nested `cursor_shape` delay (findings F11)

Through a nested tmux (3.7c), nvim's cursor shape reached the outer tmux
~0.5 s late on Esc, even with `escape-time 0` on both servers. Cause
unknown. Only matters for nvim on tmux ≥ 3.6; the owner runs 3.4 where
`cursor_shape` does not exist at all. Not looked at in M7 (the vim specs
do not depend on it: markers are primary, shape only vetoes). Related but
different: the ~600 ms Escape through a remote tmux in M7's nested vim
tests is the inner tmux's `escape-time` (500 ms default on 3.4) plus vim's
`ttimeoutlen`, fully explained.

## B3. Inner prefix key while remapping through nested tmux

Accepted by the owner: after the inner tmux's prefix key, a remapped key
reaches the inner tmux remapped (`prefix l` → `prefix Right`; tmux's
default prefix table binds only `l` of hjkl). Workaround documented
(escape leader). A fix would need to know the inner prefix — not visible
from outside. Only revisit if the owner complains.

## B4. Scheduler leftovers

- `@modal_burst_decay` is parsed but unused (event-driven scheduling made
  it moot; kept for a possible polling fallback).
- (Done in M6: spec `poll_interval` is used by the scheduler.)
- (Done in M6: unwatched panes' output is switched off for the daemon, D28.)

## B5. Small things

- (Done in 1.0: CI actions bumped to checkout@v7 / setup-go@v7, Node 24.)
- htop: typing digits starts an incremental PID search with no prompt drawn
  (findings F16). Digits are not remapped, so harmless; nothing to do
  unless a spec ever remaps digits.
- A customised inner status line (not tmux's default `[session] 0:win*`
  shape) is not recognised as nesting. Harmless with htop now (its bottom
  bar is required, so a foreign last row makes it N/A) and with the default
  nested-remap policy; would matter only for `@modal_nested_remap off` or
  for B1.
- Test specs `keyecho`/`keyhook` trip the "under-specified" lint (7-char
  `PROMPT>` marker). Cosmetic.

## B6. Spec ideas left from M6

- less over SSH: recognise it from its first screen. The file name prompt
  in reverse video plus the cursor after it is suggestive but not
  specific; would need a second anchor.
- k9s with its header hidden (`ctrl-e`): identify from the crumbs row and
  table frame.
- fzf `--reverse` / `--layout=reverse-list`: the counter and query are on
  the top rows; only the default layout is recognised over SSH.
- REPLs over SSH: a cursor-row-relative clause (`row = "cursor"`) would let
  a spec say "the prompt `>>> ` is on the cursor's row", which is specific
  enough to identify Python remotely.
- `mc`: a real normal mode would need to tell the panels from a focused
  dialog input; ship only with fixtures proving it.

## B7. Left over from M7 (vim, Windows side)

- **Warm Windows bridge for the AHK endpoint.** `ahk-http.sh` starts
  Windows `curl.exe` per transition (~80 ms, off the hook's critical path
  since it is detached, but still that much later at the AHK side). A
  long-lived Windows process (PowerShell or a tiny exe) started once from
  WSL, fed lines through its stdin from the FIFO listener
  (`examples/listener.sh`), would send the HTTP request in ~3 ms and keep
  order. Only if the owner finds 80–130 ms too slow for their layer.
  Alternative: WSL mirrored networking (`networkingMode=mirrored` in
  .wslconfig) would let Linux `curl` reach Windows' localhost directly —
  the owner's choice, it changes their WSL setup.
- **Real tmux ≥ 3.6 run of the cursor-shape veto** with nvim (the image
  `tmux-modal-tmuxsrc:3.7c` from F11 can host it). Today only a unit test
  sets the shape on a real capture.
- **vim's ~100 ms `unknown` on Escape** (it clears the mode line at once
  and redraws the ruler after `ttimeoutlen`): subscribers see typing a
  little longer. Could be smoothed by treating "sticky vim, last row
  blank, tildes present" as normal — declined for now: it weakens the
  "never normal from a half-drawn screen" rule (§9) for a cosmetic gain.
- **Configured vims are out of scope by the owner's call** (lualine,
  airline, noshowmode, no ruler): don't add fingerprints for them unless
  the owner asks; they would rather add an explicit hint (a `titlestring`
  carrying the mode) to their own config. A `title`-based identity/mode
  rule is already supported by the spec engine (`match.title`) if that
  day comes — it would need mode rules on the title too (not supported
  yet: mode rules read the screen only).
- vim/nvim **name over SSH** is a layout guess (D30). Harmless for modes;
  if a subscriber ever needs the right name remotely, there is no screen
  signal that separates them reliably.
- `fr_FR` is generated in the images but no French fixtures were
  captured (the full catalogue is covered by `TestVimMarkers`); upstream
  nvim (`nvim-upstream`) exists only in the ubuntu-24.04 image.
- `emacs -nw` (listed as experimental in the M7 plan) was not started.

## B8. Left over from the debconf/whiptail specs (1.1.0, D33)

- `dialog(1)`-based tools (some installers, `menuconfig`'s lxdialog) and
  `nmtui` (newt, but its own layout) are not recognised; add specs only
  when the owner meets one. Probe first as for whiptail: does the cursor
  still mark text entries, which keys do letters trigger?
- `whiptail --gauge` has no buttons, so it is N/A (it takes no keys).
- debconf's other frontends (readline, noninteractive) draw no dialog:
  nothing to do.
- Titles are generated from debconf-i18n; if a new debconf adds a
  language, rerun `scripts/fixtures/debconf-titles.py` and paste `--regex`.
