// Package classify turns a captured screen into (app, mode, bucket,
// confidence) using the loaded specs.
package classify

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
)

// Confidence levels exported to subscribers.
const (
	High = "high"
	Low  = "low"
)

// Result is the classification of one pane on one capture.
type Result struct {
	App        string // "" when no spec claimed the pane
	Mode       string
	Bucket     string
	Confidence string
	Reason     string

	// Confirmed is true when the app's identity rules fired on this
	// capture (not merely remembered from an earlier one).
	Confirmed bool

	// Nested is the evidence of an inner multiplexer ("" if none); see
	// DetectNested.
	Nested string

	// What the badges show about how this result came about.
	//
	// Evidence is how the app was identified on this capture: "cmd"
	// (the pane's command), "title", "fp" (the screen fingerprint), joined
	// with "+" when several held, or "mem" when none did and the app is
	// remembered from an earlier capture. Score is the fingerprint's score
	// against its threshold ("70/40"), or satisfied/total clauses for a
	// rule without numeric weights ("3/3"); "" when the spec has none.
	Evidence, Score string
	// NestedKind is the inner multiplexer (Nested.Kind); InnerPanes the
	// number of inner panes when its window is split, and FocusBy how the
	// focused one was found ("border colour", "cursor", or "" for none).
	NestedKind string
	InnerPanes int
	FocusBy    string
}

// Identity is the identity evaluation of one spec on one screen.
type Identity struct {
	AltOK        bool
	CommandMatch bool
	TitleMatch   bool
	Screen       *spec.RuleResult
	// Identified: this pane runs the app, by any evidence.
	Identified bool
	// Confirmed: the rendered screen itself shows the app on this
	// capture. Only a confirmed identity may conclude a mode from the
	// absence of a marker.
	Confirmed bool
}

// Shells are local commands that mean "no TUI" when the alternate screen
// is off.
var Shells = map[string]bool{
	"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true,
	"ksh": true, "mksh": true, "tcsh": true, "csh": true, "nu": true,
	"xonsh": true, "elvish": true, "pwsh": true, "login": true, "ash": true,
	"-bash": true, "-zsh": true, "-sh": true,
}

// EvalIdentity evaluates a spec's identity rules.
func EvalIdentity(sp *spec.Spec, s *screen.Screen) Identity {
	id := Identity{AltOK: !sp.Identity.RequiresAlt || s.AltScreen}
	id.CommandMatch = sp.MatchesCommand(filepath.Base(s.Command))
	if sp.Identity.Title != nil && s.Title != "" {
		id.TitleMatch = sp.Identity.Title.MatchString(s.Title)
	}
	if sp.Identity.Screen != nil {
		r := sp.Identity.Screen.Eval(s)
		id.Screen = &r
	}
	screenFired := id.Screen != nil && id.Screen.Fired
	id.Identified = id.AltOK && (id.CommandMatch || id.TitleMatch || screenFired)
	if sp.Identity.Screen != nil {
		id.Confirmed = id.AltOK && screenFired
	} else {
		id.Confirmed = id.Identified
	}
	return id
}

// Trace is the full evaluation of one spec, for validate.
type Trace struct {
	Spec        *spec.Spec
	Identity    Identity
	ModeRules   []spec.RuleResult
	Corroborate []spec.RuleResult
	Result      Result
}

