// Package validate renders the spec-authoring report: the resolved spec,
// lint warnings, and a per-clause score sheet against a screen.
package validate

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
)

// Options controls the report.
type Options struct {
	ShowResolved bool
	LocaleAudit  bool
	Sticky       bool
}

// Spec writes the static part of the report for one spec.
func Spec(w io.Writer, sp *spec.Spec, opt Options) {
	kind := "app"
	if sp.Group {
		kind = "group"
	}
	fmt.Fprintf(w, "%s %s  (%s)\n", kind, sp.Name, sp.File)
	if len(sp.Chain) > 1 {
		fmt.Fprintf(w, "  extends:  %s\n", strings.Join(sp.Chain, " -> "))
	}
	fmt.Fprintf(w, "  priority: %d\n", sp.Priority)
	if len(sp.Modes) > 0 {
		var bs []string
		for _, m := range sp.Modes {
			bs = append(bs, m+"="+sp.Bucket(m))
		}
		fmt.Fprintf(w, "  modes:    %s\n", strings.Join(bs, " "))
	}
	if sp.Always != "" {
		fmt.Fprintf(w, "  always:   %s\n", sp.Always)
	} else if !sp.Group {
		fmt.Fprintf(w, "  default:  %s when identity is confirmed, otherwise %s\n", sp.DefaultMode, sp.OtherwiseMode)
	}
	if sp.HasKeys() {
		var ks []string
		for _, k := range sp.Keys {
			ks = append(ks, k.Key+"->"+k.Send)
		}
		fmt.Fprintf(w, "  keys:     %s\n", strings.Join(ks, " "))
		leader := sp.EscapeLeader
		if leader == "" {
			leader = "(global @modal_escape_leader)"
		}
		fmt.Fprintf(w, "  leader:   %s\n", leader)
		if len(sp.Shadowed) > 0 {
			var sh []string
			for k, v := range sp.Shadowed {
				sh = append(sh, k+"="+v)
			}
			sort.Strings(sh)
			fmt.Fprintf(w, "  shadows:  %s\n", strings.Join(sh, " "))
		}
	} else if !sp.Group {
		fmt.Fprintf(w, "  keys:     none (hooks-only: the session key-table is never touched)\n")
	}
	capture := "full screen"
	if sp.CaptureBottom > 0 {
		capture = fmt.Sprintf("bottom %d rows once identity is sticky", sp.CaptureBottom)
	}
	if sp.NeedsColour {
		capture += ", with colour (-e)"
	}
	fmt.Fprintf(w, "  capture:  %s\n", capture)
	tiers := []string{}
	for _, r := range sp.ModeRules {
		tiers = append(tiers, fmt.Sprintf("%s:tier%d", label(r), r.Tier))
	}
	if len(tiers) > 0 {
		fmt.Fprintf(w, "  rules:    %s\n", strings.Join(tiers, " "))
	}

	warnings := append([]string{}, sp.Warnings...)
	if opt.LocaleAudit {
		for _, a := range sp.LocaleAudit() {
			warnings = append(warnings, "locale: "+a)
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintf(w, "\nwarnings:\n")
		for _, x := range warnings {
			fmt.Fprintf(w, "  - %s\n", x)
		}
	}
	if opt.ShowResolved {
		fmt.Fprintf(w, "\nresolved spec (after inheritance):\n")
		var b strings.Builder
		doc := map[string]any{}
		for k, v := range sp.Resolved {
			doc[k] = v
		}
		doc["name"] = sp.Name
		if err := toml.NewEncoder(&b).Encode(doc); err != nil {
			fmt.Fprintf(w, "  (cannot render: %v)\n", err)
		}
		for _, l := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
			fmt.Fprintf(w, "  | %s\n", l)
		}
	}
}

func label(r *spec.Rule) string {
	if r.Name != "" {
		return r.Mode + "/" + r.Name
	}
	return r.Mode
}

