# Architecture and code map

```
tmux server
 ├─ human clients
 └─ tmux-modal daemon  (one per server; flock in $XDG_RUNTIME_DIR/tmux-modal-<uid>/)
      ├─ control clients: one `-C attach -f ignore-size` per human-attached session,
      │     attached one at a time after 250 ms of quiet (F49)
      │     └─ %output / focus notifications ──► events (coalesced, reader goroutines)
      ├─ loop (single goroutine): reconcile ► cycle ► sleep until next due / event
      │     cycle = list-panes -a (tier 1) ► gates ► pipelined capture-pane (tier 2)
      │             ► classify ► transition ► publish ► key tables ► focus sweep ► status refresh
      └─ hookRunner (goroutines): debounce, supersede, timeout, process-group kill
```

## Packages

| Path | Role |
|---|---|
| `cmd/tmux-modal/main.go` | CLI: `daemon`, `stop`, `validate`, `capture`, `lint`, `version` |
| `specs.go` | `//go:embed specs` — bundled specs (`specs/*.toml`, `specs/groups/*.toml`) |
| `internal/screen` | `Screen` (lines, optional cells, cursor, flags; full-screen row coordinates, `Top` for partial captures), SGR parser (state carried across lines), colour normalisation, fixture file format |
| `internal/spec` | TOML load (bundled then user dirs; shadow by file stem, or merge when `overlay = true`, D29), `normalise` sugar, `resolve` extends (list = mixin chain, cycle check), `merge`, `compile` to `Spec`/`Rule`/`Clause`, clause/rule `Eval` (tri-state), `lint`, `LocaleAudit` |
| `internal/classify` | `EvalIdentity` (Identified vs Confirmed), `Classify` (always → mode rules → absence needs confirmed identity → corroboration veto → default), `Identify` (all specs: priority, then command match, then confirmed, then score), `Pane` (entry point: nested detection + status-row strip, sticky app, shell fast path, nested policy); `nested.go` `DetectNested` |
| `internal/validate` | the score-sheet report |
| `internal/tmux` | `Exec` runner, `Control` client (pipelined `Send`/`Wait`, guarded block parser, `ClientLabel`), `Quote`/`Command`, `ListPanes` tier-1 snapshot, `CaptureArgs`/`Fill`, `Capture` |
| `internal/daemon` | `daemon.go` lifecycle/reconcile/shutdown/`runLines`; `cycle.go` scheduler, examine, transition, publish, indicator, focus sweep, hook events; `badges.go` badge facts, rendering and publishing, the hook flash (D35–D38; the hook runner reports calls through `hookRunner.observe` → `events.onHook` → `publishBadges`); `sessions.go` key-table ownership, client re-point, install, recovery; `keys.go` binding generation; `focus.go` which terminal is typed into and the hook's focus stream (D39: client focus flags, the `client-focus-in/out[7171]` hooks that SIGWINCH the daemon, replay, blur); `hooks.go` runner (per key: leading edge, then coalesce within the debounce window, D31); `config.go` options/profiles/unescape; `sys.go` lock + CPU throttle; `log.go` |
| `modal.tmux`, `scripts/binary.sh` | TPM entry: resolve binary, `run-shell -b` the daemon with `--socket '#{socket_path}'` |
| `examples/` | hook scripts (`log`, `notify`, `windows-toast`, `fifo`) and `listener.sh` |

## Key invariants (do not break)

1. A spec without `[keys]` never causes `key-table` or `@modal_saved_key_table` to be written.
2. `normal` (the default mode) is only concluded when identity is **confirmed on the same capture**; otherwise `OtherwiseMode` (`unknown`).
3. Unknown / none ⇒ pass-through: no table switch, remap guard false.
4. The session key-table is only ever set to a table that becomes the default; `switch-client -T` is only used (a) to re-point clients to that same default and (b) for the one-shot literal table.
5. The daemon never sends keys to panes.
6. Every failure mode (spec error, tmux error, crash) degrades to pass-through; a spec that fails to load is skipped with a warning.
7. Formats in status/border only read `@modal_*` variables. The only
   server state the daemon adds besides options and key tables is its
   `client-focus-in/out[7171]` hook entries, removed at shutdown (D39).
8. A multiplexer seen on screen counts only behind a remote transport command, or when the pane's command is the multiplexer (D36); otherwise the pane is one screen and the finding is only reported. A pane whose inner window is split reports a mode only for the **focused inner pane** — shown by tmux's default green active border (D34) or the visible cursor (D32), never both disagreeing — cut out along the borders and classified on its own; when neither shows it (themed colours and a hidden cursor, a cursor outside every inner pane), it never reports a mode. With `@modal_nested_remap off`, no nested pane reports a key-remapping spec's mode (D20).
9. A remapping spec's commanding default should only be concluded when a binding identity clause anchors the rows its mode rules read, covering them or pinned to one row on the same edge (D21, D29; `lint` warns, and a unit test holds every bundled spec to it as if it had keys).

## Per-pane state (`paneState`)

`app` (sticky identity), `identFails` (3 → drop identity), `cur` (published
`modeState{App,Mode,Bucket,Confidence}`), `pending`/`pendingN` (confirmation
before resuming remap), `due` (next examination), `burstStart`,
`lastOutput`, `lastExam`, `inScope`, `indicator`, `altToggles` (counted, not
used: the §3.8 tier-1 nesting signals proved useless, F21).

Invalidations (tier 1): alt-screen toggle or command change → drop identity,
examine now; resize → examine now; newly in scope → examine now.

Tier-1 gate: a local shell (`classify.Shells`) on the primary screen is
`none` without a capture, always (REPL specs match their own command
names, D26). A spec's `poll_interval` replaces `@modal_poll_interval` for
panes identified as that app (`pollFor`).

Capture band: sticky + healthy identity + not nested + spec's rules all bottom-relative →
capture only `max(CaptureBottom, @modal_capture_rows)` rows; otherwise full
screen. (htop's identity uses top rows, so htop captures the full screen.)

## Published per-pane options

`@modal_app`, `@modal_mode`, `@modal_bucket`, `@modal_confidence`,
`@modal_nested`, `@modal_indicator`. Server/session: `@modal_daemon_pid` (global),
`@modal_saved_key_table` (session, only while owned).
