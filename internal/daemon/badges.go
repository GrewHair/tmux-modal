package daemon

import (
	"strconv"
	"strings"
	"time"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// Badges show what the engine concluded about a pane, one fact each, so a
// wrong conclusion is visible at a glance (D35). Each is published as its
// own pane option (@modal_badge_<name>) and all of them, in this order,
// joined by spaces as @modal_badges. A badge whose template is "off" is
// never shown. The order reads as a path: over ssh, into a tmux, its
// split, the app there, its mode, the keys, the cursor.
var badgeNames = []string{"alt", "via", "nest", "split", "app", "mode", "map", "cursor", "hook"}

// Default badge templates. Placeholders: {app} {APP} {mode} {MODE}
// {evidence} {score} {via} {kind} {panes} {focus} {basis} {rule}
// {confidence} {conf} (hi/lo) {dropped} {near} {shape} {glyph} {leader}; "{ name}" and
// "{:name}" are a space or a colon and the value, or nothing when the
// value is empty.
var defaultBadges = map[string]string{
	"alt": "#[fg=colour16,bg=colour244] ALT #[default]",
	"app": "#[fg=colour255,bg=colour238] {app} {evidence}{ score} #[default]",
	// Struck through: the app that came closest without being recognised.
	"app_unknown": "#[fg=colour255,bg=colour238] ?#[fg=colour246,strikethrough]{ near}#[nostrikethrough] #[default]",
	// The mode, then how it was decided and the confidence in it (D37);
	// the _low variants put a low confidence in red. "?" is never sure:
	// its reason is grey, and a mode read but not reported (the nested
	// policy) is struck through.
	"mode_commanding":     "#[fg=colour16,bg=colour75,bold] {MODE}#[nobold]{ basis}{:rule}{ conf} #[default]",
	"mode_typing":         "#[fg=colour16,bg=colour114,bold] {MODE}#[nobold]{ basis}{:rule}{ conf} #[default]",
	"mode_unknown":        "#[fg=colour16,bg=colour244,bold] ?#[nobold,fg=colour237]{ basis}{:rule}#[strikethrough]{ dropped}#[nostrikethrough] #[default]",
	"mode_commanding_low": "#[fg=colour16,bg=colour75,bold] {MODE}#[nobold]{ basis}{:rule} #[fg=colour255,bg=colour124,bold] {conf} #[default]",
	"mode_typing_low":     "#[fg=colour16,bg=colour114,bold] {MODE}#[nobold]{ basis}{:rule} #[fg=colour255,bg=colour124,bold] {conf} #[default]",
	"via":                 "#[fg=colour16,bg=colour180] VIA {via} #[default]",
	"nest":                "#[fg=colour16,bg=colour139] NEST {kind} #[default]",
	"split":               "#[fg=colour16,bg=colour139] SPLIT {panes} {focus} #[default]",
	// Seen on screen but not acted on: no transport in the pane (D36).
	"nest_off":  "#[fg=colour244,strikethrough] NEST {kind} #[default]",
	"split_off": "#[fg=colour244,strikethrough] SPLIT {panes} {focus} #[default]",
	// The escape leader switches the client to a one-shot table; the
	// daemon never sees that, tmux does when it draws the border.
	"map": "#[fg=colour16,bg=colour220,bold] #{?#{m:modal-literal-*,#{client_key_table}},MAP {leader},MAP} #[default]",
	// The cursor drawn as the app set it: show, don't tell.
	"cursor": "#[fg=colour255,bg=colour238] {glyph} #[default]",
	// The transition hook just ran for this pane: shown for
	// @modal_hook_flash from the moment it fired, then gone.
	"hook_fired":   "#[fg=colour16,bg=colour183] HOOK … #[default]",
	"hook_ok":      "#[fg=colour16,bg=colour183] HOOK ✓{ mode}{ merged} #[default]",
	"hook_fail":    "#[fg=colour255,bg=colour124,bold] HOOK ✗ {code}{ merged} #[default]",
	"hook_timeout": "#[fg=colour255,bg=colour124,bold] HOOK ⏱{ merged} #[default]",
	// A call for a pane not typed into (MODAL_PANE_ACTIVE=0): it ran, but
	// an outer keyboard layer ignores it.
	"hook_fired_off":   "#[fg=colour244,strikethrough] HOOK … #[default]",
	"hook_ok_off":      "#[fg=colour244,strikethrough] HOOK ✓{ mode}{ merged} #[default]",
	"hook_fail_off":    "#[fg=colour244,strikethrough] HOOK ✗ {code}{ merged} #[default]",
	"hook_timeout_off": "#[fg=colour244,strikethrough] HOOK ⏱{ merged} #[default]",
}

// badgeOptions are the template options, by badge template key.
func badgeOption(key string) string { return "@modal_badge_" + key + "_format" }

// detail is what the badges show beyond the published mode: facts about the
// latest capture that do not change the mode, so they never fire a hook.
type detail struct {
	Alt        bool
	Unknown    bool // alternate screen, no spec recognised the app
	Via        string
	NestedOff  string // inner multiplexer seen but not in effect: the evidence
	ModeBasis  string
	ModeRule   string
	Dropped    string    // mode read but not reported (nested policy)
	Near       string    // the closest app not recognised, with its score
	Hook       hookFlash // set by publishBadges while it shows
	Evidence   string
	Score      string
	NestedKind string
	InnerPanes int
	FocusBy    string
	Shape      string
	Reason     string
	Remap      bool
}

// hookFlash is the hook badge of a pane: the latest call's phase, until
// it expires.
type hookFlash struct {
	id     uint64
	phase  string // fired, ok, fail, timeout; "" when not showing
	mode   string
	active bool // MODAL_PANE_ACTIVE: the pane typed into
	code   int
	merged int
	until  time.Time
}

// noteHook applies a hook note to a pane's flash: a call starting replaces
// whatever showed and starts the clock; its result replaces "fired" if it
// arrives while the badge still shows.
func (f *hookFlash) note(n hookNote, flash time.Duration) {
	if n.Phase == "fired" {
		if flash <= 0 {
			*f = hookFlash{}
			return
		}
		*f = hookFlash{id: n.ID, phase: "fired", mode: n.Mode, active: n.Active, merged: n.Merged, until: n.At.Add(flash)}
		return
	}
	if n.ID == f.id && f.phase != "" && n.At.Before(f.until) {
		f.phase, f.code = n.Phase, n.Code
	}
}

func (f *hookFlash) expire(now time.Time) {
	if f.phase != "" && !now.Before(f.until) {
		*f = hookFlash{}
	}
}

func detailOf(res classify.Result, s *screen.Screen) detail {
	d := detail{
		Alt:        s.AltScreen,
		Unknown:    res.App == "" && res.Mode == spec.ModeUnknown,
		Evidence:   res.Evidence,
		Score:      res.Score,
		NestedKind: res.NestedKind,
		Via:        res.Via,
		NestedOff:  res.NestedOff,
		ModeBasis:  res.ModeBasis,
		ModeRule:   res.ModeRule,
		Dropped:    res.Dropped,
		InnerPanes: res.InnerPanes,
		FocusBy:    res.FocusBy,
		Reason:     res.Reason,
	}
	if res.NearApp != "" {
		d.Near = res.NearApp + " " + res.NearScore
	}
	if sh := s.Cursor.Shape; sh != "" && sh != "default" {
		d.Shape = sh
	}
	return d
}

// badgeValues are the raw facts, published as pane options next to the
// badges so formats can use them directly.
var detailOptions = []string{
	"@modal_alt", "@modal_via", "@modal_nested_off", "@modal_mode_basis", "@modal_mode_rule",
	"@modal_evidence", "@modal_score", "@modal_nested_kind",
	"@modal_split", "@modal_focus_by", "@modal_remap", "@modal_cursor_shape", "@modal_reason",
}

func (det detail) values() []string {
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return ""
	}
	split := ""
	if det.InnerPanes > 0 {
		split = strconv.Itoa(det.InnerPanes)
	}
	return []string{onOff(det.Alt), det.Via, det.NestedOff, det.ModeBasis, det.ModeRule, det.Evidence, det.Score, det.NestedKind, split,
		focusWord(det), onOff(det.Remap), det.Shape, det.Reason}
}

