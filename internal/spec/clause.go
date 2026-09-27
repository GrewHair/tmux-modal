package spec

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"

	"github.com/GrewHair/tmux-modal/internal/screen"
)

// ClauseKind is the matcher a clause uses.
type ClauseKind int

const (
	KindRegex ClauseKind = iota
	KindColor
	KindCursor
)

func (k ClauseKind) String() string {
	return [...]string{"regex", "color", "cursor"}[k]
}

// WeightKind distinguishes load-bearing numeric weights from the two
// special weights: "bonus" (reported, never needed) and "required" (the
// rule cannot fire without it).
type WeightKind int

const (
	WeightNumeric WeightKind = iota
	WeightBonus
	WeightRequired
)

// Anchor constrains where on a line a regex match may sit.
type Anchor int

const (
	AnchorAnywhere Anchor = iota
	AnchorStart
	AnchorEnd
)

// Clause is one matcher plus geometry plus a weight (§5.2.1).
type Clause struct {
	Kind       ClauseKind
	Negate     bool
	WeightKind WeightKind
	Weight     float64
	Label      string // human-readable origin, for validate

	// regex
	Regex      *regexp.Regexp
	RowA, RowB int // inclusive, negative counts from the bottom
	ColA, ColB int // allowed start columns; ColA < 0 means any
	Anchor     Anchor
	Counting   bool
	MinOcc     int
	MaxOcc     int // -1: unbounded
	Contiguous bool

	// color (single cell)
	Row, Col  int
	Fg, Bg    *screen.Color
	Attrs     screen.Attr
	Tolerance float64

	// cursor
	CursorRows    *[2]int
	CursorCols    *[2]int
	CursorVisible *bool
	CursorShape   string
}

// Tier is the cheapest tier that can evaluate this clause: 1 needs only
// format variables, 2 needs a capture.
func (c *Clause) Tier() int {
	if c.Kind == KindCursor {
		return 1
	}
	return 2
}

// literalChars counts the literal runes a regex requires — a rough measure
// of how specific a pattern is, used by the under-specification lint.
func literalChars(re string) int {
	t, err := syntax.Parse(re, syntax.Perl)
	if err != nil {
		return 0
	}
	var walk func(*syntax.Regexp) int
	walk = func(r *syntax.Regexp) int {
		switch r.Op {
		case syntax.OpLiteral:
			return len(r.Rune)
		case syntax.OpConcat:
			n := 0
			for _, s := range r.Sub {
				n += walk(s)
			}
			return n
		case syntax.OpAlternate:
			// the weakest branch bounds the specificity
			min := -1
			for _, s := range r.Sub {
				if n := walk(s); min < 0 || n < min {
					min = n
				}
			}
			if min < 0 {
				return 0
			}
			return min
		case syntax.OpCapture, syntax.OpPlus:
			return walk(r.Sub[0])
		case syntax.OpRepeat:
			if r.Min > 0 {
				return walk(r.Sub[0]) * r.Min
			}
		}
		return 0
	}
	return walk(t)
}

var clauseKeys = map[string]bool{
	"regex": true, "rows": true, "row": true, "col": true, "cols": true,
	"anchor": true, "min_occurrences": true, "max_occurrences": true,
	"contiguous": true, "negate": true, "weight": true,
	"fg": true, "bg": true, "attrs": true, "tolerance": true,
	"cursor_rows": true, "cursor_cols": true, "cursor_visible": true,
	"cursor_shape": true, "name": true, "note": true, "color": true,
}

