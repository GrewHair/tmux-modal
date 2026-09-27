# Decisions

Each entry: what was decided, why, and whether the owner approved it. "Brief"
means [brief.md](brief.md); departures from it are marked **deviation**.

## D1. Go, one static binary, no shell fallback — approved

Compiled daemon keeps the CPU budget easy and ships as one static binary.
Brief §10 asked for "prebuilt binaries **and** a shell fallback".
**Deviation, approved:** the owner called a shell reimplementation "sluggish
overkill". Install order (`scripts/binary.sh`): existing `bin/tmux-modal` with
matching version → download release asset `tmux-modal-<os>-<arch>` →
`go build` if Go is on PATH → stale binary → inert (keys untouched).

## D2. Control-mode transport instead of spawning tmux per poll — approved

The daemon keeps one `tmux -C attach-session -f ignore-size -t <session>`
client per session that has a human (non-control) client attached. Commands
are pipelined over it (no process spawn per command); `%output`
notifications are the tier-0 activity signal, so the brief's "there is no
pane-output hook, polling is unavoidable" (§3.6) is sidestepped: detection is
event-driven with a trailing debounce.

- Only sessions with humans get a connection; with none attached the daemon
  falls back to `tmux` exec calls every idle interval. Reason: an attached
  control client counts as "attached" (matters for `destroy-unattached`).
- Control clients are labelled by setting `TERM=TMUX-MODAL-DAEMON` for the
  control process, which `list-clients` prints — **owner's requirement**:
  "something very explicit so i can always tell what's that thing, even if
  i'm drunk or sleepy".
- `pane-activity`/`monitor-activity` (§3.6, §13) were not used: `%output` is
  strictly better and needs no user option changes.
- `#{C:}` (§3.2) is not used: capture over the control socket is cheap
  (no spawn) and gives the full matcher.
- Output bytes are read and discarded (only the pane id is kept). If chatty
  background panes ever cost too much, `refresh-client -A '%N:off'` for
  unfocused panes is the planned mitigation (owner was told about this).

## D3. `switch-client -c <client> -T <table>` after every `set key-table` — approved

Setting the `key-table` option alone leaves each client sitting in its old
table until its next key. That old table is no longer the default, so an
unbound key is retried in the new default and then **discarded**
(`server_client_key_callback`). Observed symptom: the first letter typed into
htop's search vanished. The daemon therefore re-points each human client
whose current table is "default-ish" (old default, `root`, a `modal-*` table,
but not `prefix` or a `modal-literal-*` one-shot table) to the new default.
This is not the §3.1 trap: the target is always the table that is already
the default. Tested: `TestRemapAndPromptPassThrough`.

## D4. Guarded bindings

Every remap is `if-shell -F '<guard>' { send-keys MAPPED } { send-keys KEY }`
where the guard checks, at keypress time, that the receiving pane's
`@modal_app` is this app, `@modal_bucket` is `commanding` and the pane is
not in a tmux mode. A stale session table (daemon lag, crash) therefore
cannot remap in the wrong pane. Tested by `TestStopAndCrashRecovery`.

## D5. Root bindings are copied into each modal table

`list-keys -T root` lines are re-executed with `-T root` rewritten to
`-T modal-<app>`, except keys the spec remaps and the leader. Without this,
mouse and every `bind -n` key would die while an app's table is the default.
Re-copied when the root table's hash changes. Tables are
`modal-<app>` / `modal-literal-<app>` (per app, since key maps differ), not
the brief's single `modal`.

## D6. Key-table ownership and restore

The daemon only restores a table it set. Before first taking over a
session it stores `@modal_saved_key_table` = `local:<value>` or `inherit`
on the session, so a restarted daemon (after a crash) can restore it.
Hooks-only specs never cause `key-table` or `@modal_saved_key_table` to be
written (test: `TestHooksOnlyNeverTouchesKeyTable`).

## D7. `extends` accepts a list — deviation

§5.2 says single inheritance; §5.4 gives htop two groups
(`curses-tui, slash-search`). A list is resolved as an ordered mixin chain
(first entry highest precedence after the child); cycles are rejected.
htop in practice extends only `curses-tui`: `slash-search` rules (`^/` on the
last row) do not apply to htop's prompt bar.

## D8. Rule normalisation

All rule forms compile to one representation — a list of clauses with a
threshold: `[[match.screen]]` → `[[match.clause]]` (`screen_mode` → `combine`),
`[[insert_when]]` → `[[mode_when]] mode="insert"` (appended after explicit
`mode_when`), inline matcher keys on a rule → one clause per matcher kind,
AND-ed. Weights: number, `"bonus"` (reported, never needed), `"required"`
(gate). Defaults: `combine="all"` (threshold = sum of numeric weights),
explicit `threshold` implies `weighted`. Clauses are tri-state
(satisfied / unsatisfied / unavailable); unavailable (row outside a partial
capture, no colour captured, `cursor_shape` unsupported) never counts either
way, and `negate` does not turn it into evidence.

## D9. Occurrence counting semantics

