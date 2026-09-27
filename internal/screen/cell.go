package screen

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Attr is a bit set of SGR attributes.
type Attr uint8

const (
	AttrBold Attr = 1 << iota
	AttrDim
	AttrItalic
	AttrUnderline
	AttrBlink
	AttrReverse
	AttrHidden
	AttrStrike
)

var attrNames = []struct {
	name string
	a    Attr
}{
	{"bold", AttrBold}, {"dim", AttrDim}, {"italic", AttrItalic},
	{"underline", AttrUnderline}, {"blink", AttrBlink},
	{"reverse", AttrReverse}, {"hidden", AttrHidden}, {"strike", AttrStrike},
}

// ParseAttr maps an attribute name to its bit.
func ParseAttr(name string) (Attr, bool) {
	for _, n := range attrNames {
		if n.name == name {
			return n.a, true
		}
	}
	return 0, false
}

func (a Attr) String() string {
	var parts []string
	for _, n := range attrNames {
		if a&n.a != 0 {
			parts = append(parts, n.name)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

// Color is a terminal colour as it arrived: default, a palette index, or
// truecolour. RGB() normalises all of them into one space for comparison.
type Color struct {
	Kind  ColorKind
	Index uint8 // palette index when Kind == ColorIndexed
	R, G, B uint8
}

type ColorKind uint8

const (
	ColorDefault ColorKind = iota
	ColorIndexed
	ColorRGB
)

func (c Color) String() string {
	switch c.Kind {
	case ColorIndexed:
		if c.Index < 16 {
			return fmt.Sprintf("%s(%d)", ansiNames[c.Index], c.Index)
		}
		return fmt.Sprintf("colour%d", c.Index)
	case ColorRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return "default"
}

// RGB returns the colour in RGB, using the xterm default palette for indexed
// colours. ok is false for the terminal default colour, whose actual value
// is unknowable from inside tmux.
func (c Color) RGB() (r, g, b uint8, ok bool) {
	switch c.Kind {
	case ColorRGB:
		return c.R, c.G, c.B, true
	case ColorIndexed:
		r, g, b = paletteRGB(c.Index)
		return r, g, b, true
	}
	return 0, 0, 0, false
}

// Distance is the Euclidean RGB distance between two colours, or +Inf when
// either is the terminal default.
func (c Color) Distance(o Color) float64 {
	r1, g1, b1, ok1 := c.RGB()
	r2, g2, b2, ok2 := o.RGB()
	if !ok1 || !ok2 {
		if c.Kind == ColorDefault && o.Kind == ColorDefault {
			return 0
		}
		return math.Inf(1)
	}
	dr, dg, db := float64(r1)-float64(r2), float64(g1)-float64(g2), float64(b1)-float64(b2)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

var ansiNames = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"brightblack", "brightred", "brightgreen", "brightyellow",
	"brightblue", "brightmagenta", "brightcyan", "brightwhite",
}

// xterm's default 16-colour palette.
var ansiRGB = [16][3]uint8{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0},
	{0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
	{92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

func paletteRGB(i uint8) (uint8, uint8, uint8) {
	switch {
	case i < 16:
		c := ansiRGB[i]
		return c[0], c[1], c[2]
	case i < 232:
		i -= 16
		lv := func(v uint8) uint8 {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return lv(i / 36), lv((i / 6) % 6), lv(i % 6)
	default:
		v := 8 + (i-232)*10
		return v, v, v
	}
}

// ParseColor parses a spec colour: a name ("cyan", "brightred",
// "default"), a palette index ("6", "colour6"), or "#RRGGBB".
func ParseColor(s string) (Color, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "bright_", "bright")
	if s == "default" {
		return Color{}, nil
	}
	for i, n := range ansiNames {
		if s == n {
			return Color{Kind: ColorIndexed, Index: uint8(i)}, nil
		}
	}
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		v, err := strconv.ParseUint(s[1:], 16, 32)
		if err != nil {
			return Color{}, fmt.Errorf("bad colour %q", s)
		}
		return Color{Kind: ColorRGB, R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}, nil
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "colour"), "color")
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 255 {
		return Color{Kind: ColorIndexed, Index: uint8(n)}, nil
	}
	return Color{}, fmt.Errorf("bad colour %q (want a name, a palette index or #RRGGBB)", s)
}

// Cell is one rendered cell with its attributes.
type Cell struct {
	Ch    rune
	Fg    Color
	Bg    Color
	Attrs Attr
}

// ParseANSI parses `capture-pane -p -e` output into a cell grid.
//
// SGR state is carried across line boundaries: tmux's capture emits only
// the attribute changes between consecutive cells, including across rows,
// so a row does not necessarily restate its colours.
func ParseANSI(text string, height int) [][]Cell {
	var (
		rows [][]Cell
		cur  []Cell
		pen  Cell
	)
	flush := func() {
		rows = append(rows, cur)
		cur = nil
	}
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\n':
			flush()
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == '[':
			j := i + 2
			for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
				j++
			}
			if j < len(rs) && rs[j] == 'm' {
				applySGR(&pen, string(rs[i+2:j]))
			}
			i = j
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == ']':
			// OSC (e.g. hyperlinks): skip to BEL or ST.
			j := i + 2
			for j < len(rs) && rs[j] != 0x07 && !(rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\') {
				j++
			}
			if j < len(rs) && rs[j] == 0x1b {
				j++
			}
			i = j
		case r < 0x20:
			// other control characters carry no cell
		default:
			c := pen
			c.Ch = r
			cur = append(cur, c)
			if RuneWidth(r) == 2 {
				pad := pen
				pad.Ch = 0
				cur = append(cur, pad)
			}
		}
	}
	if len(cur) > 0 {
		flush()
	}
	for height > 0 && len(rows) < height {
		rows = append(rows, nil)
	}
	if height > 0 && len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

func applySGR(pen *Cell, params string) {
	if params == "" {
		params = "0"
	}
	// tmux emits colon sub-parameters for some forms (38:2::r:g:b); treat
	// colons as separators so both spellings decode the same.
	fields := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	nums := make([]int, 0, len(fields))
	for _, f := range fields {
		n, _ := strconv.Atoi(f)
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		nums = []int{0}
	}
	for i := 0; i < len(nums); i++ {
		n := nums[i]
		switch {
		case n == 0:
			*pen = Cell{}
		case n == 1:
			pen.Attrs |= AttrBold
		case n == 2:
			pen.Attrs |= AttrDim
		case n == 3:
			pen.Attrs |= AttrItalic
		case n == 4:
			pen.Attrs |= AttrUnderline
		case n == 5 || n == 6:
			pen.Attrs |= AttrBlink
		case n == 7:
			pen.Attrs |= AttrReverse
		case n == 8:
			pen.Attrs |= AttrHidden
		case n == 9:
			pen.Attrs |= AttrStrike
		case n == 22:
			pen.Attrs &^= AttrBold | AttrDim
		case n == 23:
			pen.Attrs &^= AttrItalic
		case n == 24:
			pen.Attrs &^= AttrUnderline
		case n == 25:
			pen.Attrs &^= AttrBlink
		case n == 27:
			pen.Attrs &^= AttrReverse
		case n == 28:
			pen.Attrs &^= AttrHidden
		case n == 29:
			pen.Attrs &^= AttrStrike
		case n >= 30 && n <= 37:
			pen.Fg = Color{Kind: ColorIndexed, Index: uint8(n - 30)}
		case n == 39:
			pen.Fg = Color{}
		case n >= 40 && n <= 47:
			pen.Bg = Color{Kind: ColorIndexed, Index: uint8(n - 40)}
		case n == 49:
			pen.Bg = Color{}
		case n >= 90 && n <= 97:
			pen.Fg = Color{Kind: ColorIndexed, Index: uint8(n - 90 + 8)}
		case n >= 100 && n <= 107:
			pen.Bg = Color{Kind: ColorIndexed, Index: uint8(n - 100 + 8)}
		case n == 38 || n == 48:
			c, used := extendedColor(nums[i+1:])
			if n == 38 {
				pen.Fg = c
			} else {
				pen.Bg = c
			}
			i += used
		}
	}
}

func extendedColor(rest []int) (Color, int) {
	if len(rest) >= 2 && rest[0] == 5 {
		return Color{Kind: ColorIndexed, Index: uint8(rest[1])}, 2
	}
	if len(rest) >= 1 && rest[0] == 2 {
		// 38;2;r;g;b, or 38:2::r:g:b (empty colourspace id collapsed by
		// FieldsFunc, so both arrive as 2,r,g,b).
		if len(rest) >= 4 {
			return Color{Kind: ColorRGB, R: uint8(rest[1]), G: uint8(rest[2]), B: uint8(rest[3])}, 4
		}
	}
	return Color{}, len(rest)
}
