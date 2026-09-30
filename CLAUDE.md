# tmux-modal — notes for Claude

Start with `docs/dev/README.md`: status, the owner's working preferences,
the remaining-milestone plan, decisions, verified findings, architecture and
testing are all there, plus the original brief verbatim (`docs/dev/brief.md`).

Hard rules:
- Never kill or restart the owner's tmux server; test on private `-L` sockets with `-f /dev/null`.
- Never commit in `~/.config`.
- The repo is public: no host data (process lists, paths, host/user names) in fixtures or docs; capture fixtures in the Docker images under `tests/docker/`.
- Keep the key invariants in `docs/dev/architecture.md` (hooks-only specs never touch `key-table`; `normal` needs identity confirmed on the same capture; every failure is pass-through).
- Before pushing: `gofmt -l .`, `go vet ./...`, `go test ./... -count=1`, shellcheck (also as `koalaman/shellcheck:v0.9.0`, CI's version); `grep -rIl "$(hostname)" . --exclude-dir=.git` must find nothing; CI must be green. Bump `VERSION` and tag a release when daemon code changes.
- Update `docs/dev/status.md` and `docs/dev/progress.md` at each milestone.

All brief milestones are done (v1.0.0); since then owner requests, one minor release each (v1.1.0: debconf/whiptail remapping, D33; v1.1.1: fix for a tmux < 3.7 crash from output gating, F43 — never use `refresh-client -A %N:off`, use pause/continue; v1.2.0: split remote tmux by border colour, D34; v1.2.1: fzf --reverse; v1.3.0: badges, D35; v1.4.0: nesting needs a transport, D36; v1.5.0: why badge, D37; v1.6.0: hook badge, D38). See CHANGELOG.md; next is whatever the owner's dogfooding brings up (docs/dev/milestones.md "After 1.0", backlog: docs/dev/backlog.md).
