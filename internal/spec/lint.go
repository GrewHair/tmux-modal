package spec

import (
	"fmt"
	"strings"
	"unicode"
)

// minAnchoredChars is the specificity floor below which a rule is flagged
// as under-specified (§5.2): a bare `Filter:` matches any pane that happens
// to display the word.
const minAnchoredChars = 12

func (sp *Spec) lint() {
	check := func(label string, r *Rule) {
		if r == nil {
			return
		}
		anchored, hasRegex := 0, false
		for _, c := range r.Clauses {
			switch c.Kind {
			case KindRegex:
				hasRegex = true
				if src := c.Regex.String(); !c.Negate && (strings.HasSuffix(src, " ") || strings.HasSuffix(src, `\s`)) &&
					!strings.HasSuffix(src, `\ `) {
					sp.warnf("%s: %s ends in whitespace, but lines are matched with trailing whitespace stripped; "+
						"an empty prompt can never match (use '( |$)')", label, c.Label)
				}
				geometric := c.RowA != 0 || c.RowB != -1 || c.ColA >= 0 || c.Anchor != AnchorAnywhere
				if !geometric {
					sp.warnf("%s: %s has no geometry (rows/col/anchor); unanchored substrings false-positive on any pane showing the text",
						label, c.Label)
				} else if !c.Negate {
					anchored += literalChars(c.Regex.String())
				}
			case KindColor:
				if c.WeightKind == WeightRequired {
					sp.warnf("%s: %s is a required colour clause; users retheme their tools, prefer weight = \"bonus\"", label, c.Label)
				}
			}
		}
		if hasRegex && anchored < minAnchoredChars && !r.hasStrongCursor() {
			sp.warnf("%s: under-specified: only %d anchored literal characters (want >= %d)", label, anchored, minAnchoredChars)
		}
	}
	check("match", sp.Identity.Screen)
	for i, r := range sp.ModeRules {
		check(fmt.Sprintf("mode_when[%d] (%s)", i, r.Mode), r)
	}

	leader := sp.EscapeLeader
	for _, k := range sp.Keys {
		if leader != "" && k.Key == leader {
			sp.warnf("keys.%s: the escape leader is also remapped; it will never reach the map", k.Key)
		}
	}
	if sp.HasKeys() && sp.Identity.Screen == nil && !sp.Group {
		sp.warnf("has [keys] but no screen identity clauses: remapping will only ever work locally (by command), never over SSH")
	}
	sp.lintAbsenceAnchor()
}

// lintAbsenceAnchor flags a remapping spec whose default (commanding) mode
// would be concluded from the absence of markers in rows that no required
// identity clause vouches for. Under a remote tmux the last row is the
// inner status line: identity anchors elsewhere still match, the prompt
// one row up is never seen, and keys get remapped while the user types.
// htop needed its bottom bar made required for exactly this reason.
func (sp *Spec) lintAbsenceAnchor() {
	id := sp.Identity.Screen
	if sp.Group || !sp.HasKeys() || sp.Always != "" || id == nil || sp.Bucket(sp.DefaultMode) != BucketCommanding {
		return
	}
	// covers: the identity clause's rows include every row the mode clause
	// reads. Ranges may mix top- and bottom-relative ends ([0, -2] is all
	// but the last row), so compare them resolved at several heights.
	covers := func(id, m *Clause) bool {
		for _, h := range []int{10, 24, 50, 100} {
			res := func(r int) int {
				if r < 0 {
					return h + r
				}
				return min(r, h-1)
			}
			if res(m.RowA) < res(id.RowA) || res(m.RowB) > res(id.RowB) {
				return false
			}
		}
		return true
	}
	for _, r := range sp.ModeRules {
		for _, m := range r.Clauses {
			if m.Kind != KindRegex || m.Negate {
				continue
			}
			vouched := false
			for _, c := range id.Clauses {
				binding := c.WeightKind == WeightRequired || (id.Combine == "all" && c.WeightKind == WeightNumeric)
				if c.Kind == KindRegex && !c.Negate && binding && covers(c, m) {
					vouched = true
				}
			}
			if !vouched {
				sp.warnf("mode_when (%s): %s reads rows [%d, %d], but no required identity clause anchors those rows; "+
					"%q would be concluded from absence even when they show something else (e.g. a remote tmux status line)",
					r.Mode, m.Label, m.RowA, m.RowB, sp.DefaultMode)
			}
		}
	}
}

// hasStrongCursor reports whether a rule also constrains the cursor, which
// makes a short textual marker acceptable.
func (r *Rule) hasStrongCursor() bool {
	for _, c := range r.Clauses {
		if c.Kind == KindCursor && !c.Negate && c.WeightKind != WeightBonus && c.CursorRows != nil {
			return true
		}
	}
	return false
}

// LocaleAudit lists mode rules whose textual markers are English only.
// showmode markers are translated by the application (vim under
// LANG=de_DE shows `-- EINFÜGEN --`), and SSH commonly forwards LANG, so a
// fresh remote host inherits the client's locale. The heuristic is blunt on
// purpose: a marker regex containing letters but no non-ASCII character
// has, in practice, no translated alternates.
func (sp *Spec) LocaleAudit() []string {
	if !sp.Translated {
		return nil
	}
	var out []string
	for i, r := range sp.ModeRules {
		for _, c := range r.Clauses {
			if c.Kind != KindRegex || c.Negate {
				continue
			}
			src := c.Regex.String()
			if strings.IndexFunc(src, unicode.IsLetter) < 0 {
				continue
			}
			if strings.IndexFunc(src, func(r rune) bool { return r > unicode.MaxASCII }) < 0 {
				out = append(out, fmt.Sprintf("mode_when[%d] (%s): %s: marker %q has no non-English alternates",
					i, r.Mode, c.Label, src))
			}
		}
	}
	return out
}
