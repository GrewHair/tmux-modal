# Changelog

Each release's section is also its GitHub release notes.

## 1.5.0 — 2026-09-30

- **`why` badge:** how the mode was decided and how sure: `absence high`
  (no marker on a screen confirmed to be the app's), `marker search high`
  (htop's search prompt matched), `veto low` (vim's cursor shape overruled
  normal), `unconfirmed low` (app only remembered), `always`, `policy`.
  Red when low. Also `@modal_mode_basis`, `@modal_mode_rule`, and in
  `tmux-modal explain`.

## 1.4.0 — 2026-09-30

- **A nested multiplexer needs a transport:** a remote tmux seen on screen
  is acted on only when the pane's command is `ssh`, `mosh-client`,
  `docker`, `kubectl` and the like (`@modal_transports` adds your own);
  a local tmux client counts by its own command. Elsewhere the finding is
  still shown, struck through in grey, and the pane is read as one screen.
- **`VIA ssh` badge** (and `@modal_via`): the transport. Badges now read
  as a path: `ALT VIA ssh NEST tmux SPLIT 2 border htop fp 70/40 NORMAL MAP`.
- New options `@modal_via`, `@modal_nested_off`, `@modal_transports`;
  templates `via`, `nest_off`, `split_off`.

## 1.3.2 — 2026-09-30

- Fix: a full-screen app with a full-width horizontal rule (Claude Code's
  line under its prompt) was taken for a split remote tmux (`NEST tmux
  SPLIT 2`). When pane borders are the only sign of a nested tmux, they
  must now be in tmux's default border colours.

## 1.3.1 — 2026-09-30

- The cursor badge draws the cursor shape the app set (`█` `▁` `▏`)
  instead of naming it; `{shape}` in a template still gives the word.

## 1.3.0 — 2026-09-30

- **Badges:** a row of small tags per pane, one fact each, instead of a
  single NORMAL/INSERT/N/A: `ALT` (alternate screen), the app and how it
  was recognised (`htop cmd+fp 110/40`: by command and fingerprint, with
  the fingerprint's score; `mem` when only remembered; `?` when nothing
  recognised it), the mode (`?` when not readable), `NEST tmux`,
  `SPLIT 3 border` (inner panes, how the focused one was found), `MAP`
  while keys are remapped (`MAP _` after the escape leader), the cursor
  shape. Use `#{E:@modal_badges}` in a border format; every badge is also
  its own option with its own template. The facts are pane options too
  (`@modal_evidence`, `@modal_score`, `@modal_nested_kind`, …).
- `tmux-modal explain [pane]`: what the daemon published for a pane and a
  fresh classification with the full score sheet (bind it to a popup).
- `@modal_indicator` is unchanged.

## 1.2.1 — 2026-09-30

- **fzf `--reverse`** (the query on top, as `fzf-tmux --reverse` shows it)
  is recognised from the screen, not only by its command: over SSH, and
  in panes whose command tmux reports as a wrapper. It used to show N/A
  there. Checked on fzf 0.29, 0.38, 0.44 and 0.70.

## 1.2.0 — 2026-09-30

- **Split remote tmux, any app:** the focused inner pane is now also found
  by tmux's default green active border, so htop and btop (which hide the
  cursor) are read and remapped there too, and vim no longer flickers to
  N/A while it redraws. Checked on tmux 3.0 to 3.7; a remote tmux with a
  themed border colour falls back to the cursor, as before.

## 1.1.1 — 2026-09-29

- **Fix: the tmux server could crash** (tmux 3.2 to 3.6) while tmux-modal
  ran. Stopping the output of a pane that left the daemon's view used
  `refresh-client -A '%N:off'`, which in those tmux versions crashes the
  server when output is still queued for the daemon (tmux issue 5054,
  fixed in tmux 3.7) — likely during heavy output in several panes. The
  daemon now pauses and resumes the stream instead, which is safe on every
  version and saves the same CPU. Upgrading is strongly recommended.

## 1.1.0 — 2026-09-28

- **apt's package questions (`debconf`) and whiptail dialogs get vim keys**:
  needrestart's service list, the "Modified configuration file" prompt
  during upgrades, `dpkg-reconfigure`, raspi-config and installer scripts.
  `hjkl`, `g`/`G`, `C-d`/`C-u` move while a list, menu or the buttons
  have focus; in a text or password field every key is literal (whiptail
  shows the cursor only there). Recognised from the screen alone, over
  SSH too, in every language debconf ships; checked on Ubuntu 22.04,
  24.04 and Debian 12.

## 1.0.0 — 2026-09-28

- **Split remote tmux:** across an inner window split into several panes,
  the inner pane with the visible cursor (tmux draws it only in the
  focused pane) is cut out along the borders, splits within splits too,
  and read on its own. vim, neovim and prompts work there as in an unsplit
  remote tmux; with the cursor hidden (htop, btop) it stays N/A as before.
- CPU benchmark re-run, with a new case: one vim pane typed into at 10
  keys a second (about 1 % of a core for the daemon, any profile).
- README brought up to date; this changelog; CI actions on their current
  majors (Node 24).

## 0.6.0 — 2026-09-28

- **vim and neovim** for the transition hook (never remapped): normal,
  visual; insert, replace, select, command line, terminal (neovim). Every
  language they ship; checked against vim 8.2, 9.0, 9.1 and neovim 0.6.1,
  0.7.2, 0.9.5, 0.12.5, locally, over SSH and inside a remote tmux.
- Transition hooks are **leading-edge**: a transition runs the hook at once
  (it used to wait out the 30 ms debounce); ~39 ms keypress to listener.
- Spec clauses can count columns from the right edge (`col = -18`).
- `examples/hooks/ahk-http.sh`: fire-and-forget calls to an AutoHotkey v2
  HTTP endpoint on the Windows side of WSL.

## 0.5.0 — 2026-09-27

- **Overlay specs:** a user file with a bundled spec's name and
  `overlay = true` adds, changes or removes keys (`keys = false`) without
  copying the detection.
- Every bundled spec that has a commanding mode is held to the remapping
  lint, so keys added by an overlay are safe; the daemon logs per-spec
  warnings and reloads spec files when they change.

## 0.4.0 — 2026-09-27

- Bundled specs for btop (remaps), less, man, tig, lazygit, k9s, ranger, lf,
  nnn, ncdu (detect), mc, fzf and common REPLs (always insert).
- Unwatched panes' output is switched off for the daemon, so the tmux
  server no longer pays for streaming it; CPU benchmark.

## 0.3.0 — 2026-09-27

- Keys are remapped through a remote tmux by default
  (`@modal_nested_remap`, `off` leaves nested panes alone).

## 0.2.0 — 2026-09-27

- SSH: apps are recognised from the screen alone, nothing on the remote
  host. Nested tmux is recognised (status line, borders, command, title)
  and its status line removed before matching.

## 0.1.0 — 2026-09-27

- First release: the daemon (control mode), spec engine with weighted
  clauses and groups, `validate`/`lint`/`capture`, htop key remapping,
  status indicator, transition hook with example hooks.
