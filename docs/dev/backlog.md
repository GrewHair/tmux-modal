# Backlog

Ideas and known gaps that are **not** scheduled in a milestone yet. Each
entry says why it exists, what was already decided, and how to build it.
Promote an entry into [milestones.md](milestones.md) when it gets
scheduled; delete it when done (record the outcome in decisions/findings).

## B1. Split remote tmux: find the focused inner pane — deferred by the owner

**Today:** when the tmux running inside a pane (usually on a remote host)
has its window split into several panes, the outer screen shows all inner
panes at once and does not say which one has the keyboard. The daemon
reports `unknown` (N/A): no remapping, keys pass through, hooks get
`MODAL_TYPING=1`. Safe, but no mode info. (`classify.PaneWith`, `Nested.Split`.)

**Discussed with the owner (after M5), who chose to defer it until after
M6.** Design sketch:

1. Cut the screen into inner pane rectangles using the borders
   `DetectNested` already finds (`hasBorders` knows the full-span border
   columns/rows; generalise it to return the rectangles — nested splits
   need recursive cutting: a full-span border splits the screen, then look
   for full-span borders within each part, like tmux's own layout tree).
2. Pick the focused inner pane:
   - **cursor**: if the cursor is visible, it is inside the focused inner
     pane (tmux puts the real cursor there). Reliable for shells and for
     prompts (htop search);
   - **active border colour**: tmux's default `pane-active-border-style` is
     `fg=green`; the border cells adjacent to the focused pane are green
     (needs a `-e` capture; with two panes tmux ≥ 3.3 colours only the half
     of the shared border next to the active pane — verify per version);
   - neither → stay `unknown`.
3. Classify only that rectangle as a sub-screen (a `Screen` view with its
   own width/height/cursor; `WithoutRow` is the precedent) and apply the
   normal nested policy (`@modal_nested_remap`).
4. Fixtures: `scripts/fixtures/nested.sh` already captures split states
   (`split-left-htop`, `split-status`, `split-three`); add cases with the
   focus on the htop pane (cursor hidden → colour needed) and on a shell,
   in both border alphabets. Integration test in `remote_test.go`.

Risk to watch: never pick a pane on weak evidence — a wrong pick remaps
keys while the user types. Estimated half a day plus tests.

## B2. Nested `cursor_shape` delay (findings F11)

Through a nested tmux (3.7c), nvim's cursor shape reached the outer tmux
~0.5 s late on Esc, even with `escape-time 0` on both servers. Cause
unknown. Only matters for nvim on tmux ≥ 3.7-ish; the owner runs 3.4 where
`cursor_shape` does not exist at all. Look at it in M7.

## B3. Inner prefix key while remapping through nested tmux

Accepted by the owner: after the inner tmux's prefix key, a remapped key
reaches the inner tmux remapped (`prefix l` → `prefix Right`; tmux's
default prefix table binds only `l` of hjkl). Workaround documented
(escape leader). A fix would need to know the inner prefix — not visible
from outside. Only revisit if the owner complains.

## B4. Scheduler leftovers

- `@modal_burst_decay` is parsed but unused (event-driven scheduling made
  it moot; kept for a possible polling fallback).
- A spec's `poll_interval` is parsed but not consulted — planned for M6/M7.
- Chatty unfocused panes still cost a little: `%output` bytes are read and
  discarded. Mitigation if ever needed: `refresh-client -A '%N:off'` for
  unfocused panes (decisions D2). Measure in the M6 benchmark first.

## B5. Small things

- CI warns that `actions/checkout@v4` and `actions/setup-go@v5` target the
  deprecated Node 20 runtime; bump when newer majors are out.
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
