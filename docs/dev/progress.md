# Progress log

**2026-09-27**
- Probed tmux 3.4: control mode, batched `list-panes`, `cursor_shape` absent;
  captured htop screens → prompts not at column 0, `cursor_flag` flips.
- Built M1–M3: screen model + SGR parser, spec engine (weighted clauses,
  extends, lint), classifier with the confirmed-absence rule, `validate`,
  control-mode daemon, key tables with guard + root copy + literal leader.
- Fixtures: htop 2.2.0 / 3.0.5 / 3.2.2 / 3.3.0 at 200×50 and 80×24, captured in
  Docker (26 states each); golden test in four variants.
- Bugs found by tests: first key after a table switch discarded (→ client
  re-point, D3); one failing `unbind` sank a batched command line (F4);
  `\x1f` config separator escaped by tmux (F7); test-spec trailing space (F18).
- Built tmux 3.5a and 3.7c in Docker: `cursor_shape` only in 3.7c; passes
  through nested tmux with ~0.5 s delay on Esc (F11).
- M3 checkpoint: owner approved control-mode transport (with explicit client
  label), client re-point; rejected the shell fallback.
- M4: pre-rendered `@modal_indicator` (three-way per owner), transition hook
  with debounce / supersession / focus stream / stop event, example hooks
  (log, notify, Windows toast, FIFO + listener).
- Published: github.com/GrewHair/tmux-modal, CI green first try, release
  v0.1.0 with static binaries. Scrubbed host name from history before the
  first push.
- Installed into the owner's tmux config (plugin clone, `tmux.conf.d/modal/`),
  verified end to end on a sandbox server with their real config. Owner
  restarted tmux: "everything seems to work alright".
- Wrote these dev notes before context compaction.
- M5: sshd container (`tests/docker/sshd.Dockerfile`) and remote
  integration tier; plain-SSH detection/remapping identical to local.
  Measured the §3.8 tier-1 nesting signals — none discriminate (F21).
  Found that htop under a remote tmux read as **normal while its search
  prompt was open** (inner status line over the last row; F22): fixed with
  a required bottom-bar anchor, screen-based nesting detection (status
  line, borders in UTF-8 and VT100 letters, local `tmux` command, title),
  never-remap policy for nested panes, and a lint for the pattern.
  Nested fixtures (16) captured in containers. Chased an intermittent
  latency-test failure down to bash `read -t` dropping bytes in the test
  app (F25, via `tmux -vv`); rewrote the test app in Go. Key-table change
  and client re-point now go in one command list (D22). Released v0.2.0.
- Owner review of M5: remap through nested tmux by default
  (`@modal_nested_remap`, default on); released v0.3.0.
- M6: probed every listed app in containers (`apps.Dockerfile` now
  installs all of them, lazygit/k9s from releases). Found most already
  have vim keys (F26) → detection-only specs (D23), groups without keys
  (D24). Specs: btop (remap), less/man (reverse-video tie-breaker, D25),
  tig, lazygit (cursor = text field, F30), k9s, ranger, lf, nnn, ncdu, mc
  and fzf (always insert), ten REPL/chat specs by command (globs, D26).
  ~300 new fixtures from Ubuntu 24.04/22.04 and Debian bookworm, one
  script (`scripts/fixtures/apps.sh`); btop's host CPU model scrubbed.
  Real screens exposed two nested-detection false positives (aligned text
  as VT100 borders, mc's separator; F27) and lint gaps (D27). SSH app
  tier (`apps_test.go`) drives every app's prompts in the sshd container.
  Wired spec `poll_interval`; CPU benchmark (5/20 panes × profiles ×
  scope) showed the tmux server paying for streaming unwatched panes'
  output → `refresh-client -A '%N:off'` (D28, F34). Released v0.4.0.
- After M6: found the owner's machine name in `docs/dev/collaboration.md`
  (committed since the dev-notes commit, in the line saying not to
  publish it). With the owner's go-ahead, rewrote history with
  `git filter-branch` (only that file changed; HEAD tree identical),
  force-pushed `main` and tags v0.2.0–v0.4.0 with the release workflow
  paused (so no rebuild), releases and assets intact, CI green, fresh
  clone clean. Re-aligned the owner's plugin clone to the new history.
  GitHub may still serve the old commits by hash until GC; a full purge
  needs GitHub Support (offered, not requested).
- Owner decision D29: overlay user specs + remap-ready bundled specs,
  first thing in M7.
- M7 part 1 (D29): `overlay = true` user specs merge onto the bundled
  spec; `keys = false`; `validate` on an overlay shows the merged spec (and
  doesn't apply it twice when it sits in a spec path). New unit test holds
  every bundled spec to the remapping lint as if it had keys: five failed
  (tig, ranger, lf, nnn, k9s). k9s got a required `Context:` anchor; for
  the rest the lint was too narrow (weighted clauses the threshold needs,
  single-row pinned anchors — reasoning in D29). The daemon now logs
  per-spec lint warnings and reloads spec files when they change. SSH
  test: an overlay written while tig runs remaps `K` in normal mode and
  leaves it a letter in the search prompt. Released v0.5.0.
- M7 part 2 (D30, D31, F35–F38): probed vim 8.2/9.0/9.1 and nvim
  0.6.1/0.7.2/0.9.5/0.12.5 in the images (neovim and locales added; the
  release build has no translations). Extracted every showmode string,
  ruler word and prompt from the editors' catalogues
  (`scripts/fixtures/vim-markers.py` → `tests/fixtures/vim/markers.json`)
  and generated the alternations; `TestVimMarkers` checks them all.
  `vim-family` group (markers on the last row only, any unknown marker is
  insert, select is typing), `vim`/`nvim` specs (command locally, layout
  over SSH; `Identify` now ranks a command match first at equal
  priority); right-relative clause columns for the ruler; bottom-relative
  identity so a sticky vim pane is captured as 12 rows. 434 real
  fixtures from three distros plus de/ru/ja. SSH tests for vim and nvim;
  hook latency measured (39 ms p50 local, 62 ms over SSH to enter insert)
  after making hook calls leading-edge (they had waited the whole 30 ms
  debounce). Distro vims never set t_SI/t_EI. Locale audit no longer
  reads regex escapes as words. Released v0.6.0.
- After v0.6.0, at the owner's request: nested-tmux vim tests
  (`TestNestedTmuxVim`: status bottom/top/off, nvim, inner split). All
  behaved; Escape there costs the inner tmux's escape-time. Owner: target
  default configs only; the no-ruler vimrc gap stays documented, unfixed.
- Owner is on AHK v2 with an HTTP endpoint (localhost:42800,
  `/send/F?<func>;;;;<arg>`). Added `examples/hooks/ahk-http.sh` (focused
  pane only, payload `app/mode/typing|commanding`); Linux curl cannot reach
  Windows' localhost (NAT), so Windows curl.exe: ~80 ms process start, ~3
  ms request; verified end to end with vim (hook ok in 77–131 ms). Then
  made fire-and-forget at the owner's request (setsid, stdio to
  /dev/null, --connect-timeout 1 -m 2): the hook returns in <10 ms with
  the server up, down, or hung; the output pipe closes at once. Payload
  gained the transition time (ms) so the receiver can drop late arrivals.
- Handover before compaction (2026-09-28): status.md known gaps and
  dogfooding refreshed, M8 plan made concrete, backlog B7 (M7 leftovers),
  findings F39 (harness pitfalls) and F40 (WSL/Windows), collaboration
  notes (default-config target, AHK v2 endpoint, verify before answering).

**2026-09-28 (M8)**
- README pass (status 1.0, nested split, benchmark table + vim typing row).
  Benchmark re-run with 30 s windows; new `vim-typing` case.
- Owner asked about B1's cost; answered (cursor-only: no extra CPU, about half the full design;
  colour: version/theme-dependent, riskier) and was asked to build it
  before 1.0. Built cursor-only (D32): `innerPanes`, `focusedPane`,
  `Screen.Sub`; 12 new nested fixtures (vim focused in a split, three
  panes, htop focused, shell focused) in both border alphabets; the split
  case of `TestNestedTmuxVim` now drives vim in the split, moves focus to
  the shell and htop (unknown) and back. Findings F41, harness pitfalls
  (prefix through the harness tmux, htop dropping typeahead).
