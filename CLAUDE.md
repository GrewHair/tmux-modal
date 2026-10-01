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

Machine etiquette (the owner's workstation is shared with heavy desktop apps; a slower agent beats a sluggish machine):
- At most 3 heavy jobs at once: a Docker build or running container, the full test suite (`go test ./...` already runs packages in parallel), a soak/stress/benchmark loop. Build images one at a time. Never saturate all cores on purpose. The owner tunes the number by halving; if they say it's still sluggish, lower it here.
- While iterating, run targeted tests (`-run ...`); run the full suite once before pushing. Announce any heavy job expected to run over ~10 minutes, then go ahead.
- Clean up before saying "done", and check it: containers `--rm` and named, `docker ps -a` shows none of yours; investigation images removed; background jobs stopped. Private tmux servers: kill them (by PID if `kill-server` gets no answer) and delete their socket files under `/tmp/tmux-$UID/`; the test harness does this itself (`reapServer`). Only touch what you started.

All brief milestones are done (v1.0.0); since then owner requests, one minor release each (v1.1.0: debconf/whiptail remapping, D33; v1.1.1: fix for a tmux < 3.7 crash from output gating, F43 — never use `refresh-client -A %N:off`, use pause/continue; v1.2.0: split remote tmux by border colour, D34; v1.2.1: fzf --reverse; v1.3.0: badges, D35; v1.4.0: nesting needs a transport, D36; v1.5.0: why badge, D37; v1.6.0: hook badge, D38; v1.7.0: hook follows the focused terminal, D39; v1.7.1: why merged into the mode badge; v1.7.2: badge colours; v1.7.3: struck-through findings, D40; v1.7.4: control clients attach after a quiet spell, F49 — a tmux < 3.7 crash; v1.8.0: attach badge, D41; v1.9.0: tmuxline evidence, D42). See CHANGELOG.md; next is whatever the owner's dogfooding brings up (docs/dev/milestones.md "After 1.0", backlog: docs/dev/backlog.md).
