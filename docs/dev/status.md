# Status

_Last updated: 2026-09-30, 1.4.0 (nesting needs a transport, VIA badge, D36). All brief milestones done; next: whatever the owner's dogfooding brings up._

## Milestones (brief §12)

| # | Milestone | State |
|---|---|---|
| 1 | Daemon skeleton, state store, tier-0/1 gating | **done** |
| 2 | Spec loader, matcher, `validate`, golden fixtures | **done** |
| 3 | htop end to end (local), key-table sync, focus reconciliation | **done**, reviewed by the owner at the checkpoint |
| 4 | Status indicator and transition hook | **done** |
| 5 | SSH integration tests; nested-tmux fallback | **done** — sshd container tier, nested detection (D20), absence-anchor lint (D21) |
| 6 | Remaining bundled specs, CPU-budget self-throttling, benchmark | **done** — 25 specs (D23–D26), fixtures from three distros, SSH app tier, benchmark + output gating (D28) |
| 7 | Spec overlays (owner's request, D29), then `vim-family` specs | **done** — overlays (v0.5.0), vim/nvim hooks-only specs (v0.6.0, D30, D31); and `examples/hooks/ahk-http.sh` for the owner's AHK v2 endpoint ([milestones.md](milestones.md)) |
| 8 | README, CI, publish | **done** — 1.0.0: README pass, benchmark re-run (with a vim typing case), CHANGELOG.md as release notes, CI actions bumped, and B1's cursor part (D32) at the owner's request |

## Where things live

- Repo: https://github.com/GrewHair/tmux-modal (public). CI: `.github/workflows/ci.yml`; release on `v*` tags: `release.yml` (static binaries for linux/darwin/freebsd, amd64/arm64 + linux/arm, SHA256SUMS).
- Current release: **v1.4.0** (nesting in effect only behind a transport, `VIA` badge, seen-but-off badges struck through, D36). v1.3.2: borders as the only nesting evidence need tmux's default colours (F46). v1.3.1: cursor badge draws █ ▁ ▏. v1.3.0: badges per pane, one fact each, D35; `tmux-modal explain`. v1.2.1: fzf `--reverse` recognised from the screen. v1.2.0: split remote tmux, the focused inner pane by the default green border too, D34. v1.1.1 was the fix: pause, not off, for unwatched panes — the off action crashed tmux 3.2–3.6, F43. v1.1.0 was debconf and whiptail dialogs remap (D33). v1.0.0 was M8 (split remote tmux read by the cursor, D32; README/benchmark/CHANGELOG). Each release's notes are its `CHANGELOG.md` section (`release.yml` fails without one). v0.6.0 was vim and neovim for the hook; v0.5.0 was overlays and remap-ready specs; v0.4.0 was M6 (bundled specs for btop, less, man, tig, lazygit, k9s, ranger, lf, nnn, ncdu, mc, fzf and REPLs; output gating for unwatched panes; spec `poll_interval`); v0.3.0 was remapping through nested tmux on by default (owner's call). `VERSION` must equal the tag without `v` (the release workflow checks). `scripts/binary.sh` downloads the binary matching `VERSION`, so **bump `VERSION` and tag whenever users should get new daemon code** — a plugin update without a new release keeps running the old binary (the resolver accepts a stale binary only as last resort, and re-downloads when the version string differs).

## The owner is dogfooding it

On the owner's own tmux (WSL2, tmux 3.4, fish shell, TPM):

- Plugin clone: `~/.config/tmux/plugins/tmux-modal` (a git clone of the public repo, binary pre-downloaded into its `bin/`). Update with `git pull` there (or TPM `prefix+U`), then the binary resolver fetches the matching release.
- Options: `~/.config/tmux/tmux.conf.d/modal/modal.tmux` (sourced from `tmux.conf`); `@plugin 'GrewHair/tmux-modal'` added to `tmux.conf`.
- Badges in `pane-border-format` (`#{E:@modal_badges}`, since 1.3.0; before: the three-way indicator NORMAL / INSERT / N/A), nothing for shells.
- Hook options in that file, one to be uncommented at a time: `log.sh`,
  `windows-toast.sh`, `ahk-http.sh` (option 4, the owner's AutoHotkey v2
  HTTP endpoint on Windows' localhost:42800, `/send/F?OSD;;;;<payload>`;
  fire and forget). As of 2026-09-28 **all are commented out** (no hook
  active); the owner decides when to switch one on. Log:
  `~/.local/state/tmux-modal/transitions.log`.
- `~/.config` is itself a git repo with the owner's unrelated uncommitted work — **never commit there** (editing the modal options file is fine; it was written for them).
- **Never kill or restart the owner's tmux server.** Test on private `-L` sockets; the owner restarts tmux themselves.
- The owner's own nvim is LazyVim (lualine, noshowmode): locally it reads
  `nvim/unknown` — expected, the vim fingerprint targets unconfigured
  hosts (see collaboration.md).

Owner feedback so far: "everything seems to work alright", "everything
looks good" (after M7); the ~80–130 ms AHK delay "doesn't bother me, it's
good for now" (B7 stays parked). The plugin clone is on v1.1.0 (pulled,
binary fetched); it takes effect when they restart tmux. The owner hit
needrestart's debconf dialog on a server — that is what 1.1.0 is for.

**History was rewritten once** (after M6, to remove the machine name):
anyone with an old clone must `git fetch && git reset --hard origin/main`.
Don't rewrite again without the owner asking.

## Known gaps right now

- htop, btop, debconf and whiptail remap by default; every other app can
  be given keys with an overlay file (D29). vim/nvim never remap (D30).
- debconf/whiptail (D33): in a menu a letter jumps to an item; the map
  shadows that for h j k l g G (`_k` for ucf's "keep"). A whiptail gauge
  (no buttons) is N/A. `dialog(1)` tools and `nmtui` are not covered
  (backlog B8).
- vim/nvim (D30, README "vim and neovim"): target default configs only
  (owner). showmode off with the ruler on reads normal; lualine/airline
  (no ruler) read unknown; a vimrc without `set ruler` reads unknown in
  normal; nvim cmdheight=0 and vim's `:terminal` read normal; over SSH the
  vim/nvim *name* is a layout guess; vim passes through `unknown` for
  ~100 ms on Escape (it clears the mode line, redraws the ruler after
  `ttimeoutlen`).
- less over SSH is recognised only once it shows one of its own prompts
  (`(END)`, HELP, a message); REPLs only locally (D25, D26).
- Inner tmux split into several panes: the focused inner pane is found by
  the default green border (D34, tmux 3.0–3.7) or the cursor (D32). A
  themed remote border colour falls back to the cursor (htop/btop N/A
  there). After `prefix <arrow>` on the remote, a remapped k/j within
  500 ms is taken by the remote tmux as a repeat (moves panes).
- Nesting needs a transport command (D36): a remote tmux reached through
  an unlisted wrapper reads as one screen until it is added to
  `@modal_transports`.
- Remapping through a nested tmux: after the inner prefix key, a remapped
  key arrives remapped (`prefix l` → `prefix Right`); owner accepted.
- Inside a remote tmux, Escape reaches the app after the inner tmux's
  `escape-time` (500 ms default on 3.4) — not ours; README says so.
- Nested `cursor_shape` ~0.5 s delay (F11): cause unknown; only nvim on
  tmux ≥ 3.6 (backlog B2).
- The cursor-shape veto was only unit-tested (shape set on a real
  capture); a manual run on tmux 3.7c with nvim (F45) showed the shapes
  reported and the modes right, but it is not in CI (CI's tmux is 3.4).
- `@modal_burst_decay` is parsed but unused (reserved for a polling fallback).
- CPU budget: measures only the daemon's own CPU, not the tmux server work
  it causes; the benchmark (README) measures both.
- `#{C:}` is not used at all (control-mode capture made it unnecessary; D2).
- Hook latency (F38): keypress → FIFO listener 39 ms p50 local, 62 ms over
  SSH (balanced); `ahk-http.sh` adds ~80–130 ms of Windows `curl.exe`
  start, after the hook has returned.
