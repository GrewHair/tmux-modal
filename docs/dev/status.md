# Status

_Last updated: 2026-09-27, after milestone 4 and release v0.1.0._

## Milestones (brief §12)

| # | Milestone | State |
|---|---|---|
| 1 | Daemon skeleton, state store, tier-0/1 gating | **done** |
| 2 | Spec loader, matcher, `validate`, golden fixtures | **done** |
| 3 | htop end to end (local), key-table sync, focus reconciliation | **done**, reviewed by the owner at the checkpoint |
| 4 | Status indicator and transition hook | **done** |
| 5 | SSH integration tests; nested-tmux fallback | **next** — see [milestones.md](milestones.md) |
| 6 | Remaining bundled specs, CPU-budget self-throttling, benchmark | todo (throttling already implemented, benchmark not) |
| 7 | `vim-family` specs, hooks-only path, per-spec `poll_interval` | todo (hooks-only path already implemented and tested; `poll_interval` parsed but not yet used by the scheduler) |
| 8 | README, CI, publish | **done early** at the owner's request (repo public, CI green, v0.1.0 released); README must keep growing with each milestone |

## Where things live

- Repo: https://github.com/GrewHair/tmux-modal (public). CI: `.github/workflows/ci.yml`; release on `v*` tags: `release.yml` (static binaries for linux/darwin/freebsd, amd64/arm64 + linux/arm, SHA256SUMS).
- Current release: **v0.1.0**. `VERSION` must equal the tag without `v` (the release workflow checks). `scripts/binary.sh` downloads the binary matching `VERSION`, so **bump `VERSION` and tag whenever users should get new daemon code** — a plugin update without a new release keeps running the old binary (the resolver accepts a stale binary only as last resort, and re-downloads when the version string differs).

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

- Only htop is recognised; every other full-screen app is `unknown` (N/A, pass-through).
- No SSH / nested-tmux tests yet (M5). Nested detection heuristics from brief §3.8 are not implemented; nested tmux currently falls back to `unknown` simply because no spec matches the combined screen.
- `@modal_burst_decay` is parsed but unused (event-driven scheduling made it moot; reserved for a polling fallback).
- Spec `poll_interval` is parsed but not consulted by the scheduler.
- CPU budget: implemented (`sys.go`), measures only the daemon's own CPU, not the tmux server work it causes. No 5/20-pane benchmark yet.
- `#{C:}` is not used at all (control-mode capture made it unnecessary; see decisions D2).
- Hook latency end-to-end (keypress → subscriber) not measured separately from detection latency; detection is ~30–40 ms into typing, ~80–95 ms back to commanding, plus 30 ms hook debounce.
