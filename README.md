# tmux-modal

A tmux plugin that works out **which mode the application in each pane is in**,
even when that application runs on the far side of an SSH connection, with
**nothing installed or configured on the remote host**.

It reads the rendered screen (the same cells you see) and fingerprints the
app and its mode from text, geometry, cursor state and, optionally, colour.
Two independent things can subscribe to that:

1. **Key remapping.** Give vim-style keys to TUIs that lack them. In `htop`
   and `btop`, `hjkl` move the selection, but the moment you open a search
   or filter prompt, `hjkl` are ordinary letters again.
2. **Transition hooks.** Run a command whenever a pane changes mode, so
   something outside tmux (AutoHotkey, kanata, karabiner, a status widget)
   can react — e.g. switch an outer keyboard layer off while vim is in
   normal mode and on while you type.

> **Status: pre-release (0.5).** Milestones done: detector, spec engine,
> `validate`, status indicator, transition hook, SSH and nested-tmux
> handling (tested against a real sshd), and [bundled specs](#bundled-specs)
> for htop, btop, less, man, tig, lazygit, k9s, ranger, lf, nnn, ncdu, mc,
> fzf and common REPLs, any of which [can be given keys](#add-keys-to-any-app).
> Still to come: the hooks-only `vim`/`nvim` specs.
> Any full-screen app no spec recognises shows as **N/A** and is left
> completely alone.

## Install

With [TPM](https://github.com/tmux-plugins/tpm):

```tmux
set -g @plugin 'GrewHair/tmux-modal'
```

then `prefix + I`. On load, the plugin downloads a prebuilt static binary
for your OS/architecture from the GitHub release matching its `VERSION`
(Linux, macOS, FreeBSD; amd64/arm64, plus linux/arm). If that fails and a Go
toolchain is on `PATH`, it builds one. If neither works the plugin stays
**inert**: nothing is remapped and keys pass through untouched.

The plugin starts one background daemon per tmux server. `tmux-modal stop`
(the binary is in the plugin's `bin/`) stops it and restores everything it
changed.

### How to recognise it

The daemon keeps one connection per session you have attached. It shows up
in `tmux list-clients` with the terminal type **`TMUX-MODAL-DAEMON`**:

```
client-4242: main [80x TMUX-MODAL-DAEMON] (attached,focused,control-mode,ignore-size,UTF-8)
```

It never affects window sizes (`ignore-size`) and never sends keys to panes.

## Showing the mode

For every pane the daemon publishes these pane options:

| Option | Values |
|---|---|
| `@modal_app` | spec name, e.g. `htop`; empty when nothing recognised |
| `@modal_mode` | `normal`, `insert`, …, `unknown` (a full-screen app no spec knows), `none` (no full-screen app) |
| `@modal_bucket` | `commanding`, `typing`, `unknown`, `none` |
| `@modal_confidence` | `high` or `low` |
| `@modal_nested` | empty, or why the pane looks like it shows another tmux: `command`, `status-line`, `borders`, `title` (see [SSH and nested tmux](#ssh-and-nested-tmux)) |
| `@modal_indicator` | a ready-rendered tag, see below |

Put the indicator wherever you like; a pane border is the natural place,
because the mode belongs to a pane:

```tmux
set -g pane-border-status top
set -g pane-border-format '#{pane_index} "#{pane_title}" #{@modal_indicator}'
# or the focused pane's mode in the status line:
set -g status-right '#{@modal_indicator} %H:%M'
```

Formats only read variables; all matching happens in the daemon, never in
a format string. You do **not** need `status-interval 0`: the daemon pushes
a status redraw (`refresh-client -S`) itself whenever an indicator changes,
and pane borders redraw on their own.

Indicator templates, one per bucket (`{MODE}`/`{mode}`, `{APP}`/`{app}`,
`{bucket}`, `{confidence}` are substituted):

| Option | Default | Shown for |
|---|---|---|
| `@modal_indicator_commanding` | `#[fg=black,bg=green,bold] {MODE} #[default]` | normal, visual, … |
| `@modal_indicator_typing` | `#[fg=black,bg=yellow,bold] {MODE} #[default]` | insert, command line, prompts |
| `@modal_indicator_unknown` | `#[fg=black,bg=colour244] N/A #[default]` | a full-screen app no spec recognises (left alone) |
| `@modal_indicator_none` | *(empty)* | plain shell, no full-screen app |

(`@modal_indicator_format` is accepted as an alias for the commanding one.)

## Transition hook

```tmux
set -g @modal_transition_hook '/path/to/command'
```

The command runs through `/bin/sh -c`, detached, with:

| Variable | Meaning |
|---|---|
| `MODAL_TYPING` | **`0` only when the pane is confidently in a commanding mode**, else `1` — the one most subscribers need |
| `MODAL_BUCKET` | `typing`, `commanding`, `none`, `unknown` |
| `MODAL_MODE_TO`, `MODAL_MODE_FROM` | e.g. `normal` → `insert` |
| `MODAL_APP`, `MODAL_APP_FROM` | spec names (empty for no app) |
| `MODAL_CONFIDENCE` | `low` when the mode came from corroboration rather than a positive marker, or through a nested tmux |
| `MODAL_NESTED` | same as `@modal_nested`: empty unless the pane shows another tmux |
| `MODAL_EVENT` | `mode` (mode changed), `focus` (focus moved to another pane), `stop` (daemon exiting) |
| `MODAL_PANE`, `MODAL_PANE_ACTIVE`, `MODAL_WINDOW`, `MODAL_SESSION`, `MODAL_SESSION_ID`, `MODAL_TIMESTAMP_MS` | where and when |

`MODAL_TYPING=1` for a plain shell (you are editing a command line) and for
unrecognised apps: a subscriber should only ever do the "commanding" thing
when the daemon is sure.

The hook reports **the focused pane of each session**, both when its mode
changes and when focus moves to another pane — that is what an outer
keyboard layer needs to know ("what am I typing into right now?").
Unfocused panes' changes are reported too, with `MODAL_PANE_ACTIVE=0`,
when `@modal_scope` makes them observed.

Guarantees:
- Rapid transitions are **debounced** (`@modal_hook_debounce`, 30 ms): one
  call with the final state, reported from the state the subscriber last saw.
- If the previous call for the same session/pane is still running, it is
  **killed** (with its whole process group) and the newer one runs: a stale
  transition is worse than a missed one.
- Each call is killed after `@modal_hook_timeout` (500 ms).
- A spec may carry its own `hook = "..."`, run after the global one.

### Examples (in `examples/hooks/`)

| Script | What it does |
|---|---|
| `log.sh` | appends a line per transition to `~/.local/state/tmux-modal/transitions.log` — `tail -f` it to see exactly what a subscriber receives |
| `notify.sh` | desktop notification (`notify-send` / `terminal-notifier`) |
| `windows-toast.sh` | a Windows toast from WSL (detached, because `powershell.exe` alone takes ~0.5 s to start) |
| `fifo.sh` + `../listener.sh` | **the low-latency pattern**: the hook writes one line to a FIFO, a long-lived listener reacts |

**Latency advice:** a long-lived listener fed by a one-line hook will always
beat spawning a heavyweight process per transition. Process start costs tens
of milliseconds natively and hundreds across a WSL→Windows boundary. For a
keyboard layer, keep the AutoHotkey/kanata side running and feed it through
`fifo.sh` (or your own equivalent), not by launching it per transition.

## Bundled specs

| App | What the spec does | Modes it reports | Verified with |
|---|---|---|---|
| `htop` | **remaps keys** | normal; insert in `Search:` / `Filter:` | 2.2.0, 3.0.5, 3.2.2, 3.3.0 |
| `btop` | **remaps keys** | normal; insert while typing the process filter, and on the options screen (it has text fields) | 1.2.3, 1.2.13, 1.3.0 |
| `less`, `man` | detects | normal; insert in any text prompt (`/` `?` search, `&` filter, `!` shell, `-` option, `:e` …) | less 590; man-db 2.10–2.12 |
| `tig` | detects | normal; insert in `/` `?` search; command at `:` | 2.5.1, 2.5.5, 2.5.8 |
| `lazygit` | detects | normal; insert whenever a text field has focus (commit message, filter, shell command, branch name …) | 0.65.1 |
| `k9s` | detects | normal; insert in the `/` filter; command at `:` | 0.51.0 |
| `ranger`, `lf`, `nnn` | detect | normal; insert / command in their prompts (search, rename, console …) | ranger 1.9.3; lf r28, r31; nnn 4.3–4.9 |
| `ncdu` | detects | always normal (it has no text prompts) | 1.15.1, 1.18, 1.19 |
| `mc` | always insert | its shell command line is always live: letters typed in the panels go there | 4.8.27–4.8.30 |
| `fzf` | always insert | every letter goes to the query | 0.29, 0.38, 0.44 |
| `python` (IPython, bpython, ptpython), `node`, `irb`/`pry`, `psql`/`pgcli`, `mysql`/`mariadb`/`mycli`, `sqlite3`/`litecli`, `gdb`, `lldb`, `weechat`, `irssi` | always insert | line-reading REPLs and chat clients | by command name |

**Why most specs only detect.** less, tig, lazygit, k9s, the file managers
and ncdu already move with `j`/`k` (most with `hjkl`, `g`/`G`); remapping
would add risk and nothing else. Their specs exist for the indicator and
the transition hook, and they never touch your key tables. Only apps that
lack vim keys get a key map. Every one of them is still ready to remap:
[a three-line file](#add-keys-to-any-app) gives any app keys.

**Over SSH** every spec works from the screen alone, with a few limits:
- `less` shows nothing identifying on its first screen (just the file name)
  or while idle (a bare `:`), so remotely it is recognised once it shows
  one of its own prompts (`(END)`, `HELP`, a `(press RETURN)` message) and
  from then on for as long as it runs. `man` is recognised at once.
- REPLs are recognised by their local command name only; a remote Python
  reads like the remote shell it was started from (both are typing).
- k9s is recognised by its header block; with the header hidden
  (`ctrl-e`) it is N/A.

**Escape does not close every prompt.** less and tig use `Esc` to start a
key sequence inside the prompt; close them with `Enter` or `C-c`. The mode
follows whatever the app does.

## Key remapping

With htop focused and in normal mode, the session's key table becomes
`modal-htop`:

| Key | Sends | Note |
|---|---|---|
| `h` `j` `k` `l` | Left Down Up Right | **shadows htop's `h` (help), `k` (kill), `l` (lsof)** |
| `g` / `G` | Home / End | |
| `C-d` / `C-u` | PageDown / PageUp | |
| `_` then a key | that key, verbatim | `_h` = help, `_k` = kill, `_l` = lsof, `_ _` = `_` |

When htop's `Search:` or `Filter:` prompt opens, the key table goes back to
yours within a few tens of milliseconds and every key types literally.
htop's other panels (F9 kill, `u` user, F2 setup) stay in normal mode.

btop (`modal-btop`) gets the same map, following btop's own `vim_keys`
option: `h`/`l` choose the sort column (btop's Left/Right). It shadows
btop's `h` (help) and `k` (kill the selected process); `_h`, `_k` reach
them. While the process filter (`f` or `/`) is being typed, and on the
options screen, nothing is remapped; the main menu, help and the signal
list (arrows pick a signal) are normal mode. If you prefer btop's own
`vim_keys = True`, drop the map with an overlay (below) holding
`overlay = true` and `keys = false`.

### Add keys to any app

Any bundled spec can get keys, or lose or change some, without copying it.
Put a file with the **same file name** in `~/.config/tmux-modal/specs/`
and start it with `overlay = true`:

```toml
# ~/.config/tmux-modal/specs/tig.toml
overlay = true

[keys]
K = "Down"          # add or change a key
# j = false         # remove one the bundled spec maps
# keys = false      # (top level, before any [table]) drop the whole map
```

The overlay is merged onto the bundled spec as if the bundled spec were
its parent: `[keys]` key by key, scalars replaced, rule lists (like
`[[insert_when]]`) added in front of the bundled ones. The detection itself
stays the bundled one, so fixes to it in later versions still reach you.
`tmux-modal validate ~/.config/tmux-modal/specs/tig.toml` prints the merged
result. Without `overlay = true`, a file of the same name **replaces** the
bundled spec instead. Spec files are re-read when they change (within
`@modal_idle_interval`), so no restart is needed.

Every bundled spec that can be in a commanding mode is held by a test to
the rules a remapping spec must meet, so keys added this way are only
applied when the app is positively recognised and no prompt is open. The
daemon log warns if an overlay (or your own spec) breaks those rules.

Details that make this safe:
- Only the session `key-table` option is used, never a sticky
  `switch-client -T`, so unbound keys still reach the pane.
- All your root-table bindings (mouse, `bind -n …`) are copied into the
  modal table, so they keep working in htop. A root binding on a remapped
  key (`h`, `j`, …) is shadowed while htop is in normal mode.
- Every remapped key re-checks, at the moment you press it, that the pane
  receiving it is really htop in a commanding mode; otherwise it sends the
  plain key. A key map can therefore never apply to the wrong pane, even in
  the instant after a focus change.
- A spec **without** a key map (hooks-only) never touches `key-table`.
- Returning to a remapped mode needs two agreeing captures
  (`@modal_confirm_captures`); entering a typing mode needs one. A false
  "normal" steals keystrokes; a false "insert" only loses the remap.

## Options

| Option | Default | |
|---|---|---|
| `@modal_enabled` | `on` | also per window/pane: `set -p @modal_enabled off` |
| `@modal_profile` | `balanced` | `frugal`, `balanced`, `snappy`; any interval set below overrides it (becomes `custom`) |
| `@modal_burst_interval` | 30 | ms of quiet after output before a pane is examined (and the fastest per-pane cadence) |
| `@modal_poll_interval` | 150 | ms: longest wait under continuous output; re-check cadence for unresolved panes |
| `@modal_idle_interval` | 2000 | ms: safety re-check and config reload cadence |
| `@modal_burst_decay` | 1200 | ms (reserved for the polling fallback) |
| `@modal_scope` | `active` | `active` (focused panes), `visible` (panes in current windows), `all` |
| `@modal_capture_rows` | 6 | minimum bottom rows captured once an app is identified |
| `@modal_cpu_budget` | 2 | % of one core; intervals stretch automatically above it (logged) |
| `@modal_spec_paths` | `~/.config/tmux-modal/specs` | colon-separated; a user spec replaces a bundled one with the same file name, or with `overlay = true` [merges onto it](#add-keys-to-any-app) |
| `@modal_escape_leader` | `_` | literal-key leader (a spec's `[escape] leader` wins) |
| `@modal_color_matching` | `on` | `off` never captures colour |
| `@modal_transition_hook` | *(none)* | see above |
| `@modal_hook_timeout` | 500 | ms |
| `@modal_hook_debounce` | 30 | ms |
| `@modal_confirm_captures` | 2 | captures that must agree before remapping resumes |
| `@modal_nested_remap` | `on` | remap keys inside a tmux running in the pane (usually on a remote host); `off`: such panes are N/A, keys untouched. See [SSH and nested tmux](#ssh-and-nested-tmux) |
| `@modal_log_level` | `warn` | log: `~/.local/state/tmux-modal/<server>.log` |

Profiles: `frugal` 100/500/5000 ms (burst/poll/idle), `balanced` 30/150/2000,
`snappy` 15/60/1000.

Options, and the spec files in `@modal_spec_paths`, are re-read every idle interval, so changes apply without restarting.

### Measured (WSL2 on a laptop, tmux 3.4)

**Latency**, keypress to published mode, one pane: entering typing
**37 ms** (balanced) / **28 ms** (snappy); returning to commanding
**94 ms** / **76 ms** (includes the deliberate two-capture confirmation).

**CPU**, % of one core, with **htop in every pane** (it redraws every
1.5 s, so every pane is busy), tiled in one window; "server" is what the
tmux server spends on top of its own baseline to serve the daemon
(`TestBenchmarkCPU`, 30 s per measurement):

| Panes | `@modal_scope` | frugal: daemon / server | balanced | snappy |
|---|---|---|---|---|
| 5 | `active` (default) | 0.20 % / +0.20 % | 0.13 % / +0.17 % | 0.23 % / +0.40 % |
| 5 | `all` / `visible` | 0.33 % / +0.57 % | 0.33 % / +0.53 % | 0.60 % / +0.93 % |
| 20 | `active` (default) | 0.10 % / +0.30 % | 0.17 % / +0.33 % | 0.27 % / +0.50 % |
| 20 | `all` / `visible` | 1.50 % / +3.63 % | 1.63 % / +4.16 % | 2.36 % / +5.46 % |

With the default scope only the focused pane of each attached session is
examined, and tmux stops streaming the other panes' output to the daemon,
so the cost barely grows with the number of panes. `visible`/`all`
examine every busy pane each time it redraws: 20 constantly redrawing
panes is the expensive case, and `@modal_cpu_budget` (2 %) then stretches
the intervals. Idle panes cost nothing in any scope.

**`escape-time` matters more than any of this:** tmux holds a bare `Esc`
for `escape-time` (default **500 ms**) to see whether an escape sequence
follows, so every "leave insert with Esc" is late by that much. Use
`set -s escape-time 10` (or 0).

## How detection works

`#{pane_current_command}` is `ssh` for every remote app, and no SSH
mechanism reveals remote process state, so identity and mode are inferred
from the rendered screen:

- **Tier 0 — events.** The daemon's control-mode connection receives a
  notification whenever a pane produces output. Idle panes cost nothing;
  a busy pane is examined once its output has been quiet for the burst
  interval (so captures do not land mid-redraw), and at least every poll
  interval under continuous output.
- **Tier 1 — format read.** One `list-panes -a` per cycle reads O(1) state
  for every pane: alternate screen, cursor position/visibility/shape,
  size, command. A plain shell on the primary screen is `none` without
  any capture.
- **Tier 2 — capture.** One `capture-pane` per examined pane serves both
  identification and mode detection; identity is sticky and re-checked
  cheaply.

Modes are declared per spec and mapped to **buckets** (`typing` /
`commanding`), which are what subscribers consume. "Normal" is never a
fallback: concluding it from the *absence* of a mode marker requires the
app's identity to be re-confirmed on the same capture. A garbled or
unrecognisable screen gives `unknown` — pass-through — never `normal`.

### SSH and nested tmux

Over plain SSH (`ssh host`, then `htop`; or `ssh -t host htop`) detection
and remapping work exactly as locally, with nothing installed on the remote
host: the pane's command is `ssh`, so the application is recognised from
its screen alone. This is tested against a real sshd in a container.

If you run **tmux on the remote host too**, the inner tmux is a terminal of
its own: it draws its status line and pane borders over the application
and has its own prefix key. The plugin recognises it (its default status
line, pane borders, a local `tmux` client in the pane, or the `set-titles`
title), removes the inner status line before looking at the screen, and
then:

- **One inner pane:** works as usual — htop is recognised and remapped,
  with `@modal_confidence` `low`. The one quirk: right after the *inner*
  prefix key, a remapped key reaches the inner tmux remapped (`prefix l`
  arrives as `prefix Right`; tmux binds none of `h j k` in its prefix
  table by default, only `l`). Type the escape leader first
  (`prefix _ l`), or set `@modal_nested_remap off` to leave every nested
  pane alone (N/A).
- **Inner window split into several panes:** N/A (`unknown`,
  pass-through). The outer screen shows all inner panes at once and does
  not say which one has the keyboard, so no mode is claimed and keys go
  through untouched.
- `@modal_nested` / `MODAL_NESTED` say why a pane was taken as nested:
  `command`, `status-line`, `borders` or `title`.

A remote tmux with its status line off and a single pane is not even
recognised as nested — it looks exactly like plain SSH — which makes no
difference with the default setting.

The brief's cheap tier-1 signals for nesting (alternate screen held with no
toggles, frozen history, a scroll region short of the pane) turned out not
to distinguish the cases on tmux 3.4: plain `ssh -t host htop` shows the
same values, and the inner tmux did not leave a scroll region set. They are
not used.

`#{cursor_shape}` (what nvim uses to show insert/normal) is absent in
tmux 3.4 and 3.5a and present in 3.7c (the exact release that added it is
not pinned down); it does pass
through a nested tmux, but the inner tmux delays the update by ~0.5 s. It
is used only to corroborate, never as the primary signal.

## Writing specs

A spec is a TOML file: an identity predicate, a mode classifier, and
optional per-consumer config. Rules are **lists of weighted clauses**
combined against a threshold, so several weak anchors can corroborate each
other. See [`specs/htop.toml`](specs/htop.toml) for a complete, verified
example and [`specs/groups/`](specs/groups/) for shared groups.

```toml
name     = "myapp"
priority = 50
extends  = "curses-tui"          # a group, or a list of them
modes    = ["normal", "insert"]

[buckets]                         # mandatory for every declared mode
typing     = ["insert"]
commanding = ["normal"]

[match]                           # identity
command             = ["myapp"]   # local fast path only (an SSH pane says "ssh"); globs ok: "python3.*"
requires_alt_screen = true
combine             = "weighted"
threshold           = 60
  [[match.clause]]
  regex  = '^MyApp v[0-9]+ +'     # POSIX-ish ERE (Go RE2 syntax), right-trimmed line
  row    = 0
  weight = 60
  [[match.clause]]
  regex  = '(F1 Help +F10 Quit|Search:( .*)?)$'   # the bar, or the prompt that replaces it
  rows   = [-1, -1]               # negative = from the bottom
  weight = "required"             # see "Anchor the rows you read" below

[[insert_when]]                   # first hit wins; no hit => normal
name  = "search"
regex = '^Search:( |$)'           # NOT '^Search: ' — lines are right-trimmed
rows  = [-2, -1]
col   = 0
cursor_rows = [-2, -1]            # and the cursor is on it: a short marker is then specific enough

[keys]                            # omit for a hooks-only spec
j = "Down"
k = "Up"
```

Clause keys: `regex` with `rows`/`row`, `col`/`cols`, `anchor`
(`start`/`end`), `min_occurrences`/`max_occurrences`/`contiguous`; colour
(`row`, `col`, `fg`, `bg`, `attrs`, `tolerance`; names, palette indices or
`#RRGGBB`, compared in RGB); cursor (`cursor_rows`, `cursor_cols`,
`cursor_visible`, `cursor_shape`); `negate`; `weight` (a number,
`"bonus"` or `"required"`). Also: `always = "insert"`, `[[corroborate]]`,
`[default_mode]`, `[escape] leader`, `[shadowed]` (documentation), `hook`,
`poll_interval` (ms; replaces `@modal_poll_interval` for panes running the
app), `translated = false`, `lint_ignore = ["under-specified"]` (accept a
lint warning you have checked; say why in a comment).

Groups in [`specs/groups/`](specs/groups/) (`extends`, a list is merged in
order): `curses-tui` (cursor visible on the bottom rows = a prompt has
focus), `slash-search` (`/` `?` on the last row with the cursor there),
`colon-command` (`:` there, mode `command`), `readline-repl` (always
insert, primary screen), `less-prompts` (less's own prompts). Groups carry
no key maps.

**Anchor the rows you read.** A remapping spec concludes its commanding
mode from the *absence* of a prompt. Make sure the rows the mode rules read
are the app's own: give the identity a `weight = "required"` clause on
those rows (the bar the prompt replaces, or the prompt itself), or a
required clause pinned to a single row on the same side of the screen
(tig's title bar on row -2 proves row -1 is tig's: anything foreign below
would push the title bar up). A weighted clause the threshold cannot be
reached without counts as required. Otherwise,
under a remote tmux whose status line covers the last row, the other
anchors still match, the prompt one row up goes unseen, and keys get
remapped while you type. `validate` and `lint` warn about this.

### `tmux-modal validate`

The authoring tool. It lints a spec, prints the fully resolved spec after
inheritance, and scores it against a screen, clause by clause:

```sh
tmux-modal validate myapp.toml                    # lint + resolved spec
tmux-modal validate --capture %3 myapp.toml       # against a live pane
tmux-modal validate --fixture saved.txt myapp     # against a saved screen
tmux-modal validate --capture %3                  # which spec claims this pane, and why
tmux-modal capture -o saved.txt %3                # save a pane as a fixture
tmux-modal lint --locale-audit                    # every spec
```

```
== htop: mode rules (first hit wins)
  [0] insert/search  (combine=all threshold=100)
    HIT  +100  w=100      total=100  regex "F3Next +(S-F3Prev +)?Esc ?Cancel +Search:( |$)" rows=[-1,-1] col=0
           @ row 23 col 0  "F3Next  S-F3Prev   EscCancel    Search: "  fg=default bg=default attrs=none
    score 100 / 100 -> FIRED
=> app=htop mode=insert bucket=typing confidence=high
```

The lint flags unanchored patterns, rules with fewer than 12 anchored
characters, required colour clauses, prompt regexes ending in whitespace,
remapping specs whose prompt rows no required identity clause anchors, and
(with `--locale-audit`) English-only markers. Every bundled spec is
lint-clean (a unit test enforces it).

## Why Go

A compiled daemon makes the CPU budget easy to keep (the hot path is a
socket read and a few regexes) and ships as one static binary with no
runtime. Installation stays frictionless because TPM fetches the prebuilt
binary; nobody needs a toolchain. There is deliberately no shell
reimplementation of the detector.

## Development

```sh
go test ./internal/...          # unit + golden fixtures (no tmux needed)
go test ./tests/integration/    # real tmux, real htop, real attached client; SSH tier needs docker
scripts/fixtures/htop.sh LABEL docker run --rm -it IMAGE htop   # recapture fixtures
scripts/fixtures/nested.sh      # recapture the nested-tmux fixtures
scripts/fixtures/apps.sh [ubuntu-24.04|ubuntu-22.04|debian-bookworm] [app...]   # the other apps
TMUX_MODAL_BENCH=1 go test ./tests/integration/ -run TestBenchmarkCPU -v -timeout 30m   # CPU table below
```

Integration tests run each case on two private tmux servers (`-L`,
`-f /dev/null`): the outer one's pane runs `tmux attach` to the inner one,
so keystrokes go through a real client's key tables. The SSH tier runs the
application in an sshd container (`tests/docker/sshd.Dockerfile`, built on
first use, with every bundled app installed) and skips itself without
docker; `tests/integration/apps_test.go` drives each app's prompts there. Fixtures are captured
inside containers (`tests/docker/`) so they contain no host data.

## License

MIT
