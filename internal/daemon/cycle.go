package daemon

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// modeState is what the daemon publishes for a pane.
type modeState struct {
	App, Mode, Bucket, Confidence string
	Nested                        string // evidence of an inner multiplexer, or ""
}

var paneOptions = []string{"@modal_app", "@modal_mode", "@modal_bucket", "@modal_confidence", "@modal_nested", "@modal_indicator"}

type paneState struct {
	id   string
	info tmux.PaneInfo
	seen bool

	app        string // sticky identity
	identFails int

	cur       modeState
	published bool
	indicator string
	det       detail   // badge facts from the latest capture
	badgeVals []string // badge and detail options as last published
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

// pollFor is poll for one pane: a spec's own poll_interval, once the pane
// is identified as that app, wins over @modal_poll_interval.
func (d *Daemon) pollFor(st *paneState) time.Duration {
	if sp := d.set.Specs[st.app]; sp != nil && sp.PollInterval > 0 {
		return time.Duration(float64(sp.PollInterval) * float64(time.Millisecond) * d.throttle)
	}
	return d.poll()
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
	if limit := st.burstStart.Add(d.pollFor(st)); due.After(limit) {
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
			st.app, st.det = "", detail{}
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
		// Primary-screen specs (REPLs) are recognised by their own command
		// name, never a shell's, so this holds even when some are loaded.
		if p.Dead || (!p.Alt && classify.Shells[filepath.Base(p.Command)]) {
			st.app, st.identFails, st.det = "", 0, detail{}
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
		// A nested pane needs the full screen: the inner status line and
		// pane borders are what keep it from being remapped.
		if sp != nil && st.identFails == 0 && sp.CaptureBottom > 0 && st.cur.Nested == "" {
			j.bottom = sp.CaptureBottom
			if d.cfg.CaptureRows > j.bottom {
				j.bottom = d.cfg.CaptureRows
			}
		}
		// A nested pane always gets colour: across an inner split, tmux's
		// green active border shows which inner pane has the keyboard.
		if d.cfg.Colour {
			j.colour = (sp != nil && sp.NeedsColour) || (sp == nil && d.anyColour()) || st.cur.Nested != ""
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
	d.publishBadges(panes, humans)
	d.syncOutput(panes, humans)
	d.sweepFocus(panes, humans)
	d.refreshStatus(r)
}

// examine classifies one captured pane and applies the result.
func (d *Daemon) examine(st *paneState, s *screen.Screen, full bool, now time.Time) {
	opt := classify.Options{NestedRemap: d.cfg.NestedRemap}
	res, _ := classify.PaneWith(d.set, s, st.app, opt)
	if st.app != "" && !res.Confirmed {
		// Sticky identity not re-confirmed. If this capture is the full
		// screen, see whether another app now owns the pane.
		if full {
			if alt, _ := classify.PaneWith(d.set, s, "", opt); alt.App != "" && alt.App != st.app && alt.Confirmed {
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

	next := modeState{App: res.App, Mode: res.Mode, Bucket: res.Bucket, Confidence: res.Confidence, Nested: res.Nested}
	st.det = detailOf(res, s)
	d.transition(st, next, res.Reason, now)

	switch {
	case st.pendingN > 0:
		st.due = now.Add(d.burst())
	case next.Mode == spec.ModeUnknown || st.identFails > 0:
		st.due = now.Add(d.pollFor(st))
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
	// The focused pane of each session is reported by sweepFocus, which
	// also covers focus moving between panes; here only the others.
	if !st.info.Focused() {
		d.hooks.Emit(d.hookEvent("mode", "p:"+st.id, &st.info, prev, next, false))
	}
}

func orNone(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// publish writes a pane's state to its @modal_* options, including the
// ready-rendered @modal_indicator, so status and border formats only ever
// read variables.
func (d *Daemon) publish(st *paneState, m modeState, reason string) {
	st.cur, st.published = m, m != (modeState{})
	ind := d.indicator(m)
	if ind != st.indicator {
		st.indicator = ind
		d.statusDirty = true
	}
	vals := []string{m.App, m.Mode, m.Bucket, m.Confidence, m.Nested, ind}
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

// indicator renders the indicator template for a state.
func (d *Daemon) indicator(m modeState) string {
	if m == (modeState{}) {
		return ""
	}
	tpl := d.cfg.Indicator[m.Bucket]
	return strings.NewReplacer(
		"{MODE}", strings.ToUpper(m.Mode), "{mode}", m.Mode,
		"{APP}", strings.ToUpper(m.App), "{app}", m.App,
		"{bucket}", m.Bucket, "{confidence}", m.Confidence,
	).Replace(tpl)
}

// republishIndicators re-renders every pane's indicator after the
// templates changed.
func (d *Daemon) republishIndicators() {
	var cmds []string
	for _, st := range d.panes {
		if !st.published {
			continue
		}
		if ind := d.indicator(st.cur); ind != st.indicator {
			st.indicator = ind
			cmds = append(cmds, tmux.Command("set-option", "-p", "-t", st.id, "@modal_indicator", ind))
		}
	}
	if len(cmds) > 0 {
		d.run(d.runner(), cmds)
		d.statusDirty = true
	}
}

// refreshStatus redraws the status lines of human clients once per cycle
// when something they may display changed. Pane borders redraw by
// themselves when a pane option changes; status lines do not.
func (d *Daemon) refreshStatus(r tmux.Runner) {
	if !d.statusDirty {
		return
	}
	d.statusDirty = false
	var cmds []string
	for _, c := range d.clients {
		if !c.Control {
			cmds = append(cmds, tmux.Command("refresh-client", "-S", "-t", c.Name))
		}
	}
	d.run(r, cmds)
}

// focusState is what the hook last reported for a session's focused pane.
type focusState struct {
	pane string
	m    modeState
}

// sweepFocus reports each session's focused pane whenever its pane or its
// mode changes. This is the stream an outer keyboard layer follows: it
// needs "what am I typing into now", which changes on focus moves as well
// as on mode changes.
func (d *Daemon) sweepFocus(panes []tmux.PaneInfo, humans map[string]bool) {
	for i := range panes {
		p := &panes[i]
		if !p.Focused() || !humans[p.SessionID] {
			continue
		}
		st, ok := d.panes[p.ID]
		if !ok || (!st.published && p.Enabled != "off") {
			continue // not examined yet; a disabled pane reports none
		}
		prev, seen := d.focusEmitted[p.SessionID]
		if seen && prev.pane == p.ID && prev.m == st.cur {
			continue
		}
		event := "mode"
		if !seen || prev.pane != p.ID {
			event = "focus"
		}
		d.focusEmitted[p.SessionID] = focusState{pane: p.ID, m: st.cur}
		d.hooks.Emit(d.hookEvent(event, "s:"+p.SessionID, p, prev.m, st.cur, true))
	}
}

// hookEvent builds a hook event; the global hook runs first, then the
// hook of the spec the pane is now in.
func (d *Daemon) hookEvent(event, key string, p *tmux.PaneInfo, from, to modeState, active bool) *HookEvent {
	e := &HookEvent{
		Event: event, Key: key, Pane: p.ID, App: to.App, AppFrom: from.App,
		ModeTo: to.Mode, ModeFrom: from.Mode, Bucket: to.Bucket, Confidence: to.Confidence, Nested: to.Nested,
		Session: p.SessionName, SessionID: p.SessionID, Window: p.WindowID, Active: active,
	}
	if e.ModeTo == "" {
		e.ModeTo, e.Bucket = spec.ModeNone, spec.ModeNone
	}
	if d.cfg.Hook != "" {
		e.Commands = append(e.Commands, d.cfg.Hook)
	}
	if sp := d.set.Specs[to.App]; sp != nil && sp.Hook != "" {
		e.Commands = append(e.Commands, sp.Hook)
	}
	return e
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
