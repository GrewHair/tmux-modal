# Status

_Last updated: 2026-09-27, after milestone 6 and release v0.4.0._

## Milestones (brief §12)

| # | Milestone | State |
|---|---|---|
| 1 | Daemon skeleton, state store, tier-0/1 gating | **done** |
| 2 | Spec loader, matcher, `validate`, golden fixtures | **done** |
| 3 | htop end to end (local), key-table sync, focus reconciliation | **done**, reviewed by the owner at the checkpoint |
| 4 | Status indicator and transition hook | **done** |
| 5 | SSH integration tests; nested-tmux fallback | **done** — sshd container tier, nested detection (D20), absence-anchor lint (D21) |
| 6 | Remaining bundled specs, CPU-budget self-throttling, benchmark | **done** — 25 specs (D23–D26), fixtures from three distros, SSH app tier, benchmark + output gating (D28) |
| 7 | `vim-family` specs, hooks-only path, per-spec `poll_interval` | **next** (hooks-only path and `poll_interval` already implemented and tested) |
| 8 | README, CI, publish | **done early** at the owner's request (repo public, CI green, v0.1.0 released); README must keep growing with each milestone |

## Where things live

- Repo: https://github.com/GrewHair/tmux-modal (public). CI: `.github/workflows/ci.yml`; release on `v*` tags: `release.yml` (static binaries for linux/darwin/freebsd, amd64/arm64 + linux/arm, SHA256SUMS).
- Current release: **v0.4.0** (M6: bundled specs for btop, less, man, tig, lazygit, k9s, ranger, lf, nnn, ncdu, mc, fzf and REPLs; output gating for unwatched panes; spec `poll_interval`). v0.3.0 was remapping through nested tmux on by default (owner's call). `VERSION` must equal the tag without `v` (the release workflow checks). `scripts/binary.sh` downloads the binary matching `VERSION`, so **bump `VERSION` and tag whenever users should get new daemon code** — a plugin update without a new release keeps running the old binary (the resolver accepts a stale binary only as last resort, and re-downloads when the version string differs).

## The owner is dogfooding it

On the owner's own tmux (WSL2, tmux 3.4, fish shell, TPM):

- Plugin clone: `~/.config/tmux/plugins/tmux-modal` (a git clone of the public repo, binary pre-downloaded into its `bin/`). Update with `git pull` there (or TPM `prefix+U`), then the binary resolver fetches the matching release.
- Options: `~/.config/tmux/tmux.conf.d/modal/modal.tmux` (sourced from `tmux.conf`); `@plugin 'GrewHair/tmux-modal'` added to `tmux.conf`.
- Indicator in `pane-border-format` (owner's choice), three-way: NORMAL / INSERT / N/A, nothing for shells.
- Hook: `examples/hooks/log.sh` active; `windows-toast.sh` and "none" provided as commented alternatives. Log: `~/.local/state/tmux-modal/transitions.log`.
- `~/.config` is itself a git repo with the owner's unrelated uncommitted work — **never commit there**.
- **Never kill or restart the owner's tmux server.** Test on private `-L` sockets; the owner restarts tmux themselves.

Owner feedback so far: "everything seems to work alright".

## Known gaps right now

- Only htop and btop remap; the other app specs detect only (D23). vim/nvim are not recognised yet (M7): N/A.
- less over SSH is recognised only once it shows one of its own prompts (`(END)`, HELP, a message); REPLs only locally (D25, D26).
- Inner tmux split into several panes: always `unknown` (the outer screen does not say which inner pane is focused). Improvement designed and deferred by the owner: [backlog.md](backlog.md) B1.
- Remapping through a nested tmux: after the inner prefix key, a remapped key arrives remapped (`prefix l` → `prefix Right`); owner accepted.
- Nested `cursor_shape` ~0.5 s delay (F11): cause still unknown; matters for nvim over nested tmux (M7).
- `@modal_burst_decay` is parsed but unused (event-driven scheduling made it moot; reserved for a polling fallback).
- CPU budget: implemented (`sys.go`), measures only the daemon's own CPU, not the tmux server work it causes; the benchmark (README) measures both.
- `#{C:}` is not used at all (control-mode capture made it unnecessary; see decisions D2).
- Hook latency end-to-end (keypress → subscriber) not measured separately from detection latency; detection is ~30–40 ms into typing, ~80–95 ms back to commanding, plus 30 ms hook debounce.
