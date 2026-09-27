# Original brief (verbatim)

> This is the project brief as given by the owner at the start of the work,
> reproduced verbatim so that its exact requirements survive context
> compaction. Where the implementation deliberately departs from it, see
> [decisions.md](decisions.md); where its factual claims turned out wrong,
> see [findings.md](findings.md). Section numbers (§) used throughout the
> dev docs refer to this document.

---

## 1. Mission

Build a tmux plugin that **detects and publishes the modal state of whatever is running in a pane** — including applications on the far side of an SSH connection, with **zero configuration on the remote host**.

Mode detection is the product. It has **two independent consumers**, and an application spec may use either, both, or neither:

1. **Key remapping.** Give vim-style navigation to TUIs that lack it. Canonical example: `htop`, navigated with arrows and function keys. We want `hjkl` to move — but the instant the user presses `/` or `\` and htop opens a text prompt, `hjkl` must become literal characters again.
2. **Transition hooks.** Run an arbitrary command whenever a pane changes mode, so external software can react. Canonical example: `vim`. Remapping `hjkl` there is pointless — vim already does it. But an outer keyboard layer (AutoHotkey, kanata, karabiner) that is helpful while typing is actively harmful in normal mode, and it has no way to know which mode vim is in. This plugin tells it.

**Design consequence, and the most important sentence in this document:** the key-remapping machinery is optional and must be *entirely skipped* for specs that do not define a key map. A hooks-only spec must never cause the session `key-table` option to be touched. Build the detector first and treat both consumers as subscribers to it.

---

## 2. Why this is hard (read this or you will design the wrong thing)

`#{pane_current_command}` resolves the foreground process of the pane's pty via the local kernel. For an SSH pane it returns `ssh`, permanently. Everything running on the remote lives in a different kernel. There is **no** OpenSSH mechanism to interrogate remote process state — no protocol message, nothing in the control channel.

Therefore **application identity and mode must be inferred from the rendered screen.** That is the core of this project. Process-table inspection is a fast path for the local case only; it must never be the sole mechanism.

Design consequence: the fingerprinting engine is the product. The key remapping is comparatively trivial.

---

## 3. Verified tmux facts

These were checked against tmux master source. **Do not re-derive them, and do not contradict them.** If your implementation seems to require contradicting one, stop and report the conflict rather than working around it.

### 3.1 Key tables

- `key-table` is a **session** option (`OPTIONS_TABLE_SESSION` in `options-table.c`), default `"root"`. **There is no per-pane key table.**
- `server_client_get_key_table()` in `server-client.c` reads that session option. `server_client_is_default_key_table()` compares the active table against it.
- **Critical:** when a key has no binding in the current table, tmux checks the `Any` binding, then: if the table is *not* the default, it resets the client to the default table and **discards the key**. If the table *is* the default, the key falls through to `forward_key` and reaches the pane.
- **Therefore:** enter the modal table with `set key-table modal` (making it the default), **never** `switch-client -T modal`. The latter causes every unbound keypress to be eaten and to kick you out of the mode.
- `Any` is a genuine catch-all key in tables. Available if needed, but the option-based approach above should make it unnecessary.
- Because the option is session-scoped and keystrokes only ever reach the active pane, the model is: **each pane owns a mode; the session's `key-table` mirrors the active pane's mode.** Re-sync on `pane-focus-in`.

### 3.2 `#{C:...}` — in-format pane content search

- `#{C:string}` searches the pane and returns the **1-based line number** of the first match, or `0`. It is not a boolean.
- `#{C/r:regex}` = POSIX extended regex. `#{C/i:...}` = case-insensitive. `#{C/ri:...}` = both.
- Without `/r`, the string is wrapped in `*…*` and matched with `fnmatch` — a substring test.
- It reads `wp->base`, the **rendered cell grid**, so it works through a nested remote tmux exactly like `capture-pane`.
- It scans the **visible screen only**, never scrollback.
- It **early-exits on the first match**, so it cannot count matches. Each additional `#{C:}` rescans from row 0.
- `#{C/r:...}` calls `regcomp` on **every evaluation**. Prefer the non-regex form where a substring test suffices.
- Cost: walks up to `pane_height` rows, one string allocation per row. Tens of microseconds. No fork, no socket. Roughly 10–50× cheaper than `capture-pane` piped to an external matcher, where process spawn dominates.
- **But** it runs synchronously inside the tmux server event loop, blocking render and input for all panes while it runs. Budget accordingly.

### 3.3 Terminal-state format variables

Cheap O(1) reads, usable as gates before expensive work:

| Variable | Meaning | Use |
|---|---|---|
| `alternate_on` | pane is in alternate screen | primary gate: is a full-screen app running at all |
| `scroll_region_upper` / `scroll_region_lower` | DECSTBM margins | `lower != pane_height - 1` ⇒ app reserves a status/ruler row |
| `cursor_x` / `cursor_y` | cursor position | strong insert-mode signal; a prompt parks the cursor on the prompt row |
| `cursor_character` | char under cursor | tiebreaker |
| `cursor_flag` | DECTCEM, cursor visible | many TUIs hide the cursor outside prompts — **high-value insert-mode signal** |
| `cursor_shape` | DECSCUSR style: `block` / `underline` / `bar` / `default` | useful corroborating signal, **absent on default-configured vim** — see below |
| `cursor_very_visible` | cursor blink state | secondary |
| `cursor_colour` | OSC 12 cursor colour | rarely set; bonus signal only |
| `history_size` | scrollback lines | frozen while in alternate screen |
| `pane_unseen_changes` | changed since last viewed | poll scheduling |
| `pane_title` | OSC 0/2 | free when set, absent otherwise |
| `alternate_saved_x` / `alternate_saved_y` | cursor saved at `smcup` | weak |

**On `cursor_shape`.** tmux parses DECSCUSR (`ESC [ n q`) per pane and exposes the resulting style as a plain string. Neovim's default `guicursor` is `n-v-c-sm:block,i-ci-ve:ver25,r-cr-o:hor20`, so neovim reports `block` in normal/visual/command, `bar` in insert, `underline` in replace — an O(1) read that crosses SSH, because DECSCUSR is just bytes in the stream.