// Classify evaluates one spec against a screen. sticky is true when the
// pane was already identified as this app on an earlier capture; a sticky
// app may still conclude a mode from a positive marker even if its
// identity anchors are momentarily absent, but never from absence.
func Classify(sp *spec.Spec, s *screen.Screen, sticky bool) Trace {
	t := Trace{Spec: sp, Identity: EvalIdentity(sp, s)}
	res := &t.Result
	id := t.Identity
	if !id.Identified && !sticky {
		res.Reason = "identity rules did not fire"
		return t
	}
	res.App = sp.Name
	res.Confirmed = id.Confirmed
	res.Evidence, res.Score = evidence(id)

	if sp.Always != "" {
		if !id.Identified {
			res.Mode, res.Confidence = spec.ModeUnknown, Low
			res.Reason = "always-mode spec, but identity not seen on this capture"
		} else {
			res.Mode, res.Confidence = sp.Always, High
			res.Reason = fmt.Sprintf("always = %q", sp.Always)
		}
		res.Bucket = sp.Bucket(res.Mode)
		return t
	}

	for _, r := range sp.ModeRules {
		rr := r.Eval(s)
		t.ModeRules = append(t.ModeRules, rr)
		if rr.Fired {
			res.Mode, res.Confidence = r.Mode, High
			res.Reason = "mode rule fired: " + ruleLabel(r)
			res.Bucket = sp.Bucket(res.Mode)
			return t
		}
	}

	// No positive marker. Concluding anything from absence requires the
	// identity to be confirmed on this very capture (§4.4).
	if !id.Confirmed {
		res.Mode, res.Confidence = sp.OtherwiseMode, Low
		res.Reason = "no mode marker, and identity not confirmed on this capture"
		res.Bucket = sp.Bucket(res.Mode)
		return t
	}
	mode, conf, reason := sp.DefaultMode, High, "no mode marker; identity confirmed"
	for _, c := range sp.Corroborate {
		rr := c.Rule.Eval(s)
		t.Corroborate = append(t.Corroborate, rr)
		if rr.Fired && contains(c.VetoModes, mode) {
			reason = fmt.Sprintf("corroboration vetoed %q", mode)
			mode, conf = c.ThenMode, c.Confidence
			break
		}
	}
	res.Mode, res.Confidence, res.Reason = mode, conf, reason
	res.Bucket = sp.Bucket(mode)
	return t
}

// evidence describes an identity for the badges (Result.Evidence, Score).
func evidence(id Identity) (string, string) {
	var parts []string
	if id.CommandMatch {
		parts = append(parts, "cmd")
	}
	if id.TitleMatch {
		parts = append(parts, "title")
	}
	if id.Screen != nil && id.Screen.Fired {
		parts = append(parts, "fp")
	}
	ev := strings.Join(parts, "+")
	if !id.Identified {
		ev = "mem"
	}
	score := ""
	if r := id.Screen; r != nil {
		if r.Rule.Threshold > 0 {
			score = fmt.Sprintf("%g/%g", r.Score, r.Rule.Threshold)
		} else {
			ok := 0
			for _, c := range r.Clauses {
				if c.State == spec.Satisfied {
					ok++
				}
			}
			score = fmt.Sprintf("%d/%d", ok, len(r.Clauses))
		}
	}
	return ev, score
}

func ruleLabel(r *spec.Rule) string {
	if r.Name != "" {
		return r.Name
	}
	return r.Mode
}

func contains(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}

// Identify runs full identification across every matchable spec and
// returns the traces of the specs that claimed the pane, best first.
func Identify(set *spec.Set, s *screen.Screen) []Trace {
	var claimed []Trace
	for _, sp := range set.Order {
		t := Classify(sp, s, false)
		if t.Result.App != "" {
			claimed = append(claimed, t)
		}
	}
	sort.SliceStable(claimed, func(i, j int) bool {
		a, b := claimed[i], claimed[j]
		if a.Spec.Priority != b.Spec.Priority {
			return a.Spec.Priority > b.Spec.Priority
		}
		// Equal footing: the one the pane's command names (nvim, not vim,
		// when both read the same screen).
		if a.Identity.CommandMatch != b.Identity.CommandMatch {
			return a.Identity.CommandMatch
		}
		if a.Identity.Confirmed != b.Identity.Confirmed {
			return a.Identity.Confirmed
		}
		return score(a) > score(b)
	})
	return claimed
}

func score(t Trace) float64 {
	if t.Identity.Screen == nil {
		return 0
	}
	return t.Identity.Screen.Score
}

// NoApp is the result for a pane no spec claimed: none when no full-screen
// application is running, unknown when one is but nothing recognised it.
func NoApp(s *screen.Screen) Result {
	if s.AltScreen {
		return Result{Mode: spec.ModeUnknown, Bucket: spec.ModeUnknown, Confidence: High,
			Reason: "alternate screen on, no spec matched"}
	}
	return Result{Mode: spec.ModeNone, Bucket: spec.ModeNone, Confidence: High,
		Reason: "no full-screen application and no spec matched"}
}

