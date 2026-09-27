# Changelog

Each release's section is also its GitHub release notes.

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