func focusWord(det detail) string {
	switch {
	case det.InnerPanes == 0:
		return ""
	case det.FocusBy == "border colour":
		return "border"
	case det.FocusBy == "":
		return "?"
	}
	return det.FocusBy
}

// renderBadges renders every badge for a pane, in badgeNames order; an
// empty string is a badge not shown.
func renderBadges(tpl map[string]string, m modeState, det detail, leader string) []string {
	vals := map[string]string{
		"app": m.App, "APP": strings.ToUpper(m.App),
		"mode": m.Mode, "MODE": strings.ToUpper(m.Mode),
		"evidence": det.Evidence, "score": det.Score,
		"basis": det.ModeBasis, "rule": det.ModeRule, "confidence": m.Confidence,
		"conf": map[string]string{"high": "hi", "low": "lo"}[m.Confidence],
		"via":  det.Via, "kind": det.NestedKind, "panes": "", "focus": focusWord(det),
		"code": strconv.Itoa(det.Hook.code), "merged": "",
		"dropped": det.Dropped, "near": det.Near,
		"shape": det.Shape, "glyph": cursorGlyphs[det.Shape], "leader": formatEscape(leader),
	}
	if det.InnerPanes > 0 {
		vals["panes"] = strconv.Itoa(det.InnerPanes)
	}
	if det.Hook.merged > 1 {
		vals["merged"] = "×" + strconv.Itoa(det.Hook.merged)
	}
	out := make([]string, len(badgeNames))
	for i, name := range badgeNames {
		key := name
		show := false
		switch name {
		case "alt":
			show = det.Alt
		case "app":
			show = m.App != "" || det.Unknown
			if m.App == "" {
				key = "app_unknown"
			}
		case "mode":
			show = m.App != "" && m.Bucket != spec.ModeNone
			key = "mode_" + m.Bucket
			if det.ModeBasis == "" || m.Bucket == spec.ModeUnknown {
				vals["conf"] = "" // with its basis; "?" is never sure
			} else if _, ok := tpl[key+"_low"]; ok && m.Confidence == "low" {
				key += "_low"
			}
		case "via":
			show = det.Via != ""
		case "nest":
			show = det.NestedKind != ""
			if det.NestedOff != "" {
				key = "nest_off"
			}
		case "split":
			show = det.InnerPanes > 0
			if det.NestedOff != "" {
				key = "split_off"
			}
		case "map":
			show = det.Remap
		case "cursor":
			show = det.Shape != ""
		case "hook":
			show = det.Hook.phase != ""
			key = "hook_" + det.Hook.phase
			if _, ok := tpl[key+"_off"]; ok && !det.Hook.active {
				key += "_off"
			}
			if det.Hook.phase != "" {
				vals["mode"] = det.Hook.mode // what the hook was told
			}
		}
		t, ok := tpl[key]
		if !show || !ok || t == "" {
			continue
		}
		out[i] = expand(t, vals)
	}
	return out
}

