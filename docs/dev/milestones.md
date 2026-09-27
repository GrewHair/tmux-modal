# Remaining milestones — working plan

The brief's §12 lists the milestones; this is the concrete plan for what is
left, with the open questions from §13 that each one must settle. After each
milestone: full test suite, commit (conventional commits), push, CI green,
update [status.md](status.md), README, and — if daemon code changed — bump
`VERSION` and tag a release so the owner's install picks it up.

## M5 — SSH and nested tmux

- Docker image with sshd (key auth for `demo`), htop, vim, tmux (distro 3.4
  and/or 3.7c from `tmux-src.Dockerfile`). Integration tests: local tmux pane
  runs `ssh -t demo@container htop`; assert htop detection and remapping
  exactly as locally (pane command is `ssh`: screen-only identity).
- Nested: pane runs `ssh -t … tmux new htop` (inner tmux with its status
  line). Assert graceful `unknown` — never remapping — and that no spec
  false-positives on the inner status line. Also inner tmux with status off
  and a single pane: htop may then be correctly recognised; that is fine
  (keys go through to the inner active pane) — decide and document.
- Implement brief §3.8 nested detection (all O(1)): alt-screen latched since
  command became ssh with zero toggles (`altToggles` already counted),
  `history_size` frozen, `scroll_region_lower != pane_height-1`, bonus
  title shape `*:*:* - "*"`. Use it to (a) publish a `@modal_nested` flag and
  (b) optionally strip a detected inner status row before matching.
  Never let it enable remapping on its own.
- Measure the nested `cursor_shape` 500 ms delay cause if cheap (inner
  tmux redraw timer?) — see findings F11.
- Update README "Nested tmux" section with measured behaviour.

## M6 — remaining specs, CPU budget, benchmark

Specs per brief §5.4, each with fixtures captured in containers (several
versions where distros differ) and integration tests for its insert
triggers. Order by value/ease: `less` + `man` (slash-search), `tig`,
`ncdu`, `btop`, `k9s`, `lazygit`, `ranger`/`lf`/`nnn`, `mc`
(probably `always = "insert"`), always-insert REPL specs (`fzf`, `psql`,
`mysql`, `sqlite3`, `gdb`, `lldb`, `weechat`, `irssi`, `python`, `node`,
`irb` — note REPLs run on the primary screen: `requires_alt_screen=false`,
and `primaryScreenSpecs()` then disables the shell fast path gate only for
non-shell commands; check cost).
Groups to add: `slash-search`, `colon-command`, `readline-repl`.
Questions to settle: `cursor_flag` behaviour per app (§13); is `_` safe as
leader in `mc`, `k9s`, `tig` (§13); per-submode key maps (defer).
Rule: no key-remapping spec without an empirically verified insert rule —
otherwise `always = "insert"`.
Benchmark: 5 and 20 panes per profile, daemon CPU and (approximately) tmux
server CPU via `/proc/<pid>/stat`; document in README. Consider
`refresh-client -A %N:off` for unfocused chatty panes (D2).
Wire spec `poll_interval` into the scheduler (or M7).

## M7 — vim-family (hooks-only)

- Group `vim-family`, specs `vim`, `nvim` (and `emacs -nw` experimental).
  Start from brief §6.2 but fix the weights (F19: any two of three anchors
  must clear the threshold). Identity anchors: tilde run (longest-run
  semantics), ruler, open-file message, statusline shapes.
- Mandatory fixtures (brief §9): normal with ruler; normal right after a
  message (`"f.txt" 12L written`, `E486: Pattern not found`); insert; insert
  with completion popup; visual, visual-line, visual-block; command line
  open; `cmdheight=2`; `showmode` off; non-English `LANG` (these last two, and
  a mid-redraw partial capture, must assert **unknown**, not normal).
- Locale variants for showmode markers; verify actual strings per locale in
  containers (install `locales`, vim translations). `validate --locale-audit`.
- nvim: `cursor_shape` corroboration needs tmux ≥ 3.7-ish (F11) — works as
  unavailable (ignored) on 3.4. Verify `-- TERMINAL --`; plain vim `:term`.
- Which distro vim packages set `t_SI`/`t_EI` by default (§13).
- Hook latency keypress → subscriber, measured (FIFO listener); if not under
  ~50 ms, say so plainly in the README (§13).
- Per-spec `poll_interval` override in the scheduler.
- This is the milestone the owner personally cares about most: the outer
  keyboard layer is AutoHotkey on the Windows side of WSL (they have an AHK
  script). The FIFO example is the path to that; a Windows-side listener
  example (AHK or PowerShell reading a named pipe / file) would be valuable.

## M8 — polish

README complete (every option, spec authoring, shadowed keys per app,
nested limitation, validate, measured costs, known vim gaps from §6.2),
release, CI green, report URL + CI status.
