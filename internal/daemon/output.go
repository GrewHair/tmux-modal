package daemon

import (
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// paneOutput is what one control client is told about each pane's output.
type paneOutput struct {
	conn *tmux.Control
	off  map[string]bool // pane id -> output paused for this client
}

// syncOutput stops tmux streaming the output of panes the daemon does not
// examine (outside @modal_scope, or disabled) to its control clients, and
// resumes it when they come into scope (`refresh-client -A '%N:pause' /
// '%N:continue'`, tmux >= 3.2). The daemon only needs the fact that a pane
// produced output, but tmux formats and sends every byte of it, which made
// the server's cost of serving the daemon grow with every busy background
// pane.
//
// pause, never off: before tmux 3.7, `off` keeps the pane's output already
// queued for this client while letting the pane buffer behind it be freed,
// and when the client catches up tmux reads freed memory and the server
// dies (tmux issue 5054; reproduced on 3.2a-3.6b, F43). pause discards the
// queue first, and otherwise stops the stream the same way.
//
// Only this client's stream changes: tmux keeps reading the pane for the
// human clients (verified with the pane visible and in a hidden window,
// F34). A pane coming into scope is examined at once anyway, so nothing is
// lost while its output was paused.
func (d *Daemon) syncOutput(panes []tmux.PaneInfo, humans map[string]bool) {
	if d.outputUnsupported {
		return
	}
	want := map[string]map[string]bool{} // session -> pane -> off
	for i := range panes {
		p := &panes[i]
		if want[p.SessionID] == nil {
			want[p.SessionID] = map[string]bool{}
		}
		watched := d.cfg.Enabled && p.Enabled != "off" && d.inScope(p, humans)
		want[p.SessionID][p.ID] = !watched
	}
	for sid, c := range d.conns {
		st := d.output[sid]
		if st == nil || st.conn != c {
			st = &paneOutput{conn: c, off: map[string]bool{}} // a new client starts with all on
			d.output[sid] = st
		}
		var args []string
		for id, off := range want[sid] {
			if st.off[id] != off {
				args = append(args, "-A", tmux.OutputGate(id, !off))
			}
		}
		for id := range st.off {
			if _, ok := want[sid][id]; !ok {
				delete(st.off, id) // pane gone
			}
		}
		if len(args) == 0 {
			continue
		}
		if _, err := c.Run(append([]string{"refresh-client"}, args...)...); err != nil {
			d.log.Infof("refresh-client -A unsupported (%v): output of unwatched panes stays on", err)
			d.outputUnsupported = true
			return
		}
		for i := 1; i < len(args); i += 2 {
			id, state := splitPaneState(args[i])
			st.off[id] = state == "pause"
		}
	}
	for sid := range d.output {
		if _, ok := d.conns[sid]; !ok {
			delete(d.output, sid)
		}
	}
}

func splitPaneState(a string) (id, state string) {
	for i := len(a) - 1; i >= 0; i-- {
		if a[i] == ':' {
			return a[:i], a[i+1:]
		}
	}
	return a, ""
}
