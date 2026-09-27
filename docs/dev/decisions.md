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

## D23. Apps that already have vim keys get detection-only specs — M6

**Deviation from brief §5.4**, which lists less, man, tig, k9s, lazygit,
ranger/lf/nnn and ncdu as key-remapping specs. Probing each one showed
they already move with `j`/`k` (most with `hjkl`, `g`/`G`, `C-d`/`C-u`):
remapping `j` → `Down` there changes nothing when it works and steals a
keystroke when detection is wrong. Their specs are hooks-only (no
`[keys]`): they feed the indicator and the transition hook and never touch
key tables. Only apps without vim navigation remap: htop and btop (btop
has an opt-in `vim_keys` option; the spec follows its semantics, `h`/`l`
= sort column). mc and fzf are `always = "insert"` (mc: the shell command
line is live in the panels, brief §5.4's suggested fallback).

## D24. Groups carry no key maps — M6

**Deviation from brief §5.3** ("curses-tui: … baseline hjkl map").
curses-tui is extended by hooks-only specs (tig, ranger, lf, nnn), which
would otherwise inherit keys, and a child can only suppress them one by
one. htop and btop declare their maps. New groups: `slash-search` (`/` `?`
at column 0 of the last row **and** the cursor there: the marker alone is
two characters), `colon-command` (same with `:`, mode `command`, typing
bucket; not for less, whose idle prompt is `:`), `readline-repl`
(`always = "insert"`, primary screen, no keys), `less-prompts` (less and
man; man must not inherit less's identity, since `extends` also merges
`[match]`).

## D25. less: reverse video is the tie-breaker; primary screen allowed — M6

less shows the file name as its first prompt, in reverse video; with an
absolute path it starts with `/`, like a search. A positive `normal` rule
(reverse video at the last row's column 0 + cursor there) runs first; text
prompts are never reverse video. This makes less the one spec whose
colour clause is load-bearing (the daemon captures `-e` for it). Without
colour (`@modal_color_matching off`, or `LESS_TERMCAP_so` restyling) such
a screen reads as insert — the harmless direction. `requires_alt_screen =
false` because git runs less with `LESS=FRX` (no alternate screen; pane
command `git`). Identity needs one of less's own prompts (`(END)`, HELP,
`(press RETURN)`, `lines N-M`), because the first screen and the idle `:`
do not identify it (vim's command line looks the same); positive mode
rules then carry a sticky identity. Locally the command identifies it.

## D26. REPLs identify by command only; shells never captured — M6

REPL specs run on the primary screen and match only their own command
names (globs allowed: `python3.*`). A prompt like `>>> ` is far too weak
to identify anything over SSH; there a remote REPL reads like the remote
shell (`none`, also typing). The tier-1 shell gate therefore stays
unconditional: a local shell on the primary screen is `none` without a
capture even when primary-screen specs are loaded (M5 disabled the gate
whenever one was, which would have captured every idle shell pane).

## D27. Lint and nesting heuristics tightened by real screens — M6

- `lintAbsenceAnchor` compares row ranges by resolving them at several
  heights and requires the identity clause to **cover** the mode rows
  (mixed ranges such as `[0, -2]` were mishandled).
- Specificity counts a small character class (`[↵↲]`, `[0-9]`) as one
  character.
- `lint_ignore = ["…"]`: a spec may accept a warning it has checked, with
  a comment (tig's title bar, k9s's one-character prompt markers). Not
  inherited. A unit test keeps every bundled spec lint-clean.
- Nested `borders` (F27): a border spanning the window must end in a plain
  line at both edges (tmux draws none along the edge, so mc's
  `├──┤├──┤` separator is not one); a column containing VT100 letters must
  also have junctions only where a `q` line meets them and blank cells
  beside it on at least half the rows.
- Per-variant fixture expectations (`expect_mode_remote`,
  `expect_mode_mono`, …) record where evidence genuinely differs, e.g.
  states with no identity anchor on screen are `unknown` remotely.

## D28. Unwatched panes' output is switched off for the daemon — M6

The daemon needs only the edge "pane X produced output", but tmux formats
and sends every byte of every pane's output to each control client. The
M6 benchmark showed the tmux server spending more on that than the daemon
spent on everything (5 htop panes, scope active: daemon 0.3 %, server
+0.47 %). `syncOutput` (each cycle) sends `refresh-client -A '%N:off'` for
panes outside `@modal_scope` (or disabled) and `on` when they come into
scope; a pane coming into scope is examined at once anyway. Per control
client state; skipped for good on tmux < 3.2 (logged). This is D2's
planned mitigation. Test: `TestUnwatchedPaneOutputResumesOnFocus` (fails
if the focused pane's output stays off).

## D29. Any app may get keys, via overlay user specs — owner, after M6

The owner asked for the *potential* to remap keys in apps that already have
vim keys ("for future unforeseen cases") and to unify the detect-only and
remapping "tiers". Finding: the engine already has one tier (`HasKeys()`
switches key tables, the two-capture confirmation D12, the nested policy
and the absence-anchor lint); the gaps are usability (adding keys meant
copying a whole bundled spec, which then goes stale) and that some
detection-only specs are not strict enough to remap safely (D21). Plan in
[milestones.md](milestones.md) M7 steps 1–4: `overlay = true` user specs,
every bundled spec made remap-ready, tested. The bundled specs stay
detection-only by default (D23 unchanged).

Done in v0.5.0:
- `overlay = true` in a user file of the same stem merges onto the earlier
  spec with the ordinary child-over-parent `merge`, except nothing is
  withheld (the bundled `extends`, `lint_ignore` keep applying); the
  overlay cannot rename (warned) and an overlay with nothing under it is
  skipped with a warning. `keys = false` drops the whole map (and `keys`
  is now validated: a table or false). `Spec.File` names both files.
- Remap-ready is enforced by `TestBundledSpecsRemapReady` (every
  non-always spec loaded with an overlay adding a key must be
  lint-clean). Only k9s needed a spec change (`Context:` required,
  threshold 60). The other four failures were the lint being too narrow,
  and it was widened on two sound grounds: (1) a weighted identity clause
  is *binding* when the other numeric weights cannot reach the threshold
  (ranger/lf/nnn: 50 + 50 against 100 — their status line on row -1 is
  replaced by the prompt, so normal can't be concluded while one is
  open); (2) a binding clause *pinned* to one row fixes the app to that
  edge, so it vouches for mode rows counted from the same edge (tig's
  title bar on row -2 for the prompt on row -1: anything foreign below
  would push it to -3). A top-pinned anchor still says nothing about the
  bottom rows, which is the htop case D21 was written for.
- The daemon logs per-spec lint warnings (bundled specs have none, so the
  log shows only user specs and overlays), and reloads the user spec
  directories when their files change (name/size/mtime stamp checked
  every idle interval), so an overlay applies without a restart.