// Options adjust classification to the user's settings.
type Options struct {
	// NestedRemap (@modal_nested_remap, default on) lets a key-remapping
	// spec report its mode through a nested multiplexer, so keys are
	// remapped there too. The price: after the inner prefix key, a
	// remapped key reaches the inner tmux mapped (prefix j arrives as
	// prefix Down); the escape leader avoids that. Off: such panes are
	// unknown (pass-through).
	NestedRemap bool
}

// DefaultOptions are the options when the user sets nothing.
var DefaultOptions = Options{NestedRemap: true}

// Pane classifies a pane from scratch or with a sticky app, with the
// default options. It is the entry point shared by the daemon and validate.
func Pane(set *spec.Set, s *screen.Screen, stickyApp string) (Result, []Trace) {
	return PaneWith(set, s, stickyApp, DefaultOptions)
}

// PaneWith is Pane with explicit options.
//
// A pane showing an inner multiplexer is classified on the screen minus
// the inner status line, with low confidence. When the inner window is
// split, only the inner pane with the cursor is classified; with no
// visible cursor the mode is unknown: the outer screen mixes several inner
// panes and does not show which one has the keyboard. A key-remapping spec is
// also unknown there unless opt.NestedRemap.
func PaneWith(set *spec.Set, s *screen.Screen, stickyApp string, opt Options) (Result, []Trace) {
	n := DetectNested(set, s)
	if n.StatusRow >= 0 {
		s = s.WithoutRow(n.StatusRow)
	}
	if n.Focus != nil {
		s = s.Sub(n.Focus.X, n.Focus.Y, n.Focus.W, n.Focus.H)
	}
	res, traces := pane(set, s, stickyApp)
	if n.Evidence == "" {
		return res, traces
	}
	res.Nested, res.NestedKind = n.Evidence, n.Kind
	if n.Split {
		res.InnerPanes, res.FocusBy = n.Panes, n.FocusBy
	}
	sp := set.Specs[res.App]
	switch {
	case res.App == "" || res.Mode == spec.ModeUnknown || res.Mode == spec.ModeNone:
	case (n.Split && n.Focus == nil) || sp == nil || (sp.HasKeys() && !opt.NestedRemap):
		res.Reason = fmt.Sprintf("nested multiplexer (%s): would be %s, but %s", n.Evidence, res.Mode, nestedWhy(n))
		res.Mode, res.Bucket, res.Confidence = spec.ModeUnknown, spec.ModeUnknown, Low
	case n.Focus != nil:
		res.Confidence = Low
		res.Reason = fmt.Sprintf("nested multiplexer (%s), the inner pane %dx%d at %d,%d is focused (by its %s): %s",
			n.Evidence, n.Focus.W, n.Focus.H, n.Focus.X, n.Focus.Y, n.FocusBy, res.Reason)
	default:
		res.Confidence = Low
		res.Reason = fmt.Sprintf("nested multiplexer (%s): %s", n.Evidence, res.Reason)
	}
	return res, traces
}

func nestedWhy(n Nested) string {
	if n.Split && n.Focus == nil {
		return "the inner window is split and neither the border colour nor the cursor shows the focused pane"
	}
	return "@modal_nested_remap is off"
}

func pane(set *spec.Set, s *screen.Screen, stickyApp string) (Result, []Trace) {
	if stickyApp != "" {
		if sp, ok := set.Specs[stickyApp]; ok {
			t := Classify(sp, s, true)
			return t.Result, []Trace{t}
		}
	}
	// Local fast path: a shell in the primary screen runs no TUI. REPL
	// specs identify by their own command, never by a shell's name.
	if !s.AltScreen && Shells[filepath.Base(s.Command)] {
		return NoApp(s), nil
	}
	traces := Identify(set, s)
	if len(traces) == 0 {
		return NoApp(s), nil
	}
	return traces[0].Result, traces
}
