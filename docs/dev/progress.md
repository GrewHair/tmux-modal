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
