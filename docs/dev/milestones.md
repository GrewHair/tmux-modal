# Remaining milestones — working plan

The brief's §12 lists the milestones; this is the concrete plan for what is
left, with the open questions from §13 that each one must settle. After each
milestone: full test suite, commit (conventional commits), push, CI green,
update [status.md](status.md), README, and — if daemon code changed — bump
`VERSION` and tag a release so the owner's install picks it up.

## M5 — SSH and nested tmux — done (v0.2.0, revised in v0.3.0)

See decisions D20–D22, findings F21–F25, `tests/integration/remote_test.go`,
`tests/fixtures/nested/`. After review the owner made remapping through a
nested tmux the default (`@modal_nested_remap`, v0.3.0). Carried forward to
[backlog.md](backlog.md): split remote tmux (B1), nested `cursor_shape`
delay (B2, look at it in M7).

## M6 — remaining specs, CPU budget, benchmark — done (v0.4.0)

See decisions D23–D28, findings F26–F34, `specs/`, `tests/fixtures/<app>/`,
`scripts/fixtures/apps.sh`, `tests/integration/apps_test.go`,
`bench_test.go`. Settled: most listed apps already have vim keys, so their
specs detect only (D23); mc and fzf always insert; `cursor_flag` per app
(F30); `_` is unbound in btop (the only new remapping spec). Per-submode
key maps: not needed (btop's options screen is simply insert).

**Next up: M7 remainder — a Windows-side listener example (see M7 below).**

### Original plan

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

## M7 — spec overlays, then vim-family (hooks-only) — done (v0.5.0, v0.6.0)

**Part 1 done in v0.5.0 (D29 has what was built and why the lint was
widened rather than the specs rewritten).** Original plan kept below.
The owner wants the
*potential* to remap keys in any app, including the detection-only ones,
for future cases. The engine is already one tier (a spec remaps iff it has
`[keys]`); what is missing:

1. **Overlay user specs.** A user file with the same file stem as a
   bundled spec and `overlay = true` (e.g.
   `~/.config/tmux-modal/specs/tig.toml` holding only `overlay = true` and a
   `[keys]` table) is **merged onto** the bundled spec (child wins, `[keys]`
   key by key, `false` removes, rule lists concatenate child first) instead
   of replacing it, so bundled fixes keep reaching the user. Without
   `overlay` a same-named user file still shadows (current behaviour).
   Implement in `spec.Load` (`raws` keyed by stem: merge instead of replace
   when the later one is an overlay); `validate` must show the merged
   result; tests in `spec_test.go`; README "Writing specs" + a short "Add
   keys to any app" recipe (replaces the btop "copy the file" advice).
2. **Make every bundled spec remap-ready.** `lintAbsenceAnchor` currently
   runs only for specs with keys. Detection-only specs that would fail it
   if keys were added: tig (mode rows -1, identity row -2), ranger, lf, nnn
   (row -1 status clause not required), k9s (prompt rows [0, 10], weighted
   header). Tighten them (required anchors over the rows their mode rules
   read, re-run fixtures, SSH app tier) and add a unit test that holds every
   bundled spec to the rule as if it had keys (less and lazygit already
   pass; always-insert specs are exempt). Also: an overlay adding keys to a
   spec that fails the rule gets the lint warning in the daemon log.
3. Integration test: an overlay adding `[keys]` to a detection-only spec
   (e.g. tig or lazygit over SSH) remaps in normal mode and passes keys
   through in its prompts.
4. Release (bump `VERSION`).

**Part 2, the vim work: done in v0.6.0** (D30, D31, F35–F38) except the
Windows listener example, which needs the owner's AHK details. Original
plan kept below:

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
- (Per-spec `poll_interval` is already wired, M6.)
- This is the milestone the owner personally cares about most: the outer
  keyboard layer is AutoHotkey on the Windows side of WSL (they have an AHK
  script). The FIFO example is the path to that; a Windows-side listener
  example (AHK or PowerShell reading a named pipe / file) would be valuable.

**M7 Windows side — done:** `examples/hooks/ahk-http.sh` for the owner's
AutoHotkey v2 script, which serves HTTP on Windows' localhost:42800
(`/send/F?<func>;;;;<arg>`; `OSD` shows a notification, the owner swaps in
a real function later). Fire and forget through Windows `curl.exe` (Linux
cannot reach the Windows localhost under WSL's NAT networking); the
payload carries the transition time so the receiver can drop late
arrivals. `windows-toast.sh` (since M4) remains as the slow demo. A warm
bridge (one long-lived Windows process, no per-call start) is in the
backlog (B7) if ~80–130 ms per call ever matters.

## M8 — polish — done (v1.0.0)

Done as planned below. Step 4: the owner asked for B1 before 1.0; built
cursor-only (D32). B7 stays in the backlog. Benchmark: a vim-typing case
was added (~1 % daemon CPU, any profile). CHANGELOG.md sections are now
the GitHub release notes (`release.yml` fails without one for the tag);
older releases' notes were filled from it.

The plan as it was:

From the brief: README complete (every option, spec authoring, shadowed
keys per app, nested limitation, validate, measured costs, known vim gaps
from §6.2), release, CI green, report URL + CI status. Concretely:

1. README pass, top to bottom: check every option in `config.go` is in
   the Options table with its real default; every bundled spec in the
   table with verified versions; shadowed keys per remapping app (htop,
   btop); the nested limitations; `validate`/`lint`/`capture` usage;
   measured costs (benchmark, latency F38); the vim known limits; the
   overlay recipe; the hook examples incl. `ahk-http.sh`. Remove stale
   statements (search for "M7", "not yet", "so far").
2. Re-run the CPU benchmark (vim panes are now a 12-row capture band;
   the benchmark predates vim) and update the README table.
3. Repo hygiene (topics are already set: tmux, tmux-plugin, tui, vim);
   CHANGELOG or release notes per tag
   (GitHub releases have only assets now); bump the CI actions that warn
   about Node 20 (B5) if newer majors exist.
4. Decide with the owner which backlog items (B1 split inner tmux, B7
   warm Windows bridge) go into a v1.0, or ship 1.0 without them.
5. Final release, CI green, report URL + CI status (brief).

## After 1.0 — owner requests

No milestones left. Work now comes from the owner's dogfooding, one
request at a time, each released as a minor version:
- 1.1.0: debconf and whiptail (D33, F42, backlog B8).
- 1.1.1: fix, output gating crashed tmux 3.2–3.6 (F43, D28 amended).
- 1.2.0: split remote tmux read by the border colour too (B1 done, D34, F44).
- 1.2.1: fzf `--reverse` recognised from the screen (owner's `fzf-tmux --reverse` read N/A).
- 1.3.0: badges, one fact each (D35), and `tmux-modal explain`.
- 1.3.1 cursor glyphs; 1.3.2 grey rule is no border (F46).
- 1.4.0: nesting needs a transport, VIA badge, seen-but-off badges (D36).

The pattern that worked for a new app: probe it in the images first
(scratch script on a private `-L` socket; what the cursor does, which
keys letters trigger, colours per distro, locale variants), report the
facts, then spec + captures from three distros + an SSH test.
