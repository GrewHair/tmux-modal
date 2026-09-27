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
