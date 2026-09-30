package daemon

import (
	"strings"

	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// Which terminal is being typed into (D39). tmux marks the client whose
// terminal has focus with the "focused" client flag (>= 3.2, fed by the
// terminal's focus reports, so it needs focus-events on); the
// client-focus-in/out hooks (>= 3.3) poke the daemon so it re-reads the
// flags at once instead of at the next idle check (F48).

// focusHookIndex is the array index of the daemon's entries in the
// client-focus-in/out hooks, chosen so that it leaves the user's own
// entries alone.
const focusHookIndex = "[7171]"

var focusHooks = []string{"client-focus-in", "client-focus-out"}

// focusPoke runs in tmux's shell on a focus change. SIGWINCH does nothing
// to a process that does not handle it, so a stale pid is harmless.
const focusPoke = "run-shell -b 'kill -WINCH #{@modal_daemon_pid} 2>/dev/null; true'"

// focusState is what the hook last reported as typed into for a client.
type focusState struct {
	pane, sid string
	m         modeState
}

type focusTracker struct {
	client  string // the terminal being typed into; "" = not known
	session string // its session id
	gained  bool   // client (re)gained focus: report its pane again
	lost    bool   // client lost focus and no terminal has it (blur)
	blurred bool   // no terminal has focus; client is the last one that had it
	was     map[string]bool
	emitted map[string]focusState // client name -> last report
}

func newFocusTracker() focusTracker {
	return focusTracker{was: map[string]bool{}, emitted: map[string]focusState{}}
}

func hasFlag(flags, flag string) bool {
	for _, f := range strings.Split(flags, ",") {
		if f == flag {
			return true
		}
	}
	return false
}

// resolveFocus decides which human client is typed into, from the
// clients' focus flags: the only one focused, else the one that just
// gained focus, else the one that had it (also while none has it: the
// user is in another window and will come back to it). With focus-events
// off every client stays flagged, so focus is not known and every
// client's pane counts as typed into.
func (d *Daemon) resolveFocus() {
	f := &d.focus
	now, byName := map[string]bool{}, map[string]clientInfo{}
	var flagged, newly []string
	for _, c := range d.clients {
		if c.Control {
			continue
		}
		byName[c.Name] = c
		if c.Focused {
			now[c.Name] = true
			flagged = append(flagged, c.Name)
			if !f.was[c.Name] {
				newly = append(newly, c.Name)
			}
		}
	}
	prev, fc := f.client, ""
	if d.cfg.FocusEvents {
		_, prevAlive := byName[prev]
		switch {
		case len(flagged) == 1:
			fc = flagged[0]
		case len(newly) == 1:
			fc = newly[0]
		case now[prev]:
			fc = prev
		case len(flagged) == 0 && prevAlive:
			fc = prev
		}
	}
	if fc != prev || (fc != "" && !now[fc] != f.blurred) {
		switch {
		case fc == "":
			d.log.Debugf("focus: not known (focus-events %v, %d terminals focused)", d.cfg.FocusEvents, len(flagged))
		case !now[fc]:
			d.log.Debugf("focus: no terminal has it; last %s", fc)
		default:
			d.log.Debugf("focus: terminal %s", fc)
		}
	}
	f.client, f.session = fc, ""
	if fc == "" {
		f.gained, f.lost, f.blurred = false, false, false
	} else {
		f.session = byName[fc].SessionID
		if fc != prev || (now[fc] && !f.was[fc]) {
			f.gained = true
		}
		f.blurred = !now[fc]
		if f.blurred && f.was[fc] {
			f.lost = true
		}
		if !f.blurred {
			f.lost = false
		}
	}
	f.was = now
	for name := range f.emitted {
		if _, ok := byName[name]; !ok {
			delete(f.emitted, name) // detached: a reattach reports afresh
		}
	}
}

// typedInto reports whether a pane is one sweepFocus reports as typed
// into: the focused pane of the focused terminal's session, or of any
// attached session while focus is not known.
func (d *Daemon) typedInto(p *tmux.PaneInfo) bool {
	return p.Focused() && (d.focus.client == "" || p.SessionID == d.focus.session)
}

// sweepFocus reports the pane being typed into whenever it changes (pane,
// window, session, terminal), when its mode changes, and again whenever
// its terminal regains focus. This is the stream an outer keyboard layer
// follows: "what am I typing into now". With @modal_hook_blur on, a
// terminal losing focus to another window is reported too (blur), and
// nothing is reported until one has focus again.
func (d *Daemon) sweepFocus(panes []tmux.PaneInfo) {
	focusedIn := map[string]*tmux.PaneInfo{}
	for i := range panes {
		if panes[i].Focused() {
			focusedIn[panes[i].SessionID] = &panes[i]
		}
	}
	ready := func(p *tmux.PaneInfo) *paneState {
		if p == nil {
			return nil
		}
		st, ok := d.panes[p.ID]
		if !ok || (!st.published && p.Enabled != "off") {
			return nil // not examined yet; a disabled pane reports none
		}
		return st
	}
	f := &d.focus
	if f.client != "" {
		p := focusedIn[f.session]
		st := ready(p)
		if st == nil {
			return
		}
		prev, seen := f.emitted[f.client]
		if f.blurred && d.cfg.HookBlur {
			if f.lost && seen {
				f.lost = false
				d.emitFocus("blur", "focus", f.client, p, st.cur, modeState{})
			}
			return
		}
		event := ""
		switch {
		case f.gained || !seen || prev.pane != p.ID:
			event = "focus"
		case prev.m != st.cur:
			event = "mode"
		default:
			return
		}
		f.gained = false
		f.emitted[f.client] = focusState{pane: p.ID, sid: p.SessionID, m: st.cur}
		d.emitFocus(event, "focus", f.client, p, prev.m, st.cur)
		return
	}
	// Focus not known: each terminal's pane is typed into.
	done := map[string]bool{}
	for _, c := range d.clients {
		p := focusedIn[c.SessionID]
		st := ready(p)
		if c.Control || st == nil {
			continue
		}
		prev, seen := f.emitted[c.Name]
		if seen && prev.pane == p.ID && prev.m == st.cur {
			continue
		}
		f.emitted[c.Name] = focusState{pane: p.ID, sid: p.SessionID, m: st.cur}
		if done[p.ID] {
			continue // two terminals on one session: one report
		}
		done[p.ID] = true
		event := "mode"
		if !seen || prev.pane != p.ID {
			event = "focus"
		}
		d.emitFocus(event, "c:"+c.Name, c.Name, p, prev.m, st.cur)
	}
}

func (d *Daemon) emitFocus(event, key, client string, p *tmux.PaneInfo, from, to modeState) {
	e := d.hookEvent(event, key, p, from, to, true)
	e.Client = client
	d.emit(e)
}

// stopFocus tells the subscriber detection has stopped, so nothing stays
// stuck in a commanding state the daemon can no longer vouch for.
func (d *Daemon) stopFocus() {
	f := &d.focus
	done := map[string]bool{}
	for name, s := range f.emitted {
		if (f.client != "" && name != f.client) || done[s.pane] {
			continue
		}
		done[s.pane] = true
		p := tmux.PaneInfo{ID: s.pane, SessionID: s.sid}
		if st, ok := d.panes[s.pane]; ok {
			p = st.info
		}
		d.emitFocus("stop", "c:"+name, name, &p, s.m, modeState{})
	}
}

func (d *Daemon) emit(e *HookEvent) {
	if d.sink != nil {
		d.sink(e)
		return
	}
	d.hooks.Emit(e)
}

// installFocusHooks makes a terminal's focus change wake the daemon.
// tmux < 3.3 has no such hooks; focus is then picked up at the next idle
// check.
func (d *Daemon) installFocusHooks(r tmux.Runner) {
	for _, h := range focusHooks {
		if _, err := r.Run("set-hook", "-g", h+focusHookIndex, focusPoke); err != nil {
			d.log.Debugf("set-hook %s: %v (tmux < 3.3: focus is read at idle checks)", h, err)
			return
		}
	}
}

func (d *Daemon) removeFocusHooks(r tmux.Runner) {
	for _, h := range focusHooks {
		r.Run("set-hook", "-gu", h+focusHookIndex)
	}
}
