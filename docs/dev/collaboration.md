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
  check), `pane-border-status top`, AutoHotkey v2 on the Windows side (the
  motivating subscriber for the hook), nvim (LazyVim, lualine) as editor.
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
- **Fingerprints target unconfigured hosts** (owner, M7). The screen
  fingerprinting is "for worst case stuff": servers nobody configured.
  Assume default configs (vim with defaults.vim). Where the owner has a
  config, they would rather add an explicit hint (e.g. a vim
  `titlestring` with the mode) than have heuristics widened — so gaps
  caused by customised configs are documented, not fixed.
- **Answer questions with verified facts.** When the owner asks "did you
  account for X?", run a probe (a scratch test on a private socket) before
  answering rather than reasoning from the code; offer to keep it as a test.
- **The AutoHotkey side** (M7): AutoHotkey **v2**, a long-running script
  that serves HTTP on Windows' localhost:42800:
  `/mnt/c/Windows/System32/curl.exe "http://localhost:42800/send/F?OSD;;;;hi"`
  calls the AHK function `OSD` with argument `hi` (an on-screen
  notification). The owner swaps `OSD` for a real function when ready.
  They offered to explain the internals if needed — ask rather than
  guess. Requirements they gave: **fire and forget** (never wait for the
  response) and **never hang** if the server is not running.
  Sending a few test OSDs while developing is fine (they asked for it).
- **Before a compaction** the owner asks for everything to be written into
  these docs; after it, they expect work to resume from the "Next" line in
  `CLAUDE.md` / status.md without re-asking.
- **Privacy slip-ups get fixed at the root.** The owner approved rewriting
  public history to remove the machine name; check new docs for host data
  before every push (`grep -rI "$(hostname)"`).
- **"Thoughts?" means probe, then propose.** When the owner shows a
  new app (the debconf screenshot), verify the claims in a container
  before recommending, list the concrete open questions, and propose
  the spec shape; they then say "go ahead". They like hearing about
  related tools that could be covered too (whiptail from scripts).
- **Auto mode's permission check can fail transiently** (no verdict,
  command blocked). It is not a permission the owner must grant; carry on
  with Write/Edit work and retry the shell command later.
- **Show the engine's conclusions plainly** (since 1.3.0): one fact per
  badge, exact terms (ALT = alternate screen, name the multiplexer:
  `NEST tmux`), short texts; findings that are not acted on are shown
  struck through, not hidden — absent only when never probed. Only start
  trading off when ~10 badges would show at once (D35, D36).
- **Wording is the owner's call, and they value space but dislike cryptic
  abbreviations.** Accepted: `NEST`, `MAP`, `VIA`, `fp`, `mem`, `hi`/`lo`,
  `fp:<rule>`, cursor glyphs █ ▁ ▏ ("show, don't tell"). Declined: `dflt`,
  `ttl`, `pol`, `bdr`/`cur`, merged path badges. "screen" reads as GNU
  screen — avoid it as a word. Offer shortenings as a table and let them
  pick; apply exactly the picks.
- **Bigger features go through plan mode**, which the owner switches on
  themselves: explore, ask 2–3 AskUserQuestion questions with a
  recommended option, write the plan file, ExitPlanMode. They answer with
  refinements in the rejection text (e.g. "NEST instead of NESTED") —
  fold those in and exit again. Smaller ideas: they ask "let's discuss" /
  "plan it out" — answer in chat with a recommendation and wait for "go".
- **"Please be brief"** means a few sentences, no headings, no tool work
  unless a fact must be checked.
- **Every behaviour change ships as a release** the same day (patch for
  fixes and wording, minor for features), plugin clone updated; the owner
  restarts tmux when it suits them.
- **When they report something odd on their own screen, investigate their
  live pane read-only** (`tmux-modal explain %N`, `capture-pane -e`) —
  never saving that capture in the repo — then reproduce it synthetically
  for the test (F46: Claude Code's grey rule).
