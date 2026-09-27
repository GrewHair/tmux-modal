# Working with the owner

- **Plain language.** The owner asked for less technical phrasing when a
  decision is theirs: give context, what it means for them, a recommendation,
  and exactly which answers are needed. When they say "be brief", be brief.
- **The owner handed over the wheel** on process and structure ("I'd rather
  hand you the wheel here"). Make sensible calls; ask only for decisions that
  are genuinely theirs (UX preferences, trade-offs they will feel).
- **Never kill or restart their tmux server.** Test on private `-L`
  sockets; they restart tmux themselves when ready.
- **Don't commit in `~/.config`** (a git repo with their own pending work).
- **Push to GitHub after each milestone**, CI green; tag a release when
  daemon code changes so their TPM install gets it.
- **Privacy:** the repo is public. Nothing from the host (process lists,
  paths, the host name — see `hostname` —, user names) in fixtures or docs.
- Their environment: WSL2 on Windows, tmux 3.4, fish as tmux shell (the
  agent's own shell is zsh), TPM, prefix `S-F1`, `escape-time 0`, mouse on,
  many `bind -n` keys (F-keys, `M-hjkl`, `S-arrows`, `C-s` via an `is_vim`
  check), `pane-border-status top`, AutoHotkey on the Windows side (the
  motivating subscriber for the hook), nvim as editor.
- Stated preferences: explicit, unmistakable naming for anything the plugin
  adds (the `TMUX-MODAL-DAEMON` client label); three-way indicator
  NORMAL / INSERT / N/A; no shell fallback daemon; don't burn cycles on
  unfocused panes.
- **Separate solved from unsolved.** After M5 the owner could not tell
  fixed problems from open ones in a long report. Report as: what's done;
  then a short numbered list of what is genuinely unsolved; then choices
  that are theirs. Don't narrate bugs that are already fixed as if they
  were limitations.
- **Pragmatic defaults, cautious behaviour behind a flag.** The owner
  prefers the useful behaviour on by default when the risk is small and
  understood (e.g. remapping through nested tmux), with an option to turn
  it off — rather than the safest-but-less-useful default. Still never
  ship something that remaps keys while the user is typing.
- **Deferring is fine.** Ideas the owner defers go to `backlog.md` with
  the design so far.
- The brief asked for a stop-and-report after M3; that checkpoint happened
  and the owner approved the design deviations (decisions D1–D3). No further
  mandatory checkpoints, but report at each milestone.
- **Keep options open.** Even where the default is "don't remap", the
  owner wants the mechanism to allow it later (D29: overlays, remap-ready
  specs). Prefer designs that make a future change a config edit.
- **Privacy slip-ups get fixed at the root.** The owner approved rewriting
  public history to remove the machine name; check new docs for host data
  before every push (`grep -rI "$(hostname)"`).