// cursorGlyphs draw the DECSCUSR shapes tmux reports (#{cursor_shape}).
var cursorGlyphs = map[string]string{"block": "█", "underline": "▁", "bar": "▏"}

// expand substitutes {name}, "{ name}" and "{:name}" placeholders.
func expand(t string, vals map[string]string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(t, '{')
		if i < 0 {
			b.WriteString(t)
			return b.String()
		}
		j := strings.IndexByte(t[i:], '}')
		if j < 0 {
			b.WriteString(t)
			return b.String()
		}
		name := t[i+1 : i+j]
		if k := strings.LastIndexByte(name, '{'); k >= 0 {
			// Inside a tmux format: move on to the innermost brace.
			b.WriteString(t[:i+1+k])
			t = t[i+1+k:]
			continue
		}
		// "{ name}" and "{:name}": the separator and the value, or nothing.
		sep := ""
		if strings.HasPrefix(name, " ") || strings.HasPrefix(name, ":") {
			sep, name = name[:1], name[1:]
		}
		v, ok := vals[name]
		b.WriteString(t[:i])
		switch {
		case !ok: // not ours (a tmux format's braces): keep as is
			b.WriteString(t[i : i+j+1])
		case v != "":
			b.WriteString(sep + v)
		}
		t = t[i+j+1:]
	}
}

