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

Next: milestone 7, part 2 — the vim-family specs (docs/dev/milestones.md). Part 1, overlays and remap-ready specs, shipped in v0.5.0 (D29).