`contiguous = true` uses the **longest run** of matching rows (not "all
matching rows form one run"), so a stray `~` line in file content cannot
break vim's tilde anchor.

## D10. Regex dialect

Go RE2 (`regexp.Compile`), not strict POSIX ERE: the brief's own examples use
`\s` and `\b`, which strict ERE lacks. No backreferences. `\b` is ASCII-only.

## D11. Colour captured only when load-bearing

The daemon uses `capture-pane -e` only if some spec has a numeric or required
colour clause. Bonus colour clauses cannot change a result, so they are
evaluated only by `validate` (and whenever colour is captured anyway).

## D12. Transition asymmetry

Entering a typing mode: first capture. Returning to a *remapped* commanding
mode: `@modal_confirm_captures` (2) agreeing captures, re-examined at burst
cadence. Hooks-only specs: every edge immediately (brief §4.4 rule 2);
the hook debounce is the only guard.

## D13. `MODAL_TYPING` is 0 only when confidently commanding

`none` (a shell: line editing) and `unknown` report `MODAL_TYPING=1`, so a
subscriber only does the "commanding" thing when the daemon is sure.
(Brief: none/unknown are "in neither bucket"; it did not specify TYPING for
them.)

## D14. Hook event model — extension of §8

Events: `mode`, `focus`, `stop` (`MODAL_EVENT`). The hook follows **each
session's focused pane** (a keyboard layer needs "what am I typing into
now", which changes on focus moves too); unfocused panes' changes are
emitted with `MODAL_PANE_ACTIVE=0`. Debounce/cancellation key is per session
for the focus stream, per pane otherwise. Coalesced events keep the
`MODE_FROM` the subscriber last saw. Supersession kills the whole process
group. A `stop` event is flushed on shutdown. Extra vars: `MODAL_EVENT`,
`MODAL_APP_FROM`, `MODAL_SESSION_ID`.

## D15. Indicator is pre-rendered by the daemon — owner's design input

`@modal_indicator` per pane, from per-bucket templates
(`@modal_indicator_{commanding,typing,unknown,none}`, placeholders `{MODE}`
`{mode}` `{APP}` `{app}` `{bucket}` `{confidence}`). **Owner asked for
three-way: NORMAL / INSERT / N/A** — N/A is the `unknown` bucket (a
full-screen app no spec recognises, left alone); shells show nothing.
`@modal_indicator_format` kept as alias for the commanding template.

## D16. No `status-interval 0`

The owner's status line has a clock (`status-interval 15`). The daemon pushes
`refresh-client -S` to human clients once per cycle when an indicator changed;
pane borders redraw by themselves on pane-option changes. The brief's
recommendation to set `status-interval 0` is documented as unnecessary.

## D17. Scheduling semantics of the intervals

With events, the knobs mean: `burst` = trailing quiet time after output
before examining (also the per-pane rate floor); `poll` = cap on waiting
under continuous output, and re-check cadence for unresolved panes
(unknown, identity failing, pending confirmation); `idle` = safety re-check,
config reload and reconcile cadence; `burst_decay` reserved. Default scope
`active`: unfocused panes are not examined at all (owner asked; confirmed).

## D18. Fixtures captured in containers

Real process lists, paths and host names must never enter the public repo.
Fixtures are captured inside `tests/docker/apps.Dockerfile` images, with a
neutral pane title (`scripts/fixtures/lib.sh`). History was rewritten once
before the first push to remove the host name.

## D19. Publishing moved earlier — owner's request

The owner asked to publish after M4 (not M8) and to keep pushing. Push after
each milestone (CI must be green); tag a release when daemon code changes.

## D20. Nested multiplexers — M5, revised by the owner

`classify.DetectNested` (O(1) per capture, full screen only for borders):
local command `tmux`/`tmate`/`screen`/`zellij`/`byobu` → `command`; else,
only when the alternate screen is on and **no spec claims the pane's
command** (a local htop is never nested): tmux's default status line on
the last or first row → `status-line` (the row is then removed before
matching); an interior full-height column of vertical border characters
or full-width row of horizontal ones (UTF-8 or VT100 letters) → `borders`
(`Split`); the default `set-titles-string` shape → `title`. Policy in
`classify.PaneWith`: every spec reports its mode with confidence `low`,
or `unknown` when split. **Owner's decision (after M5):** remapping
through a nested tmux is **on by default**, option `@modal_nested_remap`
(`off` → a remapping spec reports `unknown` there, as in v0.2.0). The owner
accepted the inner-prefix quirk (tmux's default prefix table binds only
`l` of hjkl). v0.2.0 shipped with nested remapping off. Published as `@modal_nested` /
`MODAL_NESTED`. Nested panes are always captured in full (no band). A
remote tmux with status off and one pane is indistinguishable from plain
SSH and remaps as usual — the plan allowed this ("keys go through to the
inner active pane"); the inner-prefix caveat is in the README.

## D21. Absence must be anchored where the mode rules read — M5

A remapping spec whose default is commanding must have a required
identity clause (or a clause under `combine = "all"`) on the rows its
mode-rule regexes read; `lint` warns otherwise (`lintAbsenceAnchor`).
htop's bottom-bar clause became `required` (threshold 100 → 40: bar plus
the PID header or both meters, the same acceptance as before when the bar
is present). Found by F22.

## D22. Key-table change and client re-point in one command list — M5

`set-option key-table X ; switch-client -c C -T X …` is sent as one control
line, so no keystroke can arrive between the two (such a key would be
discarded, see D3). Theoretical window; the latency-test flake that
prompted the look turned out to be F25. The option comes first, so a
failing re-point (client just detached) cannot drop it (F4).
