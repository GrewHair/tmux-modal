package daemon

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// TableName is the key table a remapping spec installs; it becomes the
// session's *default* table (the key-table option) while the focused pane
// is in a commanding mode, so unbound keys fall through to the pane (§3.1).
func TableName(app string) string { return "modal-" + app }

// LiteralTableName is the one-shot table entered by the escape leader.
func LiteralTableName(app string) string { return "modal-literal-" + app }

// guard is evaluated at keypress time against the pane that receives the
// key. The session key-table can lag the daemon by a cycle (focus moved,
// app exited); the guard makes a stale table harmless instead of wrong.
func guard(app string) string {
	return fmt.Sprintf("#{&&:#{==:#{@modal_app},%s},#{&&:#{==:#{@modal_bucket},%s},#{?pane_in_mode,0,1}}}",
		app, spec.BucketCommanding)
}

// UnbindCommands clear an app's tables. tmux 3.4 reports an (empty) error
// for unbind-key -a on a table that does not exist yet, even with -q, so
// callers run these separately and ignore their errors.
func UnbindCommands(app string) []string {
	return []string{
		tmux.Command("unbind-key", "-a", "-q", "-T", TableName(app)),
		tmux.Command("unbind-key", "-a", "-q", "-T", LiteralTableName(app)),
	}
}

// BindingCommands generates the tmux commands that install an app's
// tables. rootBindings is `list-keys -T root` output, copied so that mouse
// and `bind -n` bindings keep working while the modal table is the default.
func BindingCommands(sp *spec.Spec, leader string, rootBindings []string) []string {
	table, lit := TableName(sp.Name), LiteralTableName(sp.Name)
	if sp.EscapeLeader != "" {
		leader = sp.EscapeLeader
	}
	mapped := map[string]bool{leader: true}
	for _, k := range sp.Keys {
		mapped[k.Key] = true
	}

	var cmds []string
	for _, line := range rootBindings {
		key, rewritten, ok := retable(line, table)
		if !ok || mapped[unquoteKey(key)] {
			continue
		}
		cmds = append(cmds, rewritten)
	}
	g := guard(sp.Name)
	for _, k := range sp.Keys {
		cmds = append(cmds, fmt.Sprintf("bind-key -T %s %s if-shell -F %s { send-keys %s } { send-keys %s }",
			tmux.Quote(table), tmux.Quote(k.Key), tmux.Quote(g), tmux.Quote(k.Send), tmux.Quote(k.Key)))
		cmds = append(cmds, tmux.Command("bind-key", "-T", lit, k.Key, "send-keys", k.Key))
	}
	// leader, then any key: send that key verbatim. leader leader sends
	// the leader itself. The one-shot reset back to the default table is
	// automatic (§3.4).
	cmds = append(cmds,
		tmux.Command("bind-key", "-T", table, leader, "switch-client", "-T", lit),
		tmux.Command("bind-key", "-T", lit, leader, "send-keys", leader),
	)
	return cmds
}

// retable rewrites one `list-keys` line from the root table to another.
// list-keys prints re-executable commands of the form
//
//	bind-key [-r] -T root <key> <command...>
func retable(line, table string) (key, out string, ok bool) {
	f := strings.Fields(line)
	if len(f) < 4 || f[0] != "bind-key" {
		return "", "", false
	}
	i := 1
	for i < len(f) && f[i] != "-T" {
		i++
	}
	if i+2 >= len(f) || f[i+1] != "root" {
		return "", "", false
	}
	key = f[i+2]
	// Replace exactly the "-T root" token pair, preserving the rest of the
	// line byte for byte (its quoting is already tmux syntax).
	idx := strings.Index(line, " -T ")
	rest := line[idx+len(" -T "):]
	rest = strings.TrimLeft(rest, " ")
	rest = strings.TrimPrefix(rest, "root")
	return key, line[:idx] + " -T " + tmux.Quote(table) + rest, true
}

func unquoteKey(k string) string {
	if len(k) >= 2 && (k[0] == '\'' || k[0] == '"') && k[len(k)-1] == k[0] {
		return k[1 : len(k)-1]
	}
	return strings.TrimPrefix(k, `\`)
}

func hashLines(lines []string) string {
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
