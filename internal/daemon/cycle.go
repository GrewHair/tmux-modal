package daemon

import (
	"path/filepath"
	"time"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// modeState is what the daemon publishes for a pane.
type modeState struct {
	App, Mode, Bucket, Confidence string
}

var paneOptions = []string{"@modal_app", "@modal_mode", "@modal_bucket", "@modal_confidence"}

type paneState struct {
	id   string
	info tmux.PaneInfo
	seen bool

	app        string // sticky identity
	identFails int

	cur       modeState
	published bool
	pending   modeState
	pendingN  int

	due        time.Time
	burstStart time.Time
	lastOutput time.Time
	lastExam   time.Time
	inScope    bool
	altToggles int
}

const identFailLimit = 3

func (d *Daemon) pane(id string) *paneState {
	st, ok := d.panes[id]
	if !ok {
		st = &paneState{id: id}
		d.panes[id] = st
	}
	return st
}

func (d *Daemon) burst() time.Duration {
	return time.Duration(float64(d.cfg.Burst) * d.throttle)
}

func (d *Daemon) poll() time.Duration {
	return time.Duration(float64(d.cfg.Poll) * d.throttle)
}

// onOutput schedules an examination once output has settled: a trailing
// debounce of one burst interval, capped at one poll interval after the
// burst began so continuous output cannot starve detection. Settling
// before capturing is also the guard against mid-redraw screens.
func (d *Daemon) onOutput(st *paneState, t time.Time) {
	st.lastOutput = t
	if st.burstStart.IsZero() {
		st.burstStart = t
	}
	due := t.Add(d.burst())
	if limit := st.burstStart.Add(d.poll()); due.After(limit) {
		due = limit
	}
	if floor := st.lastExam.Add(d.burst()); due.Before(floor) {
		due = floor
	}
	st.due = due
}

func (d *Daemon) drainEvents() {
	out, notes, overflow := d.ev.take()
	for id, t := range out {
		d.onOutput(d.pane(id), t)
	}
	for _, n := range notes {
		switch n.Name {
		case "window-pane-changed", "session-window-changed", "pane-mode-changed",
			"layout-change", "window-add", "window-close", "unlinked-window-close":
			// focus or layout moved: the next cycle re-reads tier 1 and
			// re-syncs key tables; newly focused panes are due at once.
		case "client-session-changed", "client-detached", "sessions-changed",
			"session-changed", "exit":
			d.forceReconcile = true
		}
	}
	if overflow {
		d.forceReconcile = true
	}
}

func (d *Daemon) inScope(p *tmux.PaneInfo, humans map[string]bool) bool {
	if !humans[p.SessionID] {
		return false
	}
	switch d.cfg.Scope {
	case "all":
		return true
	case "visible":
		return p.WindowActive
	}
	return p.Focused()
}

// cycle is one scheduler pass: a single batched tier-1 read, gates, then
// pipelined tier-2 captures for the panes that are due.
func (d *Daemon) cycle() {
	now := time.Now()
	r := d.runner()
	panes, err := tmux.ListPanes(r)
	if err != nil {
		d.log.Debugf("list-panes: %v", err)
		return
	}
	humans := map[string]bool{}
	for _, c := range d.clients {
		if !c.Control {
			humans[c.SessionID] = true
		}
	}

	type job struct {
		st     *paneState
		s      *screen.Screen
		bottom int
		colour bool
		wait   func() ([]string, error)
	}
	var jobs []*job

	// A linked window lists its panes once per session; keep one listing
	// per pane, preferring one that is in scope.
	byID := map[string]int{}
	var order []int
	for i := range panes {
		p := &panes[i]
		if j, ok := byID[p.ID]; ok {
			if !d.inScope(&panes[j], humans) && d.inScope(p, humans) {
				byID[p.ID] = i
			}
			continue
		}
		byID[p.ID] = i
		order = append(order, i)
	}
	seen := map[string]bool{}
	for _, i := range order {
		p := &panes[byID[panes[i].ID]]
		seen[p.ID] = true
		st := d.pane(p.ID)
		prev := st.info
		st.info = *p
		enabled := d.cfg.Enabled && p.Enabled != "off"
		scope := enabled && d.inScope(p, humans)

		if !enabled {
			if st.published {
				d.publish(st, modeState{}, "disabled")
			}
			st.app = ""
			continue
		}
		// Tier-1 invalidation signals.
		if st.seen {
			if prev.Alt != p.Alt {
				st.altToggles++
				st.app, st.due = "", now
			}
			if prev.Command != p.Command {
				st.app, st.due = "", now
			}
			if prev.Width != p.Width || prev.Height != p.Height {
				st.due = now
			}
		}
		st.seen = true
		if scope && !st.inScope {
			st.due = now // newly focused: examine at once
		}
		st.inScope = scope
		if !scope || now.Before(st.due) {
			continue
		}
		st.lastExam, st.burstStart = now, time.Time{}

		// Tier-1 gate: a local shell on the primary screen runs no TUI.
		if p.Dead || (!p.Alt && classify.Shells[filepath.Base(p.Command)] && !d.primaryScreenSpecs()) {
			st.app, st.identFails = "", 0
			d.transition(st, modeState{Mode: spec.ModeNone, Bucket: spec.ModeNone, Confidence: classify.High}, "tier-1: shell on primary screen", now)
			st.due = now.Add(d.cfg.Idle)
			continue
		}

		// Tier 2: one capture serves identification and mode detection.
		j := &job{st: st, s: p.Screen()}
		var sp *spec.Spec
		if st.app != "" {
			sp = d.set.Specs[st.app]
		}
		if sp != nil && st.identFails == 0 && sp.CaptureBottom > 0 {
			j.bottom = sp.CaptureBottom
			if d.cfg.CaptureRows > j.bottom {
				j.bottom = d.cfg.CaptureRows
			}
		}
		if d.cfg.Colour {
			j.colour = (sp != nil && sp.NeedsColour) || (sp == nil && d.anyColour())
		}
		args := tmux.CaptureArgs(p.ID, p.Height, j.bottom, j.colour)
		if c, ok := r.(*tmux.Control); ok {
			ch, err := c.Send(tmux.Command(args...))
			if err != nil {
				continue
			}
			j.wait = func() ([]string, error) { return tmux.Wait(ch) }
		} else {
			lines, err := r.Run(args...)
			j.wait = func() ([]string, error) { return lines, err }
		}
		jobs = append(jobs, j)
	}

	for _, j := range jobs {
		lines, err := j.wait()
		if err != nil {
			d.log.Debugf("capture %s: %v", j.st.id, err)
			continue
		}
		tmux.Fill(j.s, lines, j.bottom, j.colour)
		d.examine(j.st, j.s, j.bottom == 0, now)
	}

	for id, st := range d.panes {
		if !seen[id] {
			delete(d.panes, id)
		} else if !st.inScope {
			st.pendingN = 0
		}
	}
	d.syncKeyTables(r, panes, humans)
}

// examine classifies one captured pane and applies the result.
func (d *Daemon) examine(st *paneState, s *screen.Screen, full bool, now time.Time) {
	res, _ := classify.Pane(d.set, s, st.app)
	if st.app != "" && !res.Confirmed {
		// Sticky identity not re-confirmed. If this capture is the full
		// screen, see whether another app now owns the pane.
		if full {
			if alt, _ := classify.Pane(d.set, s, ""); alt.App != "" && alt.App != st.app && alt.Confirmed {
				d.log.Infof("%s: identity %s -> %s", st.id, st.app, alt.App)
				res, st.app, st.identFails = alt, alt.App, 0
			}
		}
		if res.App == st.app && !res.Confirmed {
			st.identFails++
			if st.identFails >= identFailLimit {
				d.log.Infof("%s: %s not seen for %d captures; re-identifying", st.id, st.app, st.identFails)
				st.app, st.identFails = "", 0
			}
		}
	} else {
		st.identFails = 0
		if res.App != "" {
			st.app = res.App
		}
	}

	next := modeState{App: res.App, Mode: res.Mode, Bucket: res.Bucket, Confidence: res.Confidence}
	d.transition(st, next, res.Reason, now)

	switch {
	case st.pendingN > 0:
		st.due = now.Add(d.burst())
	case next.Mode == spec.ModeUnknown || st.identFails > 0:
		st.due = now.Add(d.poll())
	default:
		st.due = now.Add(d.cfg.Idle)
	}
}

// transition applies a classification. Text-entry modes are entered
// eagerly; returning to a remapped commanding mode needs Confirm agreeing
// captures, because a false commanding steals keystrokes while a false
// typing merely loses the remap (§4.3). Hooks-only specs remap nothing,
// so for them every edge is taken at once (§4.4 rule 2).
func (d *Daemon) transition(st *paneState, next modeState, reason string, now time.Time) {
	if next == st.cur && st.published {
		st.pendingN = 0
		return
	}
	sp := d.set.Specs[next.App]
	remap := sp != nil && sp.HasKeys() && next.Bucket == spec.BucketCommanding
	wasRemap := st.cur.App == next.App && st.cur.Bucket == spec.BucketCommanding
	if remap && !wasRemap {
		if st.pending == next {
			st.pendingN++
		} else {
			st.pending, st.pendingN = next, 1
		}
		if st.pendingN < d.cfg.Confirm {
			d.log.Debugf("%s: %s/%s pending confirmation %d/%d", st.id, next.App, next.Mode, st.pendingN, d.cfg.Confirm)
			return
		}
	}
	st.pendingN = 0
	prev := st.cur
	d.publish(st, next, reason)
	d.log.Infof("%s: %s/%s -> %s/%s (%s, %s)", st.id, orNone(prev.App), orNone(prev.Mode),
		orNone(next.App), next.Mode, next.Confidence, reason)
}

func orNone(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// publish writes a pane's state to its @modal_* options.
func (d *Daemon) publish(st *paneState, m modeState, reason string) {
	st.cur, st.published = m, m != (modeState{})
	vals := []string{m.App, m.Mode, m.Bucket, m.Confidence}
	var cmds []string
	for i, o := range paneOptions {
		if vals[i] == "" {
			cmds = append(cmds, tmux.Command("set-option", "-p", "-u", "-q", "-t", st.id, o))
		} else {
			cmds = append(cmds, tmux.Command("set-option", "-p", "-t", st.id, o, vals[i]))
		}
	}
	d.run(d.runner(), cmds)
}

func (d *Daemon) primaryScreenSpecs() bool {
	for _, sp := range d.set.Order {
		if !sp.Identity.RequiresAlt {
			return true
		}
	}
	return false
}

func (d *Daemon) anyColour() bool {
	for _, sp := range d.set.Order {
		if sp.NeedsColour {
			return true
		}
	}
	return false
}

// nextWake is how long to sleep: until the earliest due pane in scope, or
// the next reconcile.
func (d *Daemon) nextWake(lastReconcile time.Time) time.Duration {
	now := time.Now()
	next := lastReconcile.Add(d.cfg.Idle)
	if d.forceReconcile {
		return 0
	}
	for _, st := range d.panes {
		if st.inScope && st.due.Before(next) {
			next = st.due
		}
	}
	return next.Sub(now)
}
