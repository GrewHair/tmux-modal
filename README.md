# tmux-modal

A tmux plugin that works out **which mode the application in each pane is in**,
even when that application runs on the far side of an SSH connection, with
**nothing installed or configured on the remote host**.

It reads the rendered screen (the same cells you see) and fingerprints the
app and its mode from text, geometry, cursor state and, optionally, colour.
Two independent things can subscribe to that:

1. **Key remapping.** Give vim-style keys to TUIs that lack them. In `htop`,
   `btop` and apt's whiptail questions, `hjkl` move the selection, but the
   moment you open a search prompt or a text field, `hjkl` are ordinary
   letters again.
2. **Transition hooks.** Run a command whenever a pane changes mode, so
   something outside tmux (AutoHotkey, kanata, karabiner, a status widget)
   can react — e.g. switch an outer keyboard layer off while vim is in
   normal mode and on while you type.

> **Status: 1.3.** Recognises [vim and neovim](#vim-and-neovim) (for the
> hook) and, through [bundled specs](#bundled-specs), htop, btop, apt's
> package questions (debconf), whiptail dialogs, less, man, tig, lazygit,
> k9s, ranger, lf, nnn, ncdu, mc, fzf and common REPLs,
> any of which [can be given keys](#add-keys-to-any-app); locally, over SSH
> and through a [remote tmux](#ssh-and-nested-tmux) (tested against a real
> sshd). Any full-screen app no spec recognises shows as `ALT ?` (N/A in
> the older single indicator) and is left completely alone.

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

### Badges

The daemon publishes a row of small **badges** per pane, one fact each, so
you can see at a glance what it concluded and why. They read as a path,
left to right:

```
ALT  VIA ssh  NEST tmux  SPLIT 3 border  htop fp 110/40  NORMAL absence lo  MAP
```

| Badge | Shown when | Default text |
|---|---|---|
| `alt` | the pane is on the alternate screen (a full-screen app runs) | `ALT` |
| `via` | the pane's command is a remote transport (below) | `VIA ssh`, `VIA docker` |
| `app` | an app is recognised: its name, how, and the fingerprint score; `?` for a full-screen app nothing recognised (left alone), followed, struck through, by the app that came closest (its fingerprint scored at least half its threshold) | `htop cmd+fp 110/40`, `?`, `?` ~~`htop 30/40`~~ |
| `mode` | an app is recognised: its mode (`?` when this capture does not show it), then how it was decided and the confidence in it (`hi`/`lo`, `lo` in red): `fp:<rule>` a mode rule's fingerprint matched (the rule's name, or its mode when it has none: `fp:search`, `fp:insert`), `absence` no mode fingerprint matched on a screen confirmed to be the app's (the default mode), `veto` a second check overruled that default (vim's cursor shape), `mem` no mode fingerprint and the app only remembered from an earlier capture, `always` the app has one mode, `policy` the nested policy made it unknown. Through a remote tmux the confidence is always `lo`. For `?` the reason is grey and has no confidence (`?` is never sure); when the nested policy dropped a mode it did read, that mode follows struck through | `NORMAL absence hi` (blue), `INSERT fp:insert hi` (green), `? mem`, `? policy` ~~`NORMAL fp:search`~~ |
| `nest` | the pane shows another multiplexer, and which one; grey and struck through when seen on screen but not in effect (no transport, below) | `NEST tmux` |
| `split` | that multiplexer's window is split: inner panes, and how the focused one was found (`border` colour, `cursor`, `?` none); struck through like `nest` | `SPLIT 3 border` |
| `map` | keys are being remapped for this pane; `MAP _` while the escape leader waits for its key (the next key goes through unchanged) | `MAP`, `MAP _` |
| `hook` | the transition hook just ran for this pane: `HOOK …` while it runs, then `HOOK ✓ insert` (exit 0, and the mode it was told), `HOOK ✗ 1` (exit status) or `HOOK ⏱` (killed at `@modal_hook_timeout`); `×3` when three transitions were merged into one call; grey and struck through when the call was for a pane you are not typing into (`MODAL_PANE_ACTIVE=0`). Gone `@modal_hook_flash` (1.5 s) after the call started, whatever it shows | `HOOK ✓ insert` |
| `cursor` | the app set a cursor shape (tmux ≥ 3.5): the cursor drawn as set, block, underline or bar | `█` `▁` `▏` |
| `attach` | the daemon just attached to this session (it starts, or you attach a terminal to a session that had none): it is running, and its version. On the focused pane, gone after `@modal_attach_flash` (2 s) | `MODAL 1.8.0` |

