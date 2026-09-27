// Package classify turns a captured screen into (app, mode, bucket,
// confidence) using the loaded specs.
package classify

import (
	"fmt"
	"path/filepath"
	"sort"

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
	cmd := filepath.Base(s.Command)
	for _, c := range sp.Identity.Commands {
		if c == cmd {
			id.CommandMatch = true
		}
	}
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

// Pane classifies a pane from scratch or with a sticky app. It is the
// single entry point shared by the daemon and validate.
//
// A pane showing an inner multiplexer is classified on the screen minus
// the inner status line, and then made safe: a key-remapping spec never
// reports a mode there (the inner multiplexer owns a prefix key and copy
// mode, and a split inner window mixes panes), and a hooks-only spec
// reports its mode with low confidence, or unknown across inner splits.
func Pane(set *spec.Set, s *screen.Screen, stickyApp string) (Result, []Trace) {
	n := DetectNested(set, s)
	if n.StatusRow >= 0 {
		s = s.WithoutRow(n.StatusRow)
	}
	res, traces := pane(set, s, stickyApp)
	if n.Evidence == "" {
		return res, traces
	}
	res.Nested = n.Evidence
	sp := set.Specs[res.App]
	switch {
	case res.App == "" || res.Mode == spec.ModeUnknown || res.Mode == spec.ModeNone:
	case n.Split || sp == nil || sp.HasKeys():
		res.Reason = fmt.Sprintf("nested multiplexer (%s): would be %s, but %s", n.Evidence, res.Mode, nestedWhy(n))
		res.Mode, res.Bucket, res.Confidence = spec.ModeUnknown, spec.ModeUnknown, Low
	default:
		res.Confidence = Low
		res.Reason = fmt.Sprintf("nested multiplexer (%s): %s", n.Evidence, res.Reason)
	}
	return res, traces
}

func nestedWhy(n Nested) string {
	if n.Split {
		return "the inner window is split"
	}
	return "keys are never remapped through a nested multiplexer"
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
