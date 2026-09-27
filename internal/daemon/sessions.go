package daemon

import (
	"strings"

	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// sessionState tracks the key-table option of one session. The daemon only
// ever restores a table it set itself; a session whose focused app has no
// key map is never touched (§5.2, §11).
type sessionState struct {
	own string // the modal table we set, "" when we have not touched it
}

// savedOpt persists the session's original key-table so a restarted (or
// crashed-and-restarted) daemon can put it back: "local:<table>" when the
// session had its own value, "inherit" when it used the global one.
const savedOpt = "@modal_saved_key_table"

func (d *Daemon) session(id string) *sessionState {
	ss, ok := d.sessions[id]
	if !ok {
		ss = &sessionState{}
		d.sessions[id] = ss
	}
	return ss
}

// syncKeyTables makes each session's key-table mirror its focused pane:
// the app's modal table while that pane is in a commanding mode of a
// remapping spec, the original table otherwise.
func (d *Daemon) syncKeyTables(r tmux.Runner, panes []tmux.PaneInfo, humans map[string]bool) {
	focus := map[string]*tmux.PaneInfo{}
	for i := range panes {
		p := &panes[i]
		if p.Focused() && humans[p.SessionID] {
			focus[p.SessionID] = p
		}
	}
	var cmds []string
	for sid, p := range focus {
		ss := d.session(sid)
		desired := ""
		if st, ok := d.panes[p.ID]; ok && d.cfg.Enabled && p.Enabled != "off" && !p.InMode &&
			st.cur.Bucket == spec.BucketCommanding {
			if sp := d.set.Specs[st.cur.App]; sp != nil && sp.HasKeys() {
				desired = TableName(sp.Name)
				d.ensureInstalled(r, sp)
			}
		}
		current := p.KeyTable
		switch {
		case desired != "" && current != desired:
			if ss.own == "" && !strings.HasPrefix(current, "modal-") {
				cmds = append(cmds, d.saveOriginal(r, sid)...)
			}
			cmds = append(cmds, oneLine(tmux.Command("set-option", "-t", sid, "key-table", desired),
				d.repointClients(sid, current, desired)))
			ss.own = desired
			d.log.Infof("session %s: key-table %s -> %s", sid, current, desired)
		case desired == "" && (ss.own != "" || strings.HasPrefix(current, "modal-")):
			restore := d.restoreCommands(r, sid, current)
			cmds = append(cmds, restore...)
			ss.own = ""
		}
	}
	d.run(r, cmds)
}

// repointClients moves the session's human clients that sit in the old
// default table onto the new one. Setting the option alone is not enough:
// a client keeps its current table until its next key, and because that
// table is no longer the default, an unbound key would be retried in the
// new default and then discarded (server_client_key_callback) — eating the
// first keystroke after every transition. Clients in any other table (the
// prefix table, a one-shot table) are left alone.
func (d *Daemon) repointClients(sid, from, to string) []string {
	var cmds []string
	for _, c := range d.clients {
		if c.Control || c.SessionID != sid {
			continue
		}
		if isDefaultish(c.KeyTable, from) {
			cmds = append(cmds, tmux.Command("switch-client", "-c", c.Name, "-T", to))
		}
	}
	return cmds
}

// oneLine joins a key-table change and the client re-points into a single
// command list. tmux runs one list without handling other clients' input
// in between, so no keystroke can arrive after the option changed but
// before its client was moved: such a key would be discarded (see
// repointClients). The option comes first, so a failing re-point (a
// client that just detached) cannot prevent it.
func oneLine(set string, repoint []string) string {
	return strings.Join(append([]string{set}, repoint...), " ; ")
}

func (d *Daemon) saveOriginal(r tmux.Runner, sid string) []string {
	val := "inherit"
	if lines, err := r.Run("show-options", "-qv", "-t", sid, "key-table"); err == nil && len(lines) > 0 && lines[0] != "" {
		val = "local:" + lines[0]
	}
	return []string{tmux.Command("set-option", "-t", sid, savedOpt, val)}
}

// restoreCommands puts a session's original key-table back.
func (d *Daemon) restoreCommands(r tmux.Runner, sid, current string) []string {
	saved := ""
	if lines, err := r.Run("show-options", "-qv", "-t", sid, savedOpt); err == nil && len(lines) > 0 {
		saved = lines[0]
	}
	var set, target string
	if strings.HasPrefix(saved, "local:") {
		target = strings.TrimPrefix(saved, "local:")
		set = tmux.Command("set-option", "-t", sid, "key-table", target)
	} else {
		set = tmux.Command("set-option", "-u", "-t", sid, "key-table")
		if lines, err := r.Run("show-options", "-gqv", "key-table"); err == nil && len(lines) > 0 {
			target = lines[0]
		}
		if target == "" {
			target = "root"
		}
	}
	cmds := []string{
		oneLine(set, d.repointClients(sid, current, target)),
		tmux.Command("set-option", "-u", "-q", "-t", sid, savedOpt),
	}
	d.log.Infof("session %s: key-table %s -> %s (restored)", sid, current, target)
	return cmds
}

// ensureInstalled (re)generates an app's key tables when they are missing
// or when the root table they copy has changed.
func (d *Daemon) ensureInstalled(r tmux.Runner, sp *spec.Spec) {
	root, err := r.Run("list-keys", "-T", "root")
	if err != nil {
		root = nil
	}
	h := hashLines(root) + ":" + d.cfg.Leader
	if d.installed[sp.Name] == h {
		return
	}
	runLines(r, UnbindCommands(sp.Name)) // errors expected, see UnbindCommands
	cmds := BindingCommands(sp, d.cfg.Leader, root)
	d.run(r, cmds)
	d.installed[sp.Name] = h
	d.log.Infof("installed key tables for %s (%d commands)", sp.Name, len(cmds))
}

// recoverKeyTables restores sessions left in a modal table by a previous
// daemon that did not shut down cleanly.
func (d *Daemon) recoverKeyTables() {
	r := d.exec
	d.refreshClients(r)
	lines, err := r.Run("list-sessions", "-F", "#{session_id}\t#{key-table}")
	if err != nil {
		return
	}
	var cmds []string
	for _, l := range lines {
		sid, kt, _ := strings.Cut(l, "\t")
		if strings.HasPrefix(kt, "modal-") {
			cmds = append(cmds, d.restoreCommands(r, sid, kt)...)
		}
	}
	d.run(r, cmds)
}

func (d *Daemon) restoreAllKeyTables(r tmux.Runner) {
	lines, err := r.Run("list-sessions", "-F", "#{session_id}\t#{key-table}")
	if err != nil {
		return
	}
	d.refreshClients(r)
	var cmds []string
	for _, l := range lines {
		sid, kt, _ := strings.Cut(l, "\t")
		if strings.HasPrefix(kt, "modal-") {
			cmds = append(cmds, d.restoreCommands(r, sid, kt)...)
		}
	}
	d.run(r, cmds)
}

// isDefaultish reports whether a client's current table is one that the
// key-table option governs (the old default, root, or a modal table) as
// opposed to a transient table the user is in the middle of (prefix, a
// one-shot literal table, copy-mode tables).
func isDefaultish(table, oldDefault string) bool {
	switch {
	case table == oldDefault, table == "", table == "root":
		return true
	case strings.HasPrefix(table, "modal-literal-"):
		return false
	}
	return strings.HasPrefix(table, "modal-")
}