// Screen writes a one-line description of a screen.
func Screen(w io.Writer, src string, s *screen.Screen) {
	shape := s.Cursor.Shape
	if shape == "" {
		shape = "unreported"
	}
	colour := "no"
	if s.Cells != nil {
		colour = "yes"
	}
	fmt.Fprintf(w, "screen: %s\n  %dx%d  cursor %d,%d visible=%v shape=%s  alt=%v  command=%q  colour=%s\n",
		src, s.Width, s.Height, s.Cursor.X, s.Cursor.Y, s.Cursor.Visible, shape, s.AltScreen, s.Command, colour)
}

// Trace writes the score sheet of one spec's evaluation.
func Trace(w io.Writer, t classify.Trace) {
	id := t.Identity
	fmt.Fprintf(w, "\n== %s: identity\n", t.Spec.Name)
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	if t.Spec.Identity.RequiresAlt {
		fmt.Fprintf(w, "  alternate screen required: %s\n", map[bool]string{true: "ok", false: "NOT ON -> cannot match"}[id.AltOK])
	}
	if len(t.Spec.Identity.Commands) > 0 {
		fmt.Fprintf(w, "  command %v: %s (local fast path only; an SSH pane reports ssh)\n", t.Spec.Identity.Commands, yn(id.CommandMatch))
	}
	if t.Spec.Identity.Title != nil {
		fmt.Fprintf(w, "  title /%s/: %s\n", t.Spec.Identity.Title, yn(id.TitleMatch))
	}
	if id.Screen != nil {
		rule(w, "screen", *id.Screen)
	}
	state := "not identified"
	switch {
	case id.Confirmed:
		state = "identified and CONFIRMED on this capture"
	case id.Identified:
		state = "identified, but NOT confirmed on this capture (absence cannot conclude a mode)"
	}
	fmt.Fprintf(w, "  => %s\n", state)

	if len(t.ModeRules) > 0 {
		fmt.Fprintf(w, "\n== %s: mode rules (first hit wins)\n", t.Spec.Name)
		for i, rr := range t.ModeRules {
			rule(w, fmt.Sprintf("[%d] %s", i, label(rr.Rule)), rr)
		}
	}
	if len(t.Corroborate) > 0 {
		fmt.Fprintf(w, "\n== %s: corroboration\n", t.Spec.Name)
		for i, rr := range t.Corroborate {
			rule(w, fmt.Sprintf("[%d]", i), rr)
		}
	}
	r := t.Result
	fmt.Fprintf(w, "\n=> app=%s mode=%s bucket=%s confidence=%s\n   %s\n",
		orDash(r.App), r.Mode, r.Bucket, r.Confidence, r.Reason)
}

func rule(w io.Writer, name string, rr spec.RuleResult) {
	fmt.Fprintf(w, "  %s  (combine=%s threshold=%g)\n", name, rr.Rule.Combine, rr.Rule.Threshold)
	var running float64
	for _, cr := range rr.Clauses {
		c := cr.Clause
		contrib := "     "
		if cr.State == spec.Satisfied && c.WeightKind == spec.WeightNumeric {
			running += c.Weight
			contrib = fmt.Sprintf("+%-4g", c.Weight)
		}
		fmt.Fprintf(w, "    %-4s %s w=%-8s total=%-4g %s\n", cr.State, contrib, c.WeightString(), running, c.Describe())
		for _, h := range cr.Hits {
			col := ""
			if h.Color != "" {
				col = "  " + h.Color
			}
			fmt.Fprintf(w, "           @ row %d col %d  %q%s\n", h.Row, h.Col, h.Text, col)
		}
		if cr.Detail != "" {
			fmt.Fprintf(w, "           (%s)\n", cr.Detail)
		}
	}
	verdict := "no"
	switch {
	case rr.Fired:
		verdict = "FIRED"
	case rr.RequiredMissing:
		verdict = "no (a required clause is missing)"
	}
	fmt.Fprintf(w, "    score %g / %g -> %s\n", rr.Score, rr.Rule.Threshold, verdict)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
