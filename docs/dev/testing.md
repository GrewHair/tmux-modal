# Testing

```sh
go test ./internal/... -count=1          # unit + golden fixtures, < 10 s, no tmux needed except config round-trip
go test ./tests/integration/ -count=1    # real tmux + htop + real client, ~20 s
go test ./tests/integration/ -run TestDetectionLatency -v   # latency/CPU numbers
docker run --rm -v "$PWD:/mnt" -w /mnt koalaman/shellcheck:stable -x modal.tmux scripts/*.sh scripts/fixtures/*.sh examples/*.sh examples/hooks/*.sh tests/integration/testdata/keyecho.sh
gofmt -l . ; go vet ./...
```

CI (`.github/workflows/ci.yml`, ubuntu-24.04, tmux 3.4) runs all of the
above including the integration tests. Keep it green before every push.

## Golden fixtures

- Format (`internal/screen/fixture.go`): `# tmux-modal fixture v1` header with
  `size`, `cursor`, `cursor_visible`, `cursor_shape`, `alternate_on`,
  `command`, `title`, `expect_app`, `expect_mode`, `note`, then `# ---` and the
  plain screen; optional sibling `.ansi` with the `-e` capture.
- `internal/classify/fixtures_test.go` walks `tests/fixtures/` and checks each
  fixture four ways: **remote** (as captured — command is `docker`, so
  identity must come from the screen), **local** (command = app), **sticky**
  (app pre-identified), **mono** (colour stripped). Expectation `unknown` on a
  screen with no anchors means app "" remotely but the app locally/sticky.
- Capture: `tmux-modal capture -o file.txt -expect-app A -expect-mode M %N`,
  or the scripted `scripts/fixtures/htop.sh LABEL docker run --rm -it IMAGE htop`
  (uses `scripts/fixtures/lib.sh`: private `-L` socket, `-f /dev/null`,
  status off, neutral pane title).
- **Always capture inside containers** (`tests/docker/apps.Dockerfile`, images
  `tmux-modal-fx:<base>`: ubuntu 20.04/22.04/24.04, debian bookworm; include
  htop, less, man-db, vim-nox; user `demo`). Never commit host process lists,
  paths, user or host names.

## Integration harness (`tests/integration/harness_test.go`)

- Two private servers per test: inner (`mi-<pid>-<n>`, session `main`) holds
  the app; outer (`mo-…`) runs `tmux -L inner attach -t main` in its pane.
  `typeKeys` sends to the outer pane ⇒ keys pass through a **real client's key
  tables** (send-keys to the inner pane would bypass them).
- Inner: `status off`, `escape-time 5`, `@modal_spec_paths` =
  `tests/integration/testdata/specs`, `@modal_log_level debug`.
- Daemon: built once in `TestMain`, started with `-L inner --log <tmp>`;
  on failure the log, stderr and final screen are dumped.
- `testdata/keyecho.sh`: deterministic alt-screen TUI printing
  `last=[KeyName]`, `/` opens `PROMPT>` on the last row; `KEYECHO_NAME` changes
  the banner (`KEYHOOK` is matched by the hooks-only test spec). It
  poll-reads (`read -t 0.1`) so the WINCH trap can redraw after resizes.
- Use pane ids (`%0`, `%1`) as targets, not `main.0` (ambiguous).

Covered: remap + literal typing incl. first key after switch; leader
(one-shot, leader-leader); hooks-only never touches key-table; key-table
follows focus; per-pane disable; custom session key-table restored; stop and
crash recovery; root bindings copied (mouse, `bind -n`); htop
search/type/cancel/confirm/filter/resize/panels/leader; hook env, focus
stream, stop event, spec hook order; indicator in real borders incl. N/A;
latency; daemon client label.

## Ad-hoc probing recipes

- Use bash scripts in the scratchpad: the agent's Bash tool runs **zsh**,
  which does not word-split `$T`-style command variables. Put probes in
  `bash script.sh` files.
- A second tmux version: `docker build --build-arg TMUX_VERSION=3.7c -t tmux-modal-tmuxsrc:3.7c -f tests/docker/tmux-src.Dockerfile tests/docker`
  (image has neovim, vim-nox, htop).
- To test the owner's real config without touching their server:
  `tmux -L modal-sandbox -f ~/.config/tmux/tmux.conf new-session -d …` with
  `XDG_STATE_HOME` pointed at a scratch dir, attach from an outer `-L`
  server, then `kill-server` both (the sandbox daemon exits by itself).

## Still missing (planned)

- SSH tier (sshd container, `ssh localhost`) and nested-tmux tier — M5.
- vim-family mandatory fixture set (brief §9) — M7.
- 20-pane CPU benchmark per profile — M6.