**This is a corroborating signal, not a primary one.** Treat it as one input among several:
- **Plain vim does not emit DECSCUSR at all by default.** It needs `t_SI` / `t_EI` set. A freshly provisioned box running stock `vim` — a very common case, and one of the main reasons this project exists — will report `default` in every mode. Any spec that treats cursor shape as primary silently fails on exactly the hosts that matter most.
- **Shape cannot separate normal from visual from command** — all three are `block`.
- **Nested-tmux behaviour is unverified.** tmux propagates the active pane's cursor style to its terminal, so it may pass through intact. Test it; do not assume.

The rule: cursor shape **raises confidence** in a mode already indicated by content, and can **veto** a reading that contradicts it. It must never be the sole basis for a transition.

Skip `origin_flag`, `wrap_flag`, `insert_flag`, `keypad_flag`, `keypad_cursor_flag` — effectively constant in practice. Skip the mouse flags entirely: on an unconfigured remote they are always off, and under a nested tmux they reflect the inner tmux's own `mouse` setting rather than the application's.

### 3.4 One-shot key tables (for the literal-key escape)

Before dispatching a binding, `server-client.c` calls `server_client_set_key_table(c, NULL)` — resetting the client to the default table — *unless* the binding is a repeat. The binding body then executes.

Consequence: `switch-client -T <table>` inside a binding takes effect for **exactly one subsequent keypress**, after which the client returns to the default table on its own. No cleanup binding needed. This is the correct mechanism for the literal-key leader in §5.2, and the only place in this project where `switch-client -T` is right rather than a trap.

Caveat: a key with no binding in that one-shot table falls back to the default table and is then matched *there* — so it would get remapped, defeating the escape. The literal table must therefore bind **every** key that the normal-mode map remaps, explicitly. Generate those bindings from the spec's `[keys]` table rather than hand-writing them.

### 3.5 Colour and attribute capture

- `capture-pane -p -e` includes SGR escape sequences for foreground, background and attributes. This is the **only** way to fingerprint on colour.
- `#{C:...}` cannot see colour. `window_pane_search` builds each line via `grid_view_string_cells`, which yields plain text. Colour matching is therefore **tier-2 only** — it can never be part of a cheap gate.
- There is no format variable exposing the colour of a given cell. `pane_fg` / `pane_bg` are pane-level defaults, not cell attributes.
- SGR is **stateful across a line**. Do not regex over raw escape output. Parse the captured stream into a cell grid carrying `(char, fg, bg, attrs)` and match against that structure.
- **Normalise the colour space before comparing.** The same visual colour arrives as `38;5;N` (256-colour) or `38;2;R;G;B` (truecolour) depending on what the emitting application believes about its terminal. Over SSH into a nested tmux, that belief is set by the remote `TERM` and the inner tmux's `terminal-overrides` — outside our control. Convert everything to RGB and compare with a tolerance, and allow specs to name a palette index *or* an RGB value *or* a named colour.
- **Colour is a supporting signal, never a required one.** Users retheme their tools; the "iconic" default green of a tmux status bar is only the default. Specs must be able to mark a colour rule as `weight = "bonus"` so that its absence cannot cause a match failure.

### 3.6 Hooks

Confirmed to exist: `pane-focus-in`, `pane-focus-out`, `pane-activity`, `pane-title-changed`, `pane-command-started`, `pane-command-finished`, `pane-created`, `pane-died`, `pane-exited`, `pane-resized`, `pane-mode-changed`, `pane-shell-prompt`, `alert-activity`, `alert-silence`, `client-active`, `client-attached`, `client-detached`, `client-resized`, plus `after-<command>` hooks.

**There is no pane-output hook.** Polling is unavoidable — but `pane-activity` (with `monitor-activity`) can gate it so idle panes cost nothing. Use `alert-silence` / `monitor-silence` to back off.

`pane-command-started` / `pane-command-finished` fire on local foreground-process changes. Useful for the local fast path and for detecting when `ssh` itself starts/exits. Useless for remote app identity.

### 3.7 Status line

- Per-pane user options: `tmux set -p -t <pane> @modal_mode normal`, read back as `#{@modal_mode}`.
- **Keep all work out of format strings on the hot path.** Set `status-interval 0` to kill timed redraws, and push updates with `refresh-client -S` only when the daemon has news. `pane-border-format` is evaluated per pane per redraw, and redraws fire on output activity — an unguarded `#{C/r:}` there means one regex compile and full grid scan per pane per redraw.

### 3.8 Nested remote tmux

If the user runs tmux on the remote too, the inner tmux is a terminal emulator and **swallows the inner application's escape state**:

- `alternate_on` latches to 1 when the inner tmux attaches and never toggles again. Remote vim's own `smcup` never reaches us.
- Mouse/keypad flags reflect the inner tmux, not the app.
- `capture-pane` and `#{C:}` still work, because both read rendered cells.

