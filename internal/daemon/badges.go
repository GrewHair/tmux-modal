package daemon

import (
	"strconv"
	"strings"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// Badges show what the engine concluded about a pane, one fact each, so a
// wrong conclusion is visible at a glance (D35). Each is published as its
// own pane option (@modal_badge_<name>) and all of them, in this order,
// joined by spaces as @modal_badges. A badge whose template is "off" is
// never shown.
var badgeNames = []string{"alt", "app", "mode", "nest", "split", "map", "cursor"}

// Default badge templates. Placeholders: {app} {APP} {mode} {MODE}
// {evidence} {score} {kind} {panes} {focus} {shape} {leader}; "{ name}"
// is a space and the value, or nothing when the value is empty.
var defaultBadges = map[string]string{
	"alt":             "#[fg=black,bg=colour244] ALT #[default]",
	"app":             "#[fg=colour255,bg=colour238] {app} {evidence}{ score} #[default]",
	"app_unknown":     "#[fg=colour255,bg=colour238] ? #[default]",
	"mode_commanding": "#[fg=black,bg=green,bold] {MODE} #[default]",
	"mode_typing":     "#[fg=black,bg=yellow,bold] {MODE} #[default]",
	"mode_unknown":    "#[fg=black,bg=colour244] ? #[default]",
	"nest":            "#[fg=black,bg=colour110] NEST {kind} #[default]",
	"split":           "#[fg=black,bg=colour110] SPLIT {panes} {focus} #[default]",
	// The escape leader switches the client to a one-shot table; the
	// daemon never sees that, tmux does when it draws the border.
	"map":    "#[fg=black,bg=cyan,bold] #{?#{m:modal-literal-*,#{client_key_table}},MAP {leader},MAP} #[default]",
	"cursor": "#[fg=colour255,bg=colour238] {shape} #[default]",
}

// badgeOptions are the template options, by badge template key.
func badgeOption(key string) string { return "@modal_badge_" + key + "_format" }

// detail is what the badges show beyond the published mode: facts about the
// latest capture that do not change the mode, so they never fire a hook.
type detail struct {
	Alt        bool
	Unknown    bool // alternate screen, no spec recognised the app
	Evidence   string
	Score      string
	NestedKind string
	InnerPanes int
	FocusBy    string
	Shape      string
	Reason     string
	Remap      bool
}

func detailOf(res classify.Result, s *screen.Screen) detail {
	d := detail{
		Alt:        s.AltScreen,
		Unknown:    res.App == "" && res.Mode == spec.ModeUnknown,
		Evidence:   res.Evidence,
		Score:      res.Score,
		NestedKind: res.NestedKind,
		InnerPanes: res.InnerPanes,
		FocusBy:    res.FocusBy,
		Reason:     res.Reason,
	}
	if sh := s.Cursor.Shape; sh != "" && sh != "default" {
		d.Shape = sh
	}
	return d
}

// badgeValues are the raw facts, published as pane options next to the
// badges so formats can use them directly.
var detailOptions = []string{
	"@modal_alt", "@modal_evidence", "@modal_score", "@modal_nested_kind",
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
	return []string{onOff(det.Alt), det.Evidence, det.Score, det.NestedKind, split,
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
		"kind": det.NestedKind, "panes": "", "focus": focusWord(det),
		"shape": det.Shape, "leader": formatEscape(leader),
	}
	if det.InnerPanes > 0 {
		vals["panes"] = strconv.Itoa(det.InnerPanes)
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
		case "nest":
			show = det.NestedKind != ""
		case "split":
			show = det.InnerPanes > 0
		case "map":
			show = det.Remap
		case "cursor":
			show = det.Shape != ""
		}
		t, ok := tpl[key]
		if !show || !ok || t == "" {
			continue
		}
		out[i] = expand(t, vals)
	}
	return out
}

// expand substitutes {name} and "{ name}" placeholders.
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
		space := strings.HasPrefix(name, " ")
		v, ok := vals[strings.TrimPrefix(name, " ")]
		b.WriteString(t[:i])
		switch {
		case !ok: // not ours (a tmux format's braces): keep as is
			b.WriteString(t[i : i+j+1])
		case space && v != "":
			b.WriteString(" " + v)
		case !space:
			b.WriteString(v)
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
	var cmds []string
	for i := range panes {
		p := &panes[i]
		st, ok := d.panes[p.ID]
		if !ok {
			continue
		}
		det := st.det
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
