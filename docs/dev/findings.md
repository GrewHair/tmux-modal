# Verified findings

Empirical facts established while building, with how each was verified.
Where a finding contradicts or refines the brief, the brief section is
noted. Environment: WSL2 Ubuntu 24.04, tmux 3.4 locally; other tmux
versions built in Docker (`tests/docker/tmux-src.Dockerfile`,
`--build-arg TMUX_VERSION=...`).

## tmux behaviour

**F1. Batched tier-1 reads work.** `list-panes -a -F <fmt>` evaluates the
format per pane in one command (answers §13 "batched display-message").
Each pane of a linked window is listed once per session.

**F2. Unattached control clients don't persist.** `tmux -C <cmd>` runs the
command and exits; a persistent control connection must `attach-session`.
With `-f ignore-size` it does not change window sizes. It receives
`%output` for every pane in its session only.

**F3. Control-mode block framing.** Replies are `%begin T N F` … `%end T N F`
(or `%error`). `F=0` for blocks not caused by this client (the attach
itself); `F=1` for ours. Pane text inside a block can look like `%end`, so a
block ends only on a terminator with the same `T N`.
Notifications never appear inside a block.

**F4. `unbind-key -a -q -T <missing table>` returns an error in 3.4** (empty
message) despite `-q`. When several commands were joined with ` ; ` on one
control line, that error dropped the whole line — this is why commands are
now sent one per line (still pipelined).

**F5. Setting `key-table` does not move clients** — see decisions D3. A client
in a no-longer-default table discards the first unbound key.

**F6. Copy mode bypasses the session key table.** When the client is in its
default table and the pane is in a mode, tmux uses the mode's table
(`copy-mode-vi`), so modal tables never interfere with copy mode.

**F7. `display-message -p` escapes non-printable characters** (`\x1f` comes
back as the text `\037`). Tabs pass through. Use tab separators.

**F8. Option values print `$` as `\$`** — in `show-options -v` and in
`#{@option}` alike, however the value was set (config file or argv). Only
`$` is escaped; a stored `\$` prints as `\\$`. So unescaping `\$` → `$`
is exact (`unescapeOption`). Matters for `@modal_transition_hook`.
Setting via shell argv also drops a trailing ` ;` (tmux reparses argv).

**F9. `pane-border-format` redraws by itself** when a pane option changes;
the status line does not (needs `refresh-client -S`).

**F10. `escape-time` dominates Esc latency.** Default 500 ms: every Esc is
held that long by tmux before reaching the app. With the default, "leave
insert" measured ~590 ms; with `escape-time 5`, ~90 ms. The integration
harness sets it to 5. The owner already has 0.

**F11. `cursor_shape` availability** (§3.3, §13 — highest-value unknown):
absent in tmux 3.4 and 3.5a (empty; also no `cursor_very_visible` /
`cursor_colour`), present in **3.7c** (latest release at the time; 3.8-rc2
exists). On 3.7c: nvim reports `block`/`bar`/`underline` as the brief
predicts; stock vim reports `default` in every mode (confirmed). **It passes
through a nested tmux**, but the switch back to `block` after Esc took
~510 ms nested versus 4 ms (nvim `ttimeoutlen=0`) / 59 ms (default) direct,
even with `escape-time 0` on both servers. Cause not identified. Conclusion:
over nested tmux it is a slow corroborator only.

**F12. Run-shell / environment.** `run-shell` expands formats in its command,
so `modal.tmux` passes `--socket '#{socket_path}'` explicitly.
`list-keys` output is re-executable tmux syntax (keeps padding; `\;` stays).
`if-shell -F cond { a } { b }` brace syntax works over control mode.

## htop (fixtures: `tests/fixtures/htop/{2.2.0,3.0.5,3.2.2,3.3.0}/{200x50,80x24}`)

**F13. Prompts are not at column 0** (contradicts the §5.2/§6.1 example
`^Search:` `col=0`). The last row's function bar is replaced by a prompt bar:
- 3.x: `F3Next  S-F3Prev   EscCancel    Search: abc`
- 2.2: `F3Next  EscCancel    Search: abc` (no `S-F3Prev`)
- all: `EnterDone  EscClear    Filter: xy`
It is always the last row, including with header meters hidden (`#`) and at
80×24.

**F14. `cursor_flag` is 1 exactly while a text prompt is open**, in every
version, and 0 in normal mode and in the F9/u/F2 panels and help. Only 3.2+
parks the cursor on the prompt row; 2.2 and 3.0 leave it at 0,0.

**F15. Panels.** F9 (signal list: `EnterSend  EscCancel`), `u` (user list:
`EnterShow  EscCancel`), F2 (setup: bar ends `F10Done`) are navigable →
normal. Help (`h`) has no anchors → `unknown` (pass-through), which is fine:
any key leaves it. After Filter+Enter the filter stays active (`F4FILTER`
in the bar) and htop is back in normal.

**F16. htop key bindings (3.3 help):** `h` help, `k` kill, `l` lsof are
shadowed by hjkl; `j`, `g`, `G`, `C-d`, `C-u` and `_` are unbound, so `_`
is a safe leader for htop. Digits start an incremental PID search (no prompt
drawn; not handled — digits are not remapped, so harmless).

**F17. htop colours** (default theme, all versions): keycap labels on the
bottom bar are fg black / bg cyan (e.g. cell (-1,2)); ` Search:` label too.
Same styling in normal and prompt bars → useful for identity only, as bonus.

## Spec-authoring pitfalls found

**F18. Lines are right-trimmed**, so a prompt regex ending in a space
(`'^PROMPT> '`, `'Search: '`) can never match an empty prompt. Use
`( |$)`. `validate` now lints this.

**F19. The brief's §5.2.1 weights are inconsistent:** 70/50/40 with
threshold 100, but "any two of those three clear the threshold" is false
(50+40=90). Use e.g. 70/60/50 when writing the real vim spec.

**F20. TOML placement:** a top-level key appended after a `[[table]]` header
belongs to that table (bit an integration test).

## Performance (one pane, WSL2, tmux 3.4)

Keypress → published mode (`TestDetectionLatency`): into typing 37 ms
(balanced) / 28 ms (snappy); back to commanding 94 / 76 ms (includes the
2-capture confirmation and ~20 ms of the test app's own Esc handling).
Daemon CPU ~0 % idle, ~1.8 % of a core while toggling modes every ~150 ms.
`powershell.exe` from WSL takes ~0.47 s just to start (hence the detached
toast hook).