How the app was recognised (`app` badge, `@modal_evidence`): `cmd` the
pane's command, `title` its title, `fp` the screen fingerprint (its score
against the spec's threshold follows, e.g. `110/40`; for rules without
weights, clauses matched of all), joined with `+`; `mem` when none of them
held on this capture and the app is remembered from an earlier one.

**Transports.** A multiplexer inside a pane only makes sense on another
machine or in a container: tmux refuses to nest locally unless `TMUX` is
unset, and a local client's command is then `tmux` itself. So a remote
tmux seen on screen (its status line, borders or title) is acted on only
when the pane's command is a transport: `ssh`, `autossh`, `mosh-client`,
`et`, `telnet`, `docker`, `podman`, `nerdctl`, `kubectl`, `oc`, `lxc`,
`incus`, `multipass`, `machinectl`, `distrobox`, `toolbox`,
`session-manager-plugin` (AWS SSM), or one you add with
`@modal_transports` (e.g. a wrapper script's name). Otherwise the pane is
read as one screen and the finding is still shown, struck through. When
a spec claims the pane's command (a local htop, a REPL), the screen is
not probed for a multiplexer at all, and those badges are absent.

A shell shows no badges. Put them in a pane border (`E:` makes tmux
evaluate the `MAP _` part when it draws):

```tmux
set -g pane-border-status top
set -g pane-border-format '#{pane_index} "#{pane_title}" #{E:@modal_badges}'
```

Each badge is also published on its own (`@modal_badge_alt`,
`@modal_badge_app`, … `@modal_badge_cursor`) to place anywhere, and each
has a template option, `@modal_badge_<key>_format`: keys `alt`, `via`,
`app`, `app_unknown`, `mode_commanding`, `mode_typing`, `mode_unknown`,
`mode_commanding_low`, `mode_typing_low` (low confidence), `nest`, `nest_off`, `split`, `split_off`, `map`, `cursor`,
`hook_fired`, `hook_ok`, `hook_fail`, `hook_timeout` and their `_off`
variants (`hook_ok_off`, …: a call for a pane not typed into), `attach`. `off` hides a
badge. Placeholders: `{app}`
`{APP}` `{mode}` `{MODE}` `{evidence}` `{score}` `{basis}` `{rule}`
`{confidence}` `{conf}` (`hi`/`lo`) `{dropped}` `{near}` `{version}` `{code}` `{merged}` `{via}` `{kind}` `{panes}`
`{focus}` `{shape}` (the word: `block`, `underline`, `bar`) `{glyph}` `{leader}`; `{ name}` / `{:name}` are a space / a colon and the value, or
nothing when it is empty. Templates may use tmux formats and styles.

**Why a pane looks the way it does:** `tmux-modal explain [pane]` prints
the pane's published options and a fresh classification with the full
score sheet. Bind it to a key:

```tmux
bind M display-popup -E -w 90% -h 90% "~/.config/tmux/plugins/tmux-modal/bin/tmux-modal explain #{pane_id} | less -R"
```

### Pane options

For every pane the daemon publishes these options (empty ones are unset):

| Option | Values |
|---|---|
| `@modal_app` | spec name, e.g. `htop`; empty when nothing recognised |
| `@modal_mode` | `normal`, `insert`, …, `unknown` (a full-screen app no spec knows, or its mode not readable), `none` (no full-screen app) |
| `@modal_bucket` | `commanding`, `typing`, `unknown`, `none` |
| `@modal_confidence` | `high` or `low` |
| `@modal_mode_basis` | how the mode was decided: `fp`, `absence`, `veto`, `mem`, `always`, `policy` (see the `why` badge) |
| `@modal_mode_rule` | the mode rule whose fingerprint matched: its name, or its mode when it has none (`search`, `filter`, `insert`, …) |
| `@modal_alt` | `on` on the alternate screen |
| `@modal_evidence` | `cmd`, `title`, `fp` (joined with `+`), or `mem` |
| `@modal_score` | the fingerprint's score against its threshold, e.g. `110/40` |
| `@modal_via` | the transport the pane's command is: `ssh`, `docker`, `kubectl`, … |
| `@modal_nested` | empty, or why the pane is taken to show another multiplexer: `command`, `status-line`, `borders`, `title` (see [SSH and nested tmux](#ssh-and-nested-tmux)) |
| `@modal_nested_off` | the same evidence when it was seen but not acted on (no transport) |
| `@modal_nested_kind` | which one: `tmux` (also for status line, borders, title: those are tmux's), or the local command: `screen`, `zellij`, `tmate`, `byobu` |
| `@modal_split` | number of inner panes when its window is split |
| `@modal_focus_by` | `border`, `cursor`, or `?` (split, focus not visible) |
| `@modal_remap` | `on` while keys are remapped for this pane |
| `@modal_cursor_shape` | `block`, `underline`, `bar` when the app set one (tmux ≥ 3.5) |
| `@modal_reason` | the classifier's own sentence for the latest capture |
| `@modal_badges`, `@modal_badge_<name>` | the rendered badges, above |
| `@modal_indicator` | the older single tag, below |

Formats only read variables; all matching happens in the daemon, never in
a format string. You do **not** need `status-interval 0`: the daemon pushes
a status redraw (`refresh-client -S`) itself whenever a badge or the
indicator changes, and pane borders redraw on their own.

### The single indicator

`@modal_indicator` is one tag per pane, kept from before the badges (e.g.
for a status line: `set -g status-right '#{@modal_indicator} %H:%M'`).
Templates, one per bucket (`{MODE}`/`{mode}`, `{APP}`/`{app}`,
`{bucket}`, `{confidence}` are substituted):

| Option | Default | Shown for |
|---|---|---|
| `@modal_indicator_commanding` | `#[fg=colour16,bg=colour75,bold] {MODE} #[default]` | normal, visual, … |
| `@modal_indicator_typing` | `#[fg=colour16,bg=colour114,bold] {MODE} #[default]` | insert, command line, prompts |
| `@modal_indicator_unknown` | `#[fg=black,bg=colour244] N/A #[default]` | a full-screen app no spec recognises, or whose mode is not readable (left alone) |
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
| `MODAL_EVENT` | `mode` (mode changed), `focus` (you now type into another pane — or the same one again, when its terminal regains focus), `blur` (the terminal lost focus; only with `@modal_hook_blur on`), `stop` (daemon exiting) |
| `MODAL_PANE`, `MODAL_PANE_ACTIVE`, `MODAL_WINDOW`, `MODAL_SESSION`, `MODAL_SESSION_ID`, `MODAL_TIMESTAMP_MS` | where and when |
| `MODAL_CLIENT` | the tmux client (terminal) typed into, e.g. `/dev/pts/3`; empty for `MODAL_PANE_ACTIVE=0` |

`MODAL_TYPING=1` for a plain shell (you are editing a command line) and for
unrecognised apps: a subscriber should only ever do the "commanding" thing
when the daemon is sure.

The hook reports **the pane you type into** (`MODAL_PANE_ACTIVE=1`) —
what an outer keyboard layer needs to know ("what am I typing into right
now?"): when its mode changes, when focus moves to another pane, window or
session, when you switch to another terminal attached to the server, and
**again whenever a terminal regains focus** (back from the browser), even
if nothing changed meanwhile, so a subscriber that reset itself in between
is set right. Other panes' changes are reported too, with
`MODAL_PANE_ACTIVE=0`, when `@modal_scope` makes them observed — including
the focused pane of another terminal you are not looking at.

Knowing which terminal has focus needs `set -g focus-events on` (tmux ≥
3.2 tracks it; ≥ 3.3 tells the daemon at once, older ones within
`@modal_idle_interval`) and a terminal that reports focus (Windows
Terminal, iTerm2, kitty, foot, xterm and most others do). Without it,
every attached terminal's focused pane counts as typed into.

With `set -g @modal_hook_blur on`, a terminal losing focus to another
window, with no other terminal of this server taking it, is reported as
`MODAL_EVENT=blur` (`MODAL_MODE_TO=none`, the pane it left), and nothing
more is reported until a terminal has focus again.

Guarantees:
- A transition runs the hook **at once**; further transitions within
  `@modal_hook_debounce` (30 ms) of that call are **coalesced** into one
  call with the final state at the end of the window, reported from the
  state the subscriber last saw.
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
| `ahk-http.sh` | calls an **AutoHotkey v2 script serving HTTP on Windows' localhost** from WSL: `…/send/F?OSD;;;;vim/insert/typing/<ms>` for the focused pane (URL, function name and curl path via `MODAL_AHK_URL`, `MODAL_AHK_FUNC`, `MODAL_AHK_CURL`). **Fire and forget**: Windows `curl.exe` (the Windows localhost is not reachable from WSL's default networking) is started detached and the hook returns at once; the request lands ~80–130 ms later, and a stopped or hung server costs nothing. Requests can arrive out of order, so the receiver should drop one older (last field, ms) than the last it acted on |

**Latency advice:** a long-lived listener fed by a one-line hook will always
beat spawning a heavyweight process per transition. Process start costs tens
of milliseconds natively and hundreds across a WSL→Windows boundary. For a
keyboard layer, keep the AutoHotkey/kanata side running and feed it through
`fifo.sh` (or your own equivalent), not by launching it per transition.

Measured (keypress → hook → a listener reading the FIFO, vim, WSL2, tmux
3.4): entering insert takes **39 ms** at the median (55 ms p90) locally and
62 ms (70 ms) over SSH with the default profile; 31 ms and 40 ms with
`@modal_profile snappy`. Leaving insert adds vim's own Escape timeout
(`ttimeoutlen`, 100 ms in `defaults.vim`; `set ttimeoutlen=10` in your
vimrc makes Escape itself fast).

## Bundled specs

| App | What the spec does | Modes it reports | Verified with |
|---|---|---|---|
| `vim`, `nvim` | detects (**hook only**, never remaps) | normal, visual; insert, replace, select, command line, terminal (nvim) — [details and limits](#vim-and-neovim) | vim 8.2, 9.0, 9.1; nvim 0.6.1, 0.7.2, 0.9.5, 0.12.5; every language they ship |
| `htop` | **remaps keys** | normal; insert in `Search:` / `Filter:` | 2.2.0, 3.0.5, 3.2.2, 3.3.0 |
| `btop` | **remaps keys** | normal; insert while typing the process filter, and on the options screen (it has text fields) | 1.2.3, 1.2.13, 1.3.0 |
| `debconf` | **remaps keys** | normal in lists, menus, yes/no and notes; insert while a text or password field has focus | debconf 1.5.79, 1.5.82, 1.5.86 (Ubuntu 22.04, Debian 12, Ubuntu 24.04); all 45 languages it ships |
| `whiptail` | **remaps keys** | the same, for whiptail dialogs from scripts (raspi-config, installers); a progress gauge is N/A | 0.52.21, 0.52.23, 0.52.24 |
| `less`, `man` | detects | normal; insert in any text prompt (`/` `?` search, `&` filter, `!` shell, `-` option, `:e` …) | less 590; man-db 2.10–2.12 |
| `tig` | detects | normal; insert in `/` `?` search; command at `:` | 2.5.1, 2.5.5, 2.5.8 |
| `lazygit` | detects | normal; insert whenever a text field has focus (commit message, filter, shell command, branch name …) | 0.65.1 |
| `k9s` | detects | normal; insert in the `/` filter; command at `:` | 0.51.0 |
| `ranger`, `lf`, `nnn` | detect | normal; insert / command in their prompts (search, rename, console …) | ranger 1.9.3; lf r28, r31; nnn 4.3–4.9 |
| `ncdu` | detects | always normal (it has no text prompts) | 1.15.1, 1.18, 1.19 |
| `mc` | always insert | its shell command line is always live: letters typed in the panels go there | 4.8.27–4.8.30 |
| `fzf` | always insert | every letter goes to the query; the default and `--reverse` layouts are recognised from the screen too (over SSH, fzf-tmux) | 0.29, 0.38, 0.44 |
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

### vim and neovim

This is the case the transition hook exists for: an outer keyboard layer
that helps while you type and gets in the way in normal mode. The spec
never touches keys; it reports the mode.

It reads what stock vim shows: the `-- INSERT --` / `-- VISUAL --` /
`-- REPLACE --` marker on the last row (`showmode`, on by default), the
ruler (`12,5   All`), the `~` column past the end of the buffer, and the
`:` command line. Where tmux reports the cursor shape (3.6 and later),
neovim's bar cursor in insert mode is used as a second opinion.

- **Every language vim and neovim ship** is covered (the markers are
  translated: `-- EINFÜGEN --`, `-- РЕЖИМ ВСТАВКИ --`, `-- 挿入 --`); the
  spec is checked against all of their message catalogues.
- **Over SSH** it works the same. vim and neovim look alike there; the
  name is guessed from the default layout (neovim's ruler sits in a status
  line), so a remote vim with split windows is reported as `nvim` and a
  remote neovim prompt as `vim`. The modes are unaffected.
- **Modes and buckets:** normal and visual are commanding; insert,
  replace, select (a letter replaces the selection), command line and
  neovim's terminal mode are typing. The hit-enter and `-- More --`
  prompts and `-- (insert) --` (one command from insert mode) are normal.

The target is a host nobody has configured: vim with its defaults
(`defaults.vim`: ruler and showmode on), neovim with its own. Where you
have written a vimrc, a hint you control (a `titlestring` carrying the
mode, say) beats any fingerprint.

Known limits — the screen alone cannot tell:
- **A vimrc without `set ruler`** (vim reads `defaults.vim`, which turns
  the ruler on, only when there is no vimrc): once the opening message is
  gone only the tilde column is left, so normal mode reads as **unknown**;
  markers (insert, visual …) still read right.
- **`set noshowmode` with the ruler on** (e.g. a custom statusline that
  keeps the ruler): insert looks exactly like normal and is reported as
  **normal**. With neovim on tmux 3.6+, the bar cursor corrects that.
- **A statusline plugin that replaces the ruler** (lualine, airline, and
  usually `noshowmode`): only the tilde column is left, which is not enough
  to be sure it is vim, so the mode is **unknown** (`MODAL_TYPING=1`) except
  while a marker shows.
- **neovim with `cmdheight=0`** has no row for the marker: always normal.
- **vim's `:terminal`** shows no marker: reported as normal (neovim's
  `-- TERMINAL --` is recognised).
- **Operator-pending** (`d` waiting for a motion) and `r` read as normal;
  both are commanding, which is the right bucket.
- **Over SSH, before vim is first recognised**, its command line (only
  tildes on screen) reads as unknown; once vim has been seen, it is known.
- **Inside a remote tmux** it works the same (the inner status line is
  removed first; low confidence), also when the remote window is split:
  the focused inner pane (green border, or the cursor) is the one read.
  Escape takes the inner tmux's `escape-time` (500 ms on tmux 3.4; `set -s
  escape-time 10` on the remote fixes that) plus vim's `ttimeoutlen`.

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

apt's configuration questions (`modal-debconf`: needrestart's "Daemons
using outdated libraries", the "Modified configuration file" prompt during
upgrades, `dpkg-reconfigure` …) and whiptail dialogs from scripts
(`modal-whiptail`: raspi-config, installers) get the same map: `jk` move
through lists and menus, `hl` between the buttons, `g`/`G` and
`C-d`/`C-u` jump. Space, Tab and Enter are untouched. whiptail shows the
cursor only while a text or password field has focus, and that is the
switch: typing a host name or password, every key is literal; Tab to the
buttons and the map is back. In a menu, whiptail jumps to the item that
starts with a typed letter; the map shadows that for `h j k l g G` (`_k`
still jumps to "keep the local version"). The screens are recognised by
debconf's `Package configuration` title (in every language debconf
ships) or, for a script's dialog, by its frame and a row of `<buttons>`,
over SSH too; the magenta (Ubuntu) or blue (Debian) background is not
needed.

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
| `@modal_hook_flash` | 1500 | ms the `hook` badge shows after each hook call; `0` never shows it |
| `@modal_attach_flash` | 2000 | ms the `attach` badge (`MODAL <version>`) shows when the daemon attaches to a session; `0` never shows it |
| `@modal_hook_debounce` | 30 | ms |
| `@modal_hook_blur` | `off` | `on` reports a terminal losing focus as `MODAL_EVENT=blur` (needs `focus-events on`) |
| `@modal_confirm_captures` | 2 | captures that must agree before remapping resumes |
| `@modal_transports` | *(none)* | extra commands that count as a remote transport (space-separated), next to the built-in list (see [Badges](#badges)) |
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
| 5 | `active` (default) | 0.10 % / +0.23 % | 0.13 % / +0.20 % | 0.23 % / +0.37 % |
| 5 | `all` / `visible` | 0.47 % / +0.70 % | 0.47 % / +0.80 % | 0.73 % / +1.10 % |
| 20 | `active` (default) | 0.23 % / +0.77 % | 0.17 % / +0.33 % | 0.50 % / +0.70 % |
| 20 | `all` / `visible` | 2.26 % / +5.30 % | 1.47 % / +3.23 % | 2.70 % / +6.20 % |
| 1 vim, typed into at 10 keys/s | any | 1.13 % / +0.73 % | 1.20 % / +0.83 % | 1.16 % / +0.76 % |

With the default scope only the focused pane of each attached session is
examined, and tmux stops streaming the other panes' output to the daemon,
so the cost barely grows with the number of panes. Typing is the busiest
case for one pane: every keystroke redraws, so the pane is examined at the
burst cadence the whole time (about 1 % either way, whatever the profile). `visible`/`all`
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
- **Inner window split into several panes:** the outer screen shows all
  inner panes at once. The focused one is cut out along the borders
  (splits within splits too) and read on its own, so htop, vim and the
  rest work as in an unsplit remote tmux, keys remapped included. It is
  found by tmux's default active-border colour (green beside the focused
  pane; with two panes only the half of the border on its side — the same
  on tmux 3.0 to 3.7), or else by the cursor, which tmux draws only in the
  focused pane. A remote tmux with a themed border colour falls back to the
  cursor alone: htop and btop hide it, so they are N/A there. If colour and
  cursor disagree, or neither says anything: N/A (`unknown`,
  pass-through). A shell in an inner pane is N/A too (no spec knows a
  shell on the alternate screen; `MODAL_TYPING=1` either way). tmux repeats
  arrow keys after the prefix for 500 ms (`repeat-time`): a remapped `k`
  (Up) typed right after `prefix Left` on the remote moves to the pane
  above.
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

Clause keys: `regex` with `rows`/`row`, `col`/`cols` (negative counts
from the right edge, as vim's ruler at `col = -18`), `anchor`
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
insert, primary screen), `less-prompts` (less's own prompts), `newt`
(whiptail: cursor visible = a text entry has focus). Groups carry no key
maps.

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
TMUX_MODAL_BENCH=1 go test ./tests/integration/ -run TestBenchmarkCPU -v -timeout 30m   # the CPU table above
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