// formatEscape makes a literal safe inside a tmux format conditional.
func formatEscape(s string) string {
	return strings.NewReplacer("#", "##", ",", "#,", "}", "#}").Replace(s)
}

func joinBadges(bs []string) string {
	var shown []string
	for _, b := range bs {
		if b != "" {
			shown = append(shown, b)
		}
	}
	return strings.Join(shown, " ")
}

// PublishedOptions are every per-pane option the daemon publishes,
// @modal_app first.
func PublishedOptions() []string {
	return append(append([]string{}, paneOptions...), badgePaneOptions()...)
}

// badgePaneOptions are every option publishBadges writes.
func badgePaneOptions() []string {
	opts := append([]string{}, detailOptions...)
	for _, n := range badgeNames {
		opts = append(opts, "@modal_badge_"+n)
	}
	return append(opts, "@modal_badges")
}

// publishBadges brings every pane's badge and detail options up to date,
// sending only the ones that changed. It runs after the key tables are
// synced, which decide the map badge: the focused pane of a session whose
// key table the daemon switched to that pane's app.
func (d *Daemon) publishBadges(panes []tmux.PaneInfo, humans map[string]bool) {
	names := badgePaneOptions()
	now := time.Now()
	for _, n := range d.ev.takeHooks() {
		if st, ok := d.panes[n.Pane]; ok {
			st.hook.note(n, d.cfg.HookFlash)
		}
	}
	var cmds []string
	for i := range panes {
		p := &panes[i]
		st, ok := d.panes[p.ID]
		if !ok {
			continue
		}
		st.hook.expire(now)
		det := st.det
		det.Hook = st.hook
		det.Remap = false
		if ss := d.sessions[p.SessionID]; ss != nil && ss.own != "" && p.Focused() && humans[p.SessionID] &&
			st.cur.App != "" && ss.own == TableName(st.cur.App) {
			det.Remap = true
		}
		var vals []string
		if st.published {
			leader := d.cfg.Leader
			if sp := d.set.Specs[st.cur.App]; sp != nil && sp.EscapeLeader != "" {
				leader = sp.EscapeLeader
			}
			bs := renderBadges(d.cfg.Badges, st.cur, det, leader)
			vals = append(append(det.values(), bs...), joinBadges(bs))
		} else {
			vals = make([]string, len(names))
		}
		if st.badgeVals == nil {
			st.badgeVals = make([]string, len(names))
		}
		for j, o := range names {
			if vals[j] == st.badgeVals[j] {
				continue
			}
			if vals[j] == "" {
				cmds = append(cmds, tmux.Command("set-option", "-p", "-u", "-q", "-t", p.ID, o))
			} else {
				cmds = append(cmds, tmux.Command("set-option", "-p", "-t", p.ID, o, vals[j]))
			}
			if o == "@modal_badges" || strings.HasPrefix(o, "@modal_badge_") {
				d.statusDirty = true
			}
		}
		st.badgeVals = vals
	}
	d.run(d.runner(), cmds)
}