- CHANGELOG.md, release notes from it; checkout/setup-go v7. Released
  v1.0.0.

**2026-09-28 (after 1.0)**
- Owner showed needrestart's debconf dialog and asked for a fingerprint.
  Probed whiptail in the images (F42): cursor visible only in a text
  entry, menus jump by letter, colour differs per distro, VT100 letters
  without UTF-8, translated backtitle (45 languages, extracted raw).
  Built the `newt` group and `debconf`/`whiptail` remapping specs (D33);
  probes in the images (`debconf-probe`, `ucf-probe`, sudo for demo);
  fixtures from Ubuntu 22.04/24.04 and Debian 12; `TestDebconfTitles`;
  SSH tests driving ucf's real prompt and a raspi-config-style menu.
  Released v1.1.0. (The auto-mode permission check failed intermittently
  during the session; work continued with file edits in between.)

**2026-09-29 (1.1.1)**
- Owner reported their tmux 3.4 server crashing at random; another
  session traced it to control mode. Root cause: our output gating's
  `refresh-client -A '%N:off'` hits tmux issue 5054 when the daemon lags
  (F43). Reproduced in Docker on 3.2a–3.6b, fixed by using pause/continue
  (D28 amended), `TestOutputGateSlowReader`. Released v1.1.1. (B1's colour
  work had just started: tmux-src images for 3.0a–3.7c built, nothing
  else yet.)

**2026-09-30 (1.2.0)**
- Owner asked for B1's colour part, default configs only, a few recent
  tmux versions. Probed 3.0a–3.7c (F44: identical, half-border rule for
  two panes); built `colourFocus` (D34), nested panes captured with
  colour; fixtures for tmux 3.4 (both border alphabets) and six more
  versions; `TestNestedFocusColour`; the split integration tests now
  expect htop/keyecho read and remapped in a split. Released v1.2.0.
- Owner: `fzf-tmux --reverse` from a fish function showed N/A. Their
  daemon log: the split was on the alternate screen but tmux never
  reported `fzf` as its command, and the screen fallback knew only the
  default layout. fzf spec now weighted, one set of three clauses per
  layout (threshold 100; the cursor clause, the heaviest, can hold for one
  layout only); `--reverse` fixtures from the three images; scored 100 on
  a live fzf 0.70. Released v1.2.1.
- Owner asked (plan mode) for richer indicators while dogfooding: several
  widgets, app and confidence visible, no "N/A" lie. Agreed plan: badges
  one fact each (ALT, app+evidence+score, mode, NEST kind, SPLIT, MAP
  with the leader state via `#{client_key_table}`, cursor shape), raw
  facts as options, templates, `explain`. Built (D35); unit tests for
  rendering and classify facts, integration tests for the border (MAP →
  MAP _ → MAP), the split facts and explain. Released v1.3.0; owner's
  border format switched to `#{E:@modal_badges}`.