func hasAny(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

var regexKeys = []string{"regex"}
var cursorKeys = []string{"cursor_rows", "cursor_cols", "cursor_visible", "cursor_shape"}
var colorKeys = []string{"fg", "bg", "attrs"}

// compileClause builds a clause from a table. kind selects which matcher
// keys are read when the table mixes several (the inline-rule form).
func compileClause(m map[string]any, kind ClauseKind, label string, defWeight WeightKind) (*Clause, error) {
	c := &Clause{Kind: kind, Label: label, ColA: -1, ColB: -1, MaxOcc: -1, RowA: 0, RowB: -1}
	var err error

	c.WeightKind, c.Weight = defWeight, 100
	if defWeight != WeightNumeric {
		c.Weight = 0
	}
	if w, ok := m["weight"]; ok {
		switch v := w.(type) {
		case string:
			switch v {
			case "bonus":
				c.WeightKind, c.Weight = WeightBonus, 0
			case "required":
				c.WeightKind, c.Weight = WeightRequired, 0
			default:
				return nil, fmt.Errorf("%s: weight must be a number, \"bonus\" or \"required\", got %q", label, v)
			}
		default:
			f, ok := toFloat(v)
			if !ok {
				return nil, fmt.Errorf("%s: bad weight %v", label, w)
			}
			c.WeightKind, c.Weight = WeightNumeric, f
		}
	}
	if c.Negate, err = getBool(m, "negate", false); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	switch kind {
	case KindRegex:
		src, _ := m["regex"].(string)
		if src == "" {
			return nil, fmt.Errorf("%s: regex must be a non-empty string", label)
		}
		if c.Regex, err = regexp.Compile(src); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		if err := readRows(m, &c.RowA, &c.RowB); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		if err := readCols(m, &c.ColA, &c.ColB); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		switch a, _ := m["anchor"].(string); a {
		case "", "anywhere":
		case "start":
			c.Anchor = AnchorStart
			if c.ColA < 0 {
				c.ColA, c.ColB = 0, 0
			}
		case "end":
			c.Anchor = AnchorEnd
		default:
			return nil, fmt.Errorf("%s: anchor must be start, end or anywhere", label)
		}
		if v, ok := m["min_occurrences"]; ok {
			n, ok := toInt(v)
			if !ok || n < 0 {
				return nil, fmt.Errorf("%s: bad min_occurrences", label)
			}
			c.MinOcc, c.Counting = n, true
		}
		if v, ok := m["max_occurrences"]; ok {
			n, ok := toInt(v)
			if !ok || n < 0 {
				return nil, fmt.Errorf("%s: bad max_occurrences", label)
			}
			c.MaxOcc, c.Counting = n, true
		}
		if c.Contiguous, err = getBool(m, "contiguous", false); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		if c.Contiguous {
			c.Counting = true
			if c.MinOcc == 0 {
				c.MinOcc = 1
			}
		}
		if c.Counting && c.MinOcc == 0 && c.MaxOcc < 0 {
			c.MinOcc = 1
		}

	case KindColor:
		if err := readSingle(m, "row", &c.Row); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		if err := readSingle(m, "col", &c.Col); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		for _, k := range []string{"fg", "bg"} {
			v, ok := m[k]
			if !ok {
				continue
			}
			col, err := parseColorValue(v)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", label, k, err)
			}
			if k == "fg" {
				c.Fg = &col
			} else {
				c.Bg = &col
			}
		}
		if v, ok := m["attrs"]; ok {
			for _, name := range toStrings(v) {
				a, ok := screen.ParseAttr(name)
				if !ok {
					return nil, fmt.Errorf("%s: unknown attribute %q", label, name)
				}
				c.Attrs |= a
			}
		}
		c.Tolerance = 24
		if v, ok := m["tolerance"]; ok {
			f, ok := toFloat(v)
			if !ok {
				return nil, fmt.Errorf("%s: bad tolerance", label)
			}
			c.Tolerance = f
		}
		if c.Fg == nil && c.Bg == nil && c.Attrs == 0 {
			return nil, fmt.Errorf("%s: colour clause asserts nothing (need fg, bg or attrs)", label)
		}

	case KindCursor:
		if v, ok := m["cursor_rows"]; ok {
			r, err := toRange(v)
			if err != nil {
				return nil, fmt.Errorf("%s: cursor_rows: %w", label, err)
			}
			c.CursorRows = &r
		}
		if v, ok := m["cursor_cols"]; ok {
			r, err := toRange(v)
			if err != nil {
				return nil, fmt.Errorf("%s: cursor_cols: %w", label, err)
			}
			c.CursorCols = &r
		}
		if _, ok := m["cursor_visible"]; ok {
			b, err := getBool(m, "cursor_visible", false)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", label, err)
			}
			c.CursorVisible = &b
		}
		if v, ok := m["cursor_shape"]; ok {
			s, _ := v.(string)
			switch s {
			case "block", "underline", "bar", "default":
			default:
				return nil, fmt.Errorf("%s: cursor_shape must be block, underline, bar or default", label)
			}
			c.CursorShape = s
		}
	}
	return c, nil
}