Cheap nested-tmux detection (all O(1)):
- `alternate_on` high continuously since the pane's command became `ssh`, with **zero** toggles. Track toggle count in a pane option.
- `history_size` frozen at its pre-SSH value.
- `scroll_region_lower != pane_height - 1` persistently. (tmux's `tty_region` caches the region and never resets it to full height after an operation, so it persists.)
- `pane_title` matching the shape `*:*:* - "*"` — that is tmux's default `set-titles-string` (`#S:#I:#W - "#T" #{session_alerts}`). `set-titles` defaults **off**, so treat this as a bonus, never a requirement.

**Nested tmux must not break the plugin.** Worst acceptable outcome is falling back to `unknown` mode (pass-through, no remapping). Silently remapping keys in the wrong context is the one unacceptable failure.

---

## 4. Architecture

```
tmux server
  └─ modal daemon (one per tmux server, NOT per pane)
       ├─ scheduler      — decides which panes to examine and when
       ├─ gate           — O(1) format reads, no capture
       ├─ fingerprinter  — capture-pane + spec matching
       ├─ state store    — per-pane @options
       └─ dispatcher     — key-table sync, status refresh, user hooks
```

### 4.1 Tiered detection

Never capture unconditionally. Three tiers, each only entered if the previous one says "maybe":

**Tier 0 — event gate (free).** Skip any pane with no `pane-activity` since last examination, unless its mode is unknown.

**Tier 1 — format gate (microseconds, no capture).** One `display-message -p` with a packed format string reading `alternate_on`, `cursor_flag`, `cursor_x`, `cursor_y`, `scroll_region_lower`, `pane_height`, `history_size`, `pane_current_command`, `@modal_*`. If `alternate_on` is 0 and the local command is a known shell, the answer is `none` — stop.

**Tier 2 — content fingerprint (milliseconds).** `capture-pane -p` once, match against the active spec. One capture serves both app identification and mode detection — never capture twice in a cycle.

Batch tier-1 reads across panes into a **single** `tmux display-message -p` invocation per cycle where possible; per-pane client spawns dominate the cost.

### 4.2 Sticky identity

App identity is stable; mode is volatile. Once a pane is identified as `htop`, re-run full identification only on a cheap invalidation signal (`alternate_on` toggled, `pane-command-finished`, pane resized, N consecutive mode-match failures). Mode detection runs every cycle but only needs the bottom few rows.

### 4.3 State machine per pane

**Mode is an open vocabulary, not a boolean.** A spec may report any of:

| Mode | Meaning |
|---|---|
| `none` | no TUI running in the pane |
| `insert` | keystrokes are being consumed as text |
| `normal` | navigable, keystrokes are commands |
| `visual` | selection-extending submode |
| `command` | a command line is open (vim `:`, k9s `:`) — text entry, but distinct from `insert` |
| `replace` | overtype submode |
| `unknown` | a TUI is running but no spec matched |

**Every mode is assigned to one of two buckets**, and the bucket is what subscribers actually consume:

| Bucket | Meaning |
|---|---|
| `typing` | keystrokes become text |
| `commanding` | everything else — normal, visual, visual-block, operator-pending |

`none` and `unknown` are in neither and are reported as-is.

The bucket is what an outer keyboard layer wants: it does not care whether you are in visual-block or operator-pending, only whether you are typing. So the hook exports **both** — `MODAL_TYPING` as a plain `0`/`1` so the common subscriber is a one-line comparison, and `MODAL_MODE_TO` alongside for anything that wants detail (the status line does).

**Bucket assignment is declared per spec, never inferred from the mode name.** It is app-specific: vim's `command` mode is `typing`; htop's `Filter:` prompt is `typing`; a k9s `:` command is `typing`; a vim operator-pending is `commanding`. A spec that omits an assignment for a mode it declares is a load-time error, not a default.

The two consumers then read this differently:

- **The key remapper** uses the bucket: remap only when the bucket is `commanding`. `unknown` behaves exactly like `none` — full pass-through.
- **The hook consumer** gets bucket and mode both, and picks.

Specs declare which modes they can distinguish (`modes = ["normal", "insert"]`); the daemon never reports a mode a spec did not declare.

Transitions are driven by the daemon. Text-entry modes must be **entered eagerly and left conservatively**: a false `normal` steals the user's keystrokes, which is far worse than a false `insert`, where they merely lose the remapping. Require a confident negative before leaving a text-entry mode.

---

### 4.4 Detecting normal mode by absence — read this before writing any vim-family spec

Most text-entry modes announce themselves. Normal mode does not. Vim's `showmode` writes `-- INSERT --`, `-- VISUAL BLOCK --`, `-- REPLACE --` and so on to the last line; in normal mode that line holds the ruler, a stale message, or nothing at all. **Normal mode is the absence of a marker.**

That creates a trap: *a failed capture also looks like absence.* A mid-redraw screen, a resize, a dropped poll, a spec that stopped matching — all produce "no marker found," and a naive classifier reports `normal` for every one of them. For the outer-keyboard-layer use case that means the layer flips off at random moments, which is worse than it never having worked.

**Three rules follow, and they are not optional.**

1. **Never treat `normal` as a fallback.** It must be an *affirmative* conclusion: app identity re-confirmed **in the same capture** that showed no mode marker. If the identity rules do not fire on that capture, the answer is `unknown`, not `normal`. Cheap identity anchors for vim: the `~` column, the ruler block, a known statusline. Costs nothing extra — it is the same captured buffer.

2. **Transitions are asymmetric, and this reverses the §4.3 advice for hooks-only specs.** Entering a text-entry mode is a *positive* edge (a marker appears) and is trustworthy on first sight. Leaving is a *negative* edge (the marker vanishes) and is inferred. For key-remapping specs, keep leaving conservative — the cost of a wrong `normal` is stolen keystrokes. For hooks-only specs **nothing is remapped, so no keystroke can be stolen**, the costs are symmetric, and vim clears the marker immediately on `Esc` — so the negative edge should be acted on promptly, with debouncing as the only guard against mid-redraw races.

3. **Corroborate with free tier-1 signals before concluding.** `cursor_y` on the last row indicates command-line mode; `cursor_y` in the body contradicts it. `cursor_shape` vetoes a `normal` reading if it says `bar`. Both are already in the batched read.

**`showmode` fails in ways the spec must handle explicitly:**
- **Localisation.** Under a non-English locale vim translates the marker — German gives `-- EINFÜGEN --`. This directly undercuts the just-provisioned-box premise, since those boxes inherit whatever `LANG` the SSH session forwards. Specs need an alternates list, and `validate` should warn when a spec ships English-only markers.
- **`showmode` is off** in most configs that use lualine/airline/powerline, which is most configured users.
- **`cmdheight=0`** (neovim 0.8+) removes the message line entirely; there is nowhere for the marker to appear.
- **Completion popups** overwrite the marker with `-- Keyword completion (^N^P) --` and similar. Match the `^-- ` prefix plus a mode word, not whole fixed strings.
- **`cmdheight > 1`** shifts the marker up off the last row. Use a `rows` range, never `-1` alone.

When none of the above can be established for a host, the correct behaviour is to report `unknown` and let the subscriber do nothing. Document that plainly rather than degrading to guesswork.

## 5. Configuration

### 5.1 Global options

```tmux
set -g @modal_enabled            'on'
set -g @modal_profile            'balanced' # frugal | balanced | snappy | custom
set -g @modal_poll_interval      '150'    # ms between cycles
set -g @modal_idle_interval      '2000'   # ms when no pane has activity
set -g @modal_burst_interval     '30'     # ms while a pane is in a burst (see §5.1.1)
set -g @modal_burst_decay        '1200'   # ms of quiet before leaving burst mode
set -g @modal_scope              'active' # active | visible | all
set -g @modal_capture_rows       '6'      # rows from bottom for mode detection
set -g @modal_cpu_budget         '2'      # % of one core; daemon self-throttles
set -g @modal_spec_paths         '~/.config/tmux-modal/specs'
set -g @modal_indicator_format   '#[fg=green]● NORMAL#[default]'
set -g @modal_escape_leader      '_'      # literal-key leader; spec overrides
set -g @modal_color_matching      'on'     # off = skip -e capture entirely
set -g @modal_transition_hook    ''       # shell command, see §8
set -g @modal_log_level          'warn'
```

Per-window and per-pane overrides via `set -w` / `set -p` of `@modal_enabled` — users must be able to kill it for one pane without disabling globally.

#### 5.1.1 The latency/cost tradeoff is the user-facing knob

Detection lag is the difference between this being pleasant and being irritating: at 150 ms, a subscriber reacting to `Esc` is wrong for long enough to mangle the first keystroke or two of a normal-mode command. But a uniformly fast poll is exactly how the CPU budget gets blown.

**Adaptive polling resolves this and should be the default strategy.** A pane that just produced output is a pane where the user is doing something; poll it at `@modal_burst_interval` until `@modal_burst_decay` of quiet elapses, then fall back. Idle panes cost nothing, active panes get near-instant detection, and the average stays low. This beats raising the global rate in both dimensions — implement it before reaching for a faster uniform interval.

`@modal_profile` presets the underlying numbers so users tune one knob, not six:

| Profile | Burst | Poll | Idle | Intended for |
|---|---|---|---|---|
| `frugal` | 100 | 500 | 5000 | battery, many panes, remote-heavy |
| `balanced` | 30 | 150 | 2000 | default |
| `snappy` | 15 | 60 | 1000 | hooks-driven workflows where lag is felt |

Any individually set option wins over its profile value and flips the profile to `custom`. Document the measured CPU cost of each profile at 5 and 20 panes in the README — a tradeoff knob whose cost is undocumented is not a tradeoff knob.

**The CPU budget knob must actually work.** The daemon measures its own cycle cost and lengthens the interval to stay inside the budget, logging when it does. A knob that only sets a timer is not a budget.

### 5.2 App spec format

TOML, one file per app, loaded from `@modal_spec_paths` (user dir shadows bundled specs by filename).

An **app** is: an identity predicate, plus a mode classifier, plus optional per-consumer configuration. The first two are the reusable parts, which is what groups exist to share.

```toml
name = "htop"
priority = 50                     # higher wins when several match
extends = "curses-tui"            # optional; see §5.3
modes = ["normal", "insert"]      # modes this spec can distinguish

# --- identification -------------------------------------------------
[match]
command = ["htop", "btop"]        # local fast path; optional
title   = '^htop'                 # regex against pane_title; optional
requires_alt_screen = true
screen_mode = "all"               # all | any

  [[match.screen]]
  regex  = '^\s*PID\s+USER\s+PRI\s+NI'
  rows   = [0, 12]                # search only rows 0-12 from the top
  anchor = "start"                # start | end | anywhere (default anywhere)

  [[match.screen]]
  regex = 'Mem\['
  rows  = [0, 6]

# --- mode detection -------------------------------------------------
# Evaluated top to bottom, first hit wins. No hit => normal.
[[insert_when]]
name   = "search"
regex  = '^Search:'
rows   = [-2, -1]                 # last two rows; safer than a single row
anchor = "start"
col    = 0                        # match must begin in column 0

[[insert_when]]
name   = "filter"
regex  = '^Filter:'
rows   = [-2, -1]
anchor = "start"
col    = 0

  # Optional colour corroboration. weight = "bonus" means its absence
  # cannot fail the rule; weight = "required" means it can.
  [[insert_when.color]]
  weight = "bonus"
  row    = -1
  col    = 0
  bg     = "cyan"                 # named | palette index | "#RRGGBB"
  tolerance = 24                  # RGB distance; ignored for named/indexed

[[insert_when]]
name = "cursor-on-prompt"
cursor_rows = [-2, -1]
cursor_visible = true

# --- key map (normal mode only) -------------------------------------
[keys]
h = "Left"
j = "Down"
k = "Up"
l = "Right"
g = "Home"
G = "End"
"C-d" = "PageDown"
"C-u" = "PageUp"

# --- literal-key escape ---------------------------------------------
# leader + <key> sends <key> verbatim to the pane. Two-key sequence,
# not a modifier. leader + leader sends the leader itself.
[escape]
leader = "_"

# Documentation only — drives README generation and `validate` warnings.
# Does not affect runtime behaviour.
[shadowed]
h = "help"                        # htop binds h to help
k = "kill"
```

Requirements:

- Regexes are POSIX ERE, matched against the rendered line with trailing whitespace stripped.
- **Row addressing:** `rows = [a, b]` is an inclusive range. Non-negative indices count from the top, negative from the bottom (`-1` = last row). A bare `row = n` is sugar for `rows = [n, n]`. Ranges are the recommended form — a single row is brittle against version and layout differences.
- **Column addressing:** `col = n` requires the match to begin at that column. `cols = [a, b]` requires it to begin within the range. Omitted means anywhere on the line.
- **`anchor`:** `start` implies `col = 0` unless `col` is given explicitly; `end` anchors to the line's last non-blank cell; `anywhere` is the default.
- **Occurrence counting:** `min_occurrences` / `max_occurrences` count how many rows *within the range* satisfy the clause. `contiguous = true` additionally requires those rows to form an unbroken run. **Any clause using these forces tier 2** — `#{C:}` early-exits on its first match and structurally cannot count (§3.2), so a counting clause can never appear in a cheap gate.
- **Geometry is mandatory for weak patterns.** A bare `Filter:` will false-positive against any pane containing that word — a log file, a man page, this spec. Specs whose rules combine to fewer than roughly 12 anchored characters should be flagged by `validate` as under-specified.
- **Colour rules** (`[[match.color]]`, `[[insert_when.color]]`) address a single cell by `row`/`col` and assert `fg`, `bg` and/or `attrs` (`bold`, `reverse`, `underline`). They require a tier-2 `capture-pane -pe` and the normalisation described in §3.5. `weight` defaults to `"bonus"`.
- **The leader is configurable per spec and globally** (`@modal_escape_leader`), spec winning. Pick a global default that is rare in normal-mode TUI navigation — `_` is a reasonable choice, but verify it against every bundled app before committing; `mc` and `k9s` both use punctuation keys heavily.
- Implementation of the leader: generate a `modal-literal-<app>` key table binding `send-keys` for every key appearing in `[keys]`, plus the leader itself. Bind `leader` in the `modal` table to `switch-client -T modal-literal-<app>`. See §3.4 — the one-shot reset is automatic, and the explicit bindings are required, not optional.
- A spec that fails to parse is **skipped with a logged warning**, never fatal.
- **`[keys]` is optional.** A spec with no key map is a hooks-only spec (see §6.2). When the active pane's app has no key map, the daemon must **not** set the session `key-table` option at all — not to `modal`, not to anything. Mode is still detected, published and hooked.
- **`always = "<mode>"`** short-circuits mode classification for apps with exactly one mode. `always = "insert"` is correct for `fzf`, `psql`, `weechat` and any REPL — it is meaningfully different from `unknown`, because it suppresses weaker specs that might otherwise claim the pane.
- **`extends = "<group>"`** pulls in a group spec. Merge semantics: scalar keys in the child win; `[[...]]` rule lists concatenate with **child rules first**, so a child can shadow a parent rule by matching earlier; `[keys]` merges key-by-key with the child winning, and a child may suppress an inherited binding with `h = false`. Single inheritance only — resolve chains depth-first and reject cycles at load time with a logged error.
- `tmux-modal validate <file>` lints a spec and reports which rules fired against a captured screen, **with the matched row/column and colour for each**, and **with the fully resolved spec after inheritance** so users can see what they actually inherited. It must also accept `--capture <pane>` to grab a live screen, and `--fixture <file>` to run against a saved one. This is the primary authoring tool — build it early, before any bundled specs beyond htop.

#### 5.2.1 Rules are lists of weighted clauses

A single regex is rarely a trustworthy fingerprint. Every rule — identity and mode alike — is therefore a **list of clauses**, each with its own geometry and weight, combined against a threshold. This is what lets several patterns in different places corroborate each other.

A **clause** is one matcher plus geometry plus a weight:

| Clause key | Matches |
|---|---|
| `regex` | POSIX ERE against the rendered line |
| `color` | cell attributes at `row`/`col` (§3.5) — tier 2 |
| `cursor_rows` / `cursor_cols` | cursor position |
| `cursor_shape` | DECSCUSR style (§3.3) |
| `negate = true` | inverts any of the above |

A rule fires when the summed weight of satisfied clauses reaches `threshold`. `weight` defaults to 100; `combine = "all"` is sugar for a threshold equal to the sum of all weights, `"any"` for a threshold equal to the smallest.

```toml
[match]
combine   = "weighted"
threshold = 100

  [[match.clause]]                # tildes: strong but not always present
  regex           = '^~$'         # exactly a tilde, NOT '^~'
  rows            = [1, -3]
  col             = 0
  min_occurrences = 3
  contiguous      = true
  weight          = 70

  [[match.clause]]                # ruler block, bottom right
  regex  = '[0-9]+,[0-9-]+ +(All|Top|Bot|[0-9]+%)'
  rows   = [-2, -1]
  anchor = "end"
  weight = 50

  [[match.clause]]                # open-file message
  regex  = '^".*" +([0-9]+L|\[New\]|\[No Name\])'
  rows   = [-2, -1]
  col    = 0
  weight = 40
```

Any two of those three clear the threshold. That is the point: no single anchor has to be universal.

**Worked reasoning on the tilde clause**, since it is the obvious vim fingerprint and also a good example of why weighting is necessary:

- Match `^~$` **exactly**, after stripping trailing whitespace. A bare `^~` false-positives on markdown `~~~` fences, on `~/path` in shell output, and on this document.
- Tildes are contiguous and anchored to the bottom of the text area, so `contiguous = true` plus a bottom-weighted range is far stronger than a scattered count.
- **They vanish on a full file.** A buffer with no empty space below the text shows zero tildes — and reading a long file is a completely ordinary thing to be doing.
- **They vanish on many neovim setups**, which set `fillchars` `eob` to a space.

So tildes are worth a high weight and must never be `required`. This is exactly the failure that would otherwise cause identity confirmation to fail, which under the §4.4 rule turns every `normal` into `unknown` and silently switches the whole feature off for that pane.

**Mode rules use the same structure.** This closes a gap a single-regex form cannot express:

```toml
[[mode_when]]
mode      = "insert"
combine   = "weighted"
threshold = 100

  [[mode_when.clause]]
  regex  = '^-- (INSERT|EINFÜGEN)\b'
  rows   = [-3, -1]
  col    = 0
  weight = 100                    # marker alone is sufficient

  [[mode_when.clause]]
  cursor_shape = "bar"
  weight       = 60               # corroborating, insufficient alone

  [[mode_when.clause]]
  negate = true
  regex  = '^:'                   # not the command line
  rows   = [-1, -1]
  col    = 0
  weight = 40
```

**`validate` must print the per-clause breakdown** — which clauses fired, their weights, the running total and the threshold. Authoring a spec against an opaque boolean is guesswork; against a score sheet it is tractable. Treat this output as a first-class feature, not debug logging.

**Scheduling consequence:** the tier a rule needs is derived from its clauses, not declared. A rule of pure `cursor_*`/`cursor_shape` clauses is tier 1. Any `regex`, `color` or occurrence-counting clause forces tier 2. Compute this at load time and store it on the resolved rule so the scheduler never has to introspect clauses on the hot path.

### 5.3 Groups

Ship these as `specs/groups/`. They are not matchable on their own — they carry no `[match]` identity, only reusable rules and key maps.

| Group | Provides |
|---|---|
| `curses-tui` | `requires_alt_screen`, cursor-hidden-outside-prompt rule, baseline `hjkl` map |
| `slash-search` | `/` and `?` bottom-line prompt rules — covers `less`, `man`, `tig`, `ranger`, `k9s` |
| `colon-command` | `:` command-line rule reporting mode `command` |
| `readline-repl` | `always = "insert"`, no key map |
| `vim-family` | cursor-shape mode rules + `showmode` fallback; **no key map** |

### 5.4 Ship with specs for

**Key-remapping specs** (navigation TUIs that lack vim keys):

| App | Extends | Insert-mode triggers | Notes |
|---|---|---|---|
| `htop` | `curses-tui`, `slash-search` | `Search:`, `Filter:` | reference spec, §6.1 |
| `btop` | `curses-tui` | filter prompt | layout differs substantially from htop |
| `less` | `slash-search` | `/`, `?`, `-` option prompt | also `ESC` cancels |
| `man` | `less` | inherits | thin spec over `less` |
| `tig` | `slash-search`, `colon-command` | `/`, `:` | |
| `k9s` | `slash-search`, `colon-command` | `:`, `/` | very distinctive header block — good identity anchor |
| `lazygit` | `curses-tui` | commit-message and filter panels | panel-based; needs geometry per panel |
| `ranger` / `lf` / `nnn` | `slash-search`, `colon-command` | `/`, `:`, rename prompt | rename prompt is easy to miss — test it |
| `ncdu` | `curses-tui` | delete confirmation | few text prompts; mostly always-normal |
| `mc` | `curses-tui` | **always-visible bottom command line** | hardest of the set. The input line is always present, so presence alone proves nothing — must detect focus. Consider shipping it as `always = "insert"` rather than shipping something wrong |

**Hooks-only specs** (no key map — detection exists purely to drive `@modal_transition_hook`):

| App | Extends | How mode is detected |
|---|---|---|
| `nvim` | `vim-family` | `cursor_shape` (tier-1, no capture); `-- INSERT --` fallback |
| `vim` | `vim-family` | same, but **must not assume DECSCUSR** — see §3.3 |
| `emacs -nw` | — | minibuffer activity; low confidence, mark experimental |

**Always-insert specs** (exist to suppress weaker matches, never to remap):

`fzf`, `psql`, `mysql`, `sqlite3`, `gdb`, `lldb`, `weechat`, `irssi`, `python`, `node`, `irb`.

Each key-remapping spec needs a genuine insert-mode rule, not a stub. Where a real rule cannot be established with confidence, ship `always = "insert"` instead — a spec that never remaps is strictly better than one that remaps at the wrong moment.

---

## 6. Worked examples

### 6.1 htop — the key-remapping case

Verify all of this empirically before encoding it; htop's layout varies by version and config.

- `/` opens a `Search:` prompt on the last row. `\` opens `Filter:`. Both are insert mode. Exit on `Enter` or `Esc`.
- `F9` (kill) opens a **signal list panel** — navigable, so this stays **normal** mode.
- `F2` (setup) is a full-screen navigable menu — **normal** mode, though the key map may want to differ. Consider per-submode key maps if this proves necessary; do not build that until htop forces it.
- `u` (user filter) opens a selectable list — normal mode.
- **Conflict:** htop binds lowercase `h` to help and `k` to kill. Remapping them shadows both. The `[escape]` leader is the answer: `_h` sends a literal `h`, `_k` a literal `k`. Document every shadowed key in the README; do not silently steal bindings.

**Geometry to establish empirically.** `Filter:` and `Search:` appear at the bottom, but confirm whether that is the final row or the second-to-last, and whether it shifts when htop's header is resized or a function-key bar is hidden. Capture fixtures for: default layout, `F2` setup open, header rows reduced to 1, and a 24-row window. Encode the union as a `rows` range, not a guess. A bare unanchored `Filter:` is far too weak — it matches any pane displaying that word.

**Colour to check.** htop draws the search/filter prompt label with a distinct attribute in most themes. Capture it with `capture-pane -pe` and, if it is stable across the default themes, add it as a `weight = "bonus"` rule. Do not make it required — htop is heavily rethemed.

---

### 6.2 vim / neovim — the hooks-only case

No key remapping. `hjkl` already works; touching it would be actively harmful. This spec exists so that an outer keyboard layer can be told which mode vim is in.

```toml
name = "vim"
priority = 60
extends = "vim-family"
modes = ["normal", "insert", "visual", "command", "replace"]

[buckets]                         # mandatory: no mode may be unassigned
typing      = ["insert", "command", "replace"]
commanding  = ["normal", "visual"]

[match]
command = ["vim", "nvim", "vi"]   # local fast path only; useless over SSH
requires_alt_screen = true

  # Identity anchors. These are re-checked on EVERY capture, because
  # concluding `normal` requires confirmed identity — see §4.4.
  [[match.screen]]
  regex = '^~$'
  rows  = [1, -3]
  col   = 0
  min_occurrences = 2             # one stray tilde is not vim

# --- mode rules, evaluated in order, first hit wins ------------------
# PRIMARY: showmode markers. Prefix-matched, not whole-string, so that
# completion suffixes ("-- INSERT --  (lang)") still match.
[[mode_when]]
mode      = "insert"
regex     = '^-- (INSERT|EINFÜGEN|INSERTION|INSERIMENTO)\b'
rows      = [-3, -1]              # cmdheight > 1 shifts this up
col       = 0
alternates_note = "add locale variants; see validate --locale-audit"

[[mode_when]]
mode  = "replace"
regex = '^-- (REPLACE|VREPLACE|ERSETZEN)\b'
rows  = [-3, -1]
col   = 0

[[mode_when]]
mode  = "visual"
regex = '^-- \(?(VISUAL|SELECT|VISUELL|AUSWAHL)\b'
rows  = [-3, -1]
col   = 0

[[mode_when]]
mode        = "command"
regex       = '^[:/?]'
rows        = [-1, -1]
col         = 0
cursor_rows = [-1, -1]            # corroboration: cursor must be there

# CORROBORATION: only consulted when no marker matched above. Vetoes a
# `normal` conclusion; never concludes a mode on its own.
[[corroborate]]
if_cursor_shape = "bar"
veto_modes      = ["normal", "visual"]
then_mode       = "insert"
confidence      = "low"           # emitted, but flagged in the hook

# FALLBACK: normal is an AFFIRMATIVE conclusion, not a default.
# Requires [match.screen] to have fired on this same capture.
[default_mode]
mode = "normal"
requires_identity_confirmed = true
otherwise = "unknown"
```

**Why this spec is shaped the way it is.** Default-configured vim on a fresh box is the target, and on that host `showmode` is on, cursor shape never changes, and `-- INSERT --` is the only thing you get. So the marker rules are primary and cursor shape is demoted to corroboration. On neovim the corroboration rule adds real value; on stock vim it silently contributes nothing, and the spec still works. That ordering is deliberate — do not invert it.

**Cost.** The marker rules need a capture, so vim panes are tier-2, unlike what an earlier draft of this document assumed. Two mitigations: capture only the bottom `@modal_capture_rows` for mode classification once identity is sticky (§4.2), and lean on adaptive burst polling (§5.1.1) rather than a fast uniform interval. A per-spec `poll_interval` override remains available but should be the last resort, not the first.

**Known gaps — document every one of these in the README:**
- `showmode` is off in most configured setups (lualine/airline users). For those, this spec degrades to `unknown`, which is correct behaviour and must be stated rather than papered over.
- `cmdheight=0` on neovim removes the message line entirely; no marker can appear.
- Localisation: the regexes above carry a few locale variants as illustration only. Audit against the actual `LANG` values in use; `validate --locale-audit` should list which modes have no non-English alternate.
- Operator-pending and single-char `r` replace are not reliably detectable. Do not attempt them; they fall into `commanding`, which is the right bucket anyway.
- A vim terminal buffer (`:term`) is `typing` but shows no marker. Neovim shows `-- TERMINAL --`; verify, and if plain vim shows nothing, document it as a known false `commanding`.

## 7. Status line

Expose `#{@modal_mode}` (`none` | `normal` | `insert` | `unknown`) and `#{@modal_app}` per pane. Provide a ready-made snippet:

```tmux
set -g status-right '#{?#{@modal_mode},#{@modal_indicator_format} #{@modal_app} ,}%H:%M'
```

The format string must contain **only variable reads**. All computation happens in the daemon. Document `status-interval 0` + `refresh-client -S` in the README and explain why.

---

## 8. Transition hook

This is a first-class consumer, not a nicety — for the `vim-family` specs it is the *only* consumer. Treat it accordingly.

On every mode change, if a hook is configured, run it detached with these in the environment:

```
MODAL_TYPING         0 | 1        <- the one most subscribers need
MODAL_BUCKET         typing | commanding | none | unknown
MODAL_MODE_TO        insert | normal | visual | command | replace | ...
MODAL_MODE_FROM
MODAL_CONFIDENCE     high | low
MODAL_PANE, MODAL_APP, MODAL_SESSION, MODAL_WINDOW,
MODAL_PANE_ACTIVE, MODAL_TIMESTAMP_MS
```

- **`MODAL_TYPING` is the primary output.** An outer keyboard layer only cares whether the user is typing text; make that a one-line comparison rather than a mode-name whitelist the subscriber has to maintain. `MODAL_MODE_TO` is there for subscribers that want detail, and the status line is one.
- `MODAL_CONFIDENCE` is `low` when the mode came from a corroboration rule rather than a positive marker match. Subscribers that prefer to do nothing rather than the wrong thing can gate on it.
- Hooks are configurable **globally** (`@modal_transition_hook`) and **per spec** (`hook = "..."`), with the per-spec hook running after the global one.
- Never block the daemon. Time it out (`@modal_hook_timeout`, default 500 ms). If the previous invocation for the same pane is still running, **drop the older one and run the newer** — mode state is edge-triggered and a stale transition is worse than a missed one.
- **Coalesce rapid transitions.** Holding `i` then `Esc` repeatedly must not spawn a process per keystroke. Debounce with a configurable floor (`@modal_hook_debounce`, default 30 ms), emitting the final state.
- Arbitrary commands, including paths outside the tmux host's own environment. Where the tmux server and the subscriber live in different environments — a WSL-hosted tmux signalling a keyboard layer on the Windows side, for instance — crossing that boundary is the **user's** problem, but its latency is **ours**: process spawn across that boundary can cost tens of milliseconds. Document that a long-lived listener fed by a one-line hook will always beat spawning a heavyweight process per transition, and ship the README example that way rather than as a naive direct invocation.
- Ship two README examples: `notify-send` / `terminal-notifier` for the simple case, and a write-to-FIFO/socket pattern for the low-latency case.

---

## 9. Testing

tmux is fully headless-scriptable and this is your iteration loop. Use it aggressively.

```bash
SOCK=modal-test-$$
tmux -L "$SOCK" -f /dev/null new-session -d -x 200 -y 50 'htop'
tmux -L "$SOCK" send-keys '/'
sleep 0.2
tmux -L "$SOCK" capture-pane -p
tmux -L "$SOCK" kill-server
```

Always use a dedicated `-L` socket and `-f /dev/null` so tests never touch the developer's real config or server. Always set `-x`/`-y` explicitly; layout-dependent fingerprints break on the 80×24 default.

**Three test tiers:**

1. **Golden fixtures (fast, no tmux).** Capture real screens once into `tests/fixtures/<app>/<state>.txt`, then unit-test the matcher against files. This is the inner loop and should run in well under a second.
2. **Integration (real tmux, real apps).** Drive genuine `htop`/`less` in a headless server, assert the detected mode after scripted keystrokes. Must cover: entering search, typing in search, cancelling with `Esc`, confirming with `Enter`, resizing mid-search.
3. **SSH and nested-tmux (containerised).** A container with sshd; run the app over `ssh localhost`, and again under a remote tmux. Assert correct detection in the plain-SSH case and graceful `unknown` fallback in the nested case. **Do not skip this tier** — it is the actual use case.

**Mandatory fixture coverage for the absence problem (§4.4).** For each `vim-family` spec, capture and assert against: normal mode with the ruler visible; normal mode immediately after a message (`"f.txt" 12L written`, `E486: Pattern not found`); insert mode; insert with a completion popup open; visual, visual-line and visual-block; command line open; `cmdheight=2`; `showmode` off; a non-English `LANG`; and a mid-redraw partial capture. The last two must assert `unknown`, **not** `normal` — a test suite that lets a degraded capture read as `normal` will not catch the failure mode this project is most likely to ship.

Add a benchmark asserting the daemon stays inside `@modal_cpu_budget` with 20 panes open, reported separately for each `@modal_profile`.

---

## 10. Repo and delivery

Layout must be TPM-compatible:

```
tmux-modal/
├── modal.tmux              # TPM entry point
├── scripts/
├── specs/                  # bundled app specs
├── tests/
│   ├── fixtures/
│   └── integration/
├── .github/workflows/ci.yml
├── README.md
└── LICENSE                 # MIT
```

- Implementation language: your call, but justify it in the README. Shell keeps installation frictionless and is idiomatic for tmux plugins; a compiled daemon is easier to keep inside the CPU budget. If you pick a compiled language, ship prebuilt binaries **and** a shell fallback — a plugin that requires a toolchain to install will not get used.
- README must cover: install, the full option list, spec authoring with a worked example, the shadowed-keys caveat, the nested-tmux limitation, and how to debug with `tmux-modal validate`.
- Conventional commits, meaningful history — do not squash the whole build into one commit.
- CI green before pushing.
(Publishing is §10.1 — it is a required deliverable, not an optional last step.)

---

### 10.1 Publishing to GitHub — required

The GitHub CLI (`gh`) is installed and **already authenticated**. Use it. Do not ask for credentials, do not print manual git remote instructions, and do not stop at "the code is ready to push."

1. `git init`, sensible `.gitignore`, MIT `LICENSE`.
2. Commit incrementally as you work — conventional commits, one per milestone at minimum. Do not squash the whole build into a single commit.
3. Verify the full test suite passes locally **before** creating the repo.
4. `gh repo create tmux-modal --public --source=. --remote=origin --description "..."` then push.
5. Confirm CI is green on the pushed commit with `gh run list` / `gh run watch`. **A red CI badge on a fresh repo is a failed delivery** — fix and push again.
6. Add repo topics with `gh repo edit --add-topic tmux,tmux-plugin,vim,tui`.
7. Report the repository URL and the CI status as your final output.

If any step fails — auth expired, name taken, CI red for an environmental reason you cannot fix — say so explicitly and state exactly where you stopped. Do not report success for a partial push.

## 11. Non-goals and pitfalls

- **Do not** require any remote-host configuration. No shell hooks, no `precmd`, no title-setting, no installed agent. This constraint is the entire point.
- **Do not** attempt remote process introspection over SSH `ControlMaster` as the primary mechanism. It is a legitimate optional enhancement behind a default-off flag, but it is not the design.
- **Do not** put `#{C:}` or any computation in `status-*` or `pane-border-format` on the hot path.
- **Do not** use `switch-client -T` to enter the modal table (see §3.1).
- **Do not** write keys into a pane to probe it. Anything sent to a pane arrives as keystrokes in the remote application.
- **Do not** let a spec parse error, a missing app, or a nested tmux break key handling. Failure mode is always pass-through.
- **Do not** assume `pane_current_command` means anything for an SSH pane beyond `ssh`.
- **Do not** make any colour rule load-bearing by default, and do not regex over raw `capture-pane -e` output (see §3.5).
- **Do not** ship a spec whose screen rules are unanchored substrings. Geometry is part of the fingerprint, not an optimisation.
- **Do not** remap keys for any `vim-family` spec, and do not set the session `key-table` option for a pane whose app has no key map.
- **Do not** conclude `normal` (or any bucket) from the absence of a marker without re-confirming app identity on the same capture. See §4.4 — this is the single most likely way for this project to ship something that misbehaves at random.
- **Do not** treat `cursor_shape` as a primary mode signal. Stock vim never changes it.
- **Do not** make subscribers reconstruct the typing/commanding split from mode names. Export `MODAL_TYPING` directly.
- **Do not** mark any single screen pattern as required when a weighted set of clauses would do. The vim tilde column is the cautionary case: obvious, strong, and absent whenever the file happens to fill the window.
- **Do not** put an occurrence-counting or colour clause in a tier-1 gate. It cannot work there (§3.2, §5.2.1).
- **Do not** ship a key-remapping spec whose insert-mode rule you could not verify empirically. Ship `always = "insert"` instead.

---

## 12. Milestones

1. Daemon skeleton, per-pane state store, tier-0/1 gating, `none`/`unknown` only. No remapping yet.
2. Spec loader + matcher + `validate` subcommand + golden-fixture tests.
3. htop spec end to end, local only. Key-table sync via the session `key-table` option, `pane-focus-in` reconciliation.
4. Status indicator and transition hook.
5. SSH integration tests; nested-tmux fallback.
6. Remaining bundled specs, CPU-budget self-throttling, benchmark.
7. `vim-family` specs and the hooks-only path, including the per-spec `poll_interval` override.
8. README, CI, and **publish to GitHub per §10.1** — repo created, pushed, CI green, URL reported.

Stop and report at the end of milestone 3 — that is the point where the core design is either proven or not, and it is worth a human look before building out.

---

## 13. Open questions — verify, don't assume

- Exact htop prompt strings and row positions across versions. Capture real screens; the strings in §6 are a starting hypothesis.
- Whether `pane-activity` fires reliably enough to serve as tier 0, and what `monitor-activity` costs when enabled on every pane. Measure before committing to it.
- Whether a single batched `display-message -p` can read formats for multiple panes in one invocation, or whether one call per pane is required. This materially affects the scheduler design.
- How `cursor_flag` actually behaves for each bundled app. It is listed as high-value but is unverified per-app.
- Whether specs need per-submode key maps (htop's `F2` setup screen is the test case). Defer until forced.
- The real cost of `capture-pane -pe` versus plain `-p` on a large pane, and whether colour matching needs its own tier behind `@modal_color_matching`. Measure before enabling it by default.
- Whether `_` is a safe global leader across all bundled apps. Check `mc`, `k9s` and `tig` specifically before settling on it.
- **Whether `cursor_shape` survives a nested remote tmux.** tmux propagates the active pane's cursor style to its own terminal, so it may pass through intact — which would make the whole `vim-family` story work unchanged over nested sessions. This is the highest-value unknown in the document; test it early.
- Which distro `vim` packages set `t_SI` / `t_EI` by default. Expected answer: few enough that cursor shape stays corroborating-only. Confirm rather than assume.
- The actual `showmode` marker strings under the locales in play. `LANG` is commonly forwarded by SSH, so a fresh box inherits the client's locale and the English regexes silently stop matching.
- Whether `-- TERMINAL --` appears in neovim's terminal mode, and what plain vim shows there.
- Achievable end-to-end hook latency from keypress to subscriber, measured, not estimated. If it cannot be brought under roughly 50 ms, say so plainly in the README — the outer-keyboard-layer use case depends on it and users should know before they build on it.
