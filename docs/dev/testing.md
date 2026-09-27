# Testing

```sh
go test ./internal/... -count=1          # unit + golden fixtures, < 10 s, no tmux needed except config round-trip
go test ./tests/integration/ -count=1    # real tmux + htop + real client + sshd container, ~25 s
go test ./tests/integration/ -run TestDetectionLatency -v   # latency/CPU numbers
docker run --rm -v "$PWD:/mnt" -w /mnt koalaman/shellcheck:stable -x modal.tmux scripts/*.sh scripts/fixtures/*.sh examples/*.sh examples/hooks/*.sh tests/docker/*.sh
gofmt -l . ; go vet ./...
```

CI (`.github/workflows/ci.yml`, ubuntu-24.04, tmux 3.4) runs all of the
above including the integration tests, with `TMUX_MODAL_REQUIRE_REMOTE=1`
so the SSH tier fails instead of skipping. Keep it green before every push.

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
  `expect_mode_<variant>` overrides one variant where the evidence really
  differs (a state with no identity anchor is `unknown` remotely and in
  mono; less's `/abs/path` prompt needs colour). The same file checks that
  every bundled spec is lint-clean and that no spec claims a shell.
- Capture: `tmux-modal capture -o file.txt -expect-app A -expect-mode M
  [-meta key=value] %N`, or the scripts: `scripts/fixtures/htop.sh LABEL
  docker run --rm -it IMAGE htop`, `scripts/fixtures/apps.sh [image-tag]
  [app...]` (every other app, one function per app listing the states),
  `scripts/fixtures/nested.sh`. All use `scripts/fixtures/lib.sh`: private
  `-L` socket, `-f /dev/null`, status off, neutral pane title; `snap FILE
  APP MODE NOTE [key=value...]`. Settle ~1.3 s after Escape for ncurses
  apps (F29); btop needs one key per write (F31) and its CPU model is
  scrubbed from the fixtures by `sanitize_btop`.
- **Always capture inside containers** (`tests/docker/apps.Dockerfile` +
  `apps-install.sh` + `apps-home.sh`, images `tmux-modal-fx:<base>`: ubuntu
  20.04/22.04/24.04, debian bookworm; every bundled app, lazygit and k9s
  from their releases; user `demo` with `~/repo` (git) and `~/tree`, and
  `~/sample.txt`). Build: `docker build --build-arg BASE=ubuntu:22.04 -t
  tmux-modal-fx:ubuntu-22.04 -f tests/docker/apps.Dockerfile tests/docker`.
  Never commit host process lists, paths, user or host names (or hardware:
  btop's CPU model).

## Integration harness (`tests/integration/harness_test.go`)

- Two private servers per test: inner (`mi-<pid>-<n>`, session `main`) holds
  the app; outer (`mo-…`) runs `tmux -L inner attach -t main` in its pane.
  `typeKeys` sends to the outer pane ⇒ keys pass through a **real client's key
  tables** (send-keys to the inner pane would bypass them).
- Inner: `status off`, `escape-time 5`, `@modal_spec_paths` =
  `tests/integration/testdata/specs`, `@modal_log_level debug`.
- Daemon: built once in `TestMain`, started with `-L inner --log <tmp>`;
  on failure the log, stderr and final screen are dumped.
- `keyecho/` (Go, built statically by `TestMain` next to the daemon):
  deterministic alt-screen TUI printing `last=[KeyName]`, `/` opens
  `PROMPT>` on the last row; `KEYECHO_NAME` changes the banner (`KEYHOOK`
  is matched by the hooks-only test spec); redraws on SIGWINCH;
  `KEYECHO_LOG=file` logs every key. It was a bash script until F25.
- **Remote tier** (`remote_test.go`): on first use builds
  `tests/docker/sshd.Dockerfile` (image `tmux-modal-sshd`: sshd, key-only
  login for `demo`, htop, vim, tmux), generates a throwaway key, runs one
  container for the whole run (`--hostname remote`, port on 127.0.0.1,
  the build dir mounted at `/testbin` so `/testbin/keyecho` runs remotely),
  removed in `TestMain`. Pane commands are `ssh -t … demo@127.0.0.1 CMD`.
  Skips without docker unless `TMUX_MODAL_REQUIRE_REMOTE` is set.
- Use pane ids (`%0`, `%1`) as targets, not `main.0` (ambiguous).

Covered: remap + literal typing incl. first key after switch; leader
(one-shot, leader-leader); hooks-only never touches key-table; key-table
follows focus; per-pane disable; custom session key-table restored; stop and
crash recovery; root bindings copied (mouse, `bind -n`); htop
search/type/cancel/confirm/filter/resize/panels/leader; hook env, focus
stream, stop event, spec hook order; indicator in real borders incl. N/A;
latency; daemon client label. Apps over SSH (`apps_test.go`, the sshd
image installs them): less (identified at `(END)`, then search/filter),
man, tig (`:` command, `/` search, C-c), btop (remap; `jk` typed into the
filter stay letters; options screen), lazygit (filter, menu, new-branch
prompt), ranger/lf/nnn prompts, ncdu, mc, fzf. Remote: plain-SSH remap/prompt, htop over
SSH, leaving the remote app for the remote shell; nested tmux with status
line (keyecho and htop: never remap), with inner split and status off
(borders), hooks-only through nested tmux (mode reported, confidence low),
status-off single-pane (documented limitation: remaps), local tmux-in-tmux
(`command`).

## Ad-hoc probing recipes

- Use bash scripts in the scratchpad: the agent's Bash tool runs **zsh**,
  which does not word-split `$T`-style command variables. Put probes in
  `bash script.sh` files.
- **What did tmux really do with a key?** Start the inner server with
  `-vv` (writes `tmux-server-PID.log` in the cwd): look for
  `complete key`, `key table …`, `writing key 0x.. (x) to %N`.
- A second tmux version: `docker build --build-arg TMUX_VERSION=3.7c -t tmux-modal-tmuxsrc:3.7c -f tests/docker/tmux-src.Dockerfile tests/docker`
  (image has neovim, vim-nox, htop).
- To test the owner's real config without touching their server:
  `tmux -L modal-sandbox -f ~/.config/tmux/tmux.conf new-session -d …` with
  `XDG_STATE_HOME` pointed at a scratch dir, attach from an outer `-L`
  server, then `kill-server` both (the sandbox daemon exits by itself).

## Still missing (planned)

- vim-family mandatory fixture set (brief §9) — M7.

## CPU benchmark

`TMUX_MODAL_BENCH=1 [TMUX_MODAL_BENCH_SECONDS=30] go test ./tests/integration/
-run TestBenchmarkCPU -v -timeout 30m`: 5 and 20 htop panes (tiled, one
window), each profile, scope `active` and `all`; daemon CPU and the tmux
server's extra CPU over a no-daemon baseline, from `/proc/<pid>/stat`
(10 ms ticks: use ≥ 20 s windows). Results in the README.