func parseColorValue(v any) (screen.Color, error) {
	if n, ok := toInt(v); ok {
		if n < 0 || n > 255 {
			return screen.Color{}, fmt.Errorf("palette index %d out of range", n)
		}
		return screen.Color{Kind: screen.ColorIndexed, Index: uint8(n)}, nil
	}
	s, ok := v.(string)
	if !ok {
		return screen.Color{}, fmt.Errorf("bad colour %v", v)
	}
	return screen.ParseColor(s)
}

func readRows(m map[string]any, a, b *int) error {
	if v, ok := m["rows"]; ok {
		r, err := toRange(v)
		if err != nil {
			return fmt.Errorf("rows: %w", err)
		}
		*a, *b = r[0], r[1]
		return nil
	}
	if v, ok := m["row"]; ok {
		n, ok := toInt(v)
		if !ok {
			return fmt.Errorf("row must be an integer")
		}
		*a, *b = n, n
	}
	return nil
}

func readCols(m map[string]any, a, b *int) error {
	if v, ok := m["cols"]; ok {
		r, err := toRange(v)
		if err != nil {
			return fmt.Errorf("cols: %w", err)
		}
		if r[0] < 0 || r[1] < r[0] {
			return fmt.Errorf("cols must be a non-negative ascending range")
		}
		*a, *b = r[0], r[1]
		return nil
	}
	if v, ok := m["col"]; ok {
		n, ok := toInt(v)
		if !ok || n < 0 {
			return fmt.Errorf("col must be a non-negative integer")
		}
		*a, *b = n, n
	}
	return nil
}

func readSingle(m map[string]any, key string, dst *int) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("colour clause needs %s", key)
	}
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf("%s must be an integer", key)
	}
	*dst = n
	return nil
}

// Describe renders a clause for validate output.
func (c *Clause) Describe() string {
	var b strings.Builder
	if c.Negate {
		b.WriteString("NOT ")
	}
	switch c.Kind {
	case KindRegex:
		fmt.Fprintf(&b, "regex %q rows=[%d,%d]", c.Regex.String(), c.RowA, c.RowB)
		if c.ColA >= 0 {
			if c.ColA == c.ColB {
				fmt.Fprintf(&b, " col=%d", c.ColA)
			} else {
				fmt.Fprintf(&b, " cols=[%d,%d]", c.ColA, c.ColB)
			}
		}
		if c.Anchor == AnchorEnd {
			b.WriteString(" anchor=end")
		}
		if c.Counting {
			fmt.Fprintf(&b, " occurrences>=%d", c.MinOcc)
			if c.MaxOcc >= 0 {
				fmt.Fprintf(&b, ",<=%d", c.MaxOcc)
			}
			if c.Contiguous {
				b.WriteString(" contiguous")
			}
		}
	case KindColor:
		fmt.Fprintf(&b, "colour at (%d,%d)", c.Row, c.Col)
		if c.Fg != nil {
			fmt.Fprintf(&b, " fg=%s", c.Fg)
		}
		if c.Bg != nil {
			fmt.Fprintf(&b, " bg=%s", c.Bg)
		}
		if c.Attrs != 0 {
			fmt.Fprintf(&b, " attrs=%s", c.Attrs)
		}
	case KindCursor:
		b.WriteString("cursor")
		if c.CursorRows != nil {
			fmt.Fprintf(&b, " rows=[%d,%d]", c.CursorRows[0], c.CursorRows[1])
		}
		if c.CursorCols != nil {
			fmt.Fprintf(&b, " cols=[%d,%d]", c.CursorCols[0], c.CursorCols[1])
		}
		if c.CursorVisible != nil {
			fmt.Fprintf(&b, " visible=%v", *c.CursorVisible)
		}
		if c.CursorShape != "" {
			fmt.Fprintf(&b, " shape=%s", c.CursorShape)
		}
	}
	return b.String()
}

func (c *Clause) WeightString() string {
	switch c.WeightKind {
	case WeightBonus:
		return "bonus"
	case WeightRequired:
		return "required"
	}
	return fmt.Sprintf("%g", c.Weight)
}
