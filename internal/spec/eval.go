package spec

import (
	"fmt"

	"github.com/GrewHair/tmux-modal/internal/screen"
)

// State is the outcome of a clause. Unavailable means the evidence could
// not be read (row outside a partial capture, colour not captured, cursor
// shape unsupported by this tmux); it never counts for or against a rule,
// and negation does not turn it into evidence.
type State int

const (
	Unsatisfied State = iota
	Satisfied
	Unavailable
)

func (s State) String() string {
	return [...]string{"miss", "HIT", "n/a"}[s]
}

// Hit records where a clause matched, for validate output.
type Hit struct {
	Row, Col int
	Text     string
	Color    string
}

// ClauseResult is one clause's evaluation.
type ClauseResult struct {
	Clause *Clause
	State  State
	Hits   []Hit
	Detail string
}

// RuleResult is one rule's evaluation.
type RuleResult struct {
	Rule            *Rule
	Fired           bool
	Score           float64
	RequiredMissing bool
	Clauses         []ClauseResult
}

// Eval evaluates a clause against a screen.
func (c *Clause) Eval(s *screen.Screen) ClauseResult {
	var r ClauseResult
	switch c.Kind {
	case KindRegex:
		r = c.evalRegex(s)
	case KindColor:
		r = c.evalColor(s)
	case KindCursor:
		r = c.evalCursor(s)
	}
	r.Clause = c
	if c.Negate && r.State != Unavailable {
		if r.State == Satisfied {
			r.State = Unsatisfied
		} else {
			r.State = Satisfied
		}
	}
	return r
}

func (c *Clause) evalRegex(s *screen.Screen) ClauseResult {
	var res ClauseResult
	a, b, ok := s.ResolveRange(c.RowA, c.RowB)
	if !ok {
		res.Detail = "row range is empty on this screen"
		return res
	}
	var matchedRows []int
	unavailable := false
	for row := a; row <= b; row++ {
		line, avail := s.Line(row)
		if !avail {
			unavailable = true
			continue
		}
		for _, loc := range c.Regex.FindAllStringIndex(line, -1) {
			col := screen.ColumnOf(line, loc[0])
			if c.ColA >= 0 && (col < c.ColA || col > c.ColB) {
				continue
			}
			if c.Anchor == AnchorEnd && loc[1] != len(line) {
				continue
			}
			h := Hit{Row: row, Col: col, Text: line[loc[0]:loc[1]]}
			if cell, ok := s.Cell(row, col); ok {
				h.Color = fmt.Sprintf("fg=%s bg=%s attrs=%s", cell.Fg, cell.Bg, cell.Attrs)
			}
			res.Hits = append(res.Hits, h)
			matchedRows = append(matchedRows, row)
			break
		}
	}
	if !c.Counting {
		switch {
		case len(matchedRows) > 0:
			res.State = Satisfied
		case unavailable:
			res.State = Unavailable
			res.Detail = "range extends outside the captured rows"
		}
		return res
	}

	n := len(matchedRows)
	if c.Contiguous {
		n = longestRun(matchedRows)
	}
	sat := n >= c.MinOcc && (c.MaxOcc < 0 || n <= c.MaxOcc)
	what := "rows"
	if c.Contiguous {
		what = "contiguous rows"
	}
	res.Detail = fmt.Sprintf("%d %s", n, what)
	switch {
	case unavailable && !(sat && c.MaxOcc < 0):
		res.State = Unavailable
		res.Detail += " (range extends outside the captured rows)"
	case sat:
		res.State = Satisfied
	}
	return res
}

func longestRun(rows []int) int {
	best, run := 0, 0
	for i, r := range rows {
		if i > 0 && r == rows[i-1]+1 {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
	}
	return best
}

func (c *Clause) evalColor(s *screen.Screen) ClauseResult {
	var res ClauseResult
	row := s.Resolve(c.Row)
	col := c.Col
	if col < 0 {
		col = s.Width + col
	}
	cell, ok := s.Cell(row, col)
	if !ok {
		res.State = Unavailable
		res.Detail = "no colour capture for this cell"
		return res
	}
	res.Hits = []Hit{{Row: row, Col: col, Text: string(cell.Ch),
		Color: fmt.Sprintf("fg=%s bg=%s attrs=%s", cell.Fg, cell.Bg, cell.Attrs)}}
	if c.Fg != nil && !colorMatch(*c.Fg, cell.Fg, c.Tolerance) {
		return res
	}
	if c.Bg != nil && !colorMatch(*c.Bg, cell.Bg, c.Tolerance) {
		return res
	}
	if cell.Attrs&c.Attrs != c.Attrs {
		return res
	}
	res.State = Satisfied
	return res
}

// colorMatch compares a spec colour with a captured one. Two palette
// indices compare exactly; anything involving truecolour is compared in
// RGB within the tolerance, since the same visual colour may arrive as
// either depending on what the application believes about its terminal.
func colorMatch(want, got screen.Color, tol float64) bool {
	if want.Kind == screen.ColorIndexed && got.Kind == screen.ColorIndexed {
		return want.Index == got.Index
	}
	if want.Kind == screen.ColorDefault || got.Kind == screen.ColorDefault {
		return want.Kind == got.Kind
	}
	return want.Distance(got) <= tol
}

func (c *Clause) evalCursor(s *screen.Screen) ClauseResult {
	var res ClauseResult
	cur := s.Cursor
	res.Hits = []Hit{{Row: cur.Y, Col: cur.X,
		Text: fmt.Sprintf("visible=%v shape=%q", cur.Visible, cur.Shape)}}
	if c.CursorShape != "" && cur.Shape == "" {
		res.State = Unavailable
		res.Detail = "cursor_shape not reported (tmux < 3.5 or app never set it)"
		return res
	}
	if c.CursorRows != nil {
		a, b, ok := s.ResolveRange(c.CursorRows[0], c.CursorRows[1])
		if !ok || cur.Y < a || cur.Y > b {
			return res
		}
	}
	if c.CursorCols != nil {
		a, b := c.CursorCols[0], c.CursorCols[1]
		if a < 0 {
			a += s.Width
		}
		if b < 0 {
			b += s.Width
		}
		if cur.X < a || cur.X > b {
			return res
		}
	}
	if c.CursorVisible != nil && cur.Visible != *c.CursorVisible {
		return res
	}
	if c.CursorShape != "" && cur.Shape != c.CursorShape {
		return res
	}
	res.State = Satisfied
	return res
}

// Eval evaluates a rule: it fires when every required clause is satisfied
// and the summed weight of satisfied numeric clauses reaches the threshold.
func (r *Rule) Eval(s *screen.Screen) RuleResult {
	res := RuleResult{Rule: r, Clauses: make([]ClauseResult, len(r.Clauses))}
	for i, c := range r.Clauses {
		cr := c.Eval(s)
		res.Clauses[i] = cr
		switch c.WeightKind {
		case WeightNumeric:
			if cr.State == Satisfied {
				res.Score += c.Weight
			}
		case WeightRequired:
			if cr.State != Satisfied {
				res.RequiredMissing = true
			}
		}
	}
	res.Fired = !res.RequiredMissing && res.Score >= r.Threshold && r.loadBearing()
	return res
}

// loadBearing reports whether the rule has any clause that can make it fire.
func (r *Rule) loadBearing() bool {
	for _, c := range r.Clauses {
		if c.WeightKind != WeightBonus {
			return true
		}
	}
	return false
}
