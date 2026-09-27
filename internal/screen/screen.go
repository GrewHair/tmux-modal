// Package screen models a captured tmux pane: the rendered text grid, an
// optional colour/attribute grid, the cursor and the O(1) terminal-state
// flags that tier 1 reads.
package screen

import (
	"strings"
	"unicode"
)

// Cursor is the cursor state as tmux reports it.
type Cursor struct {
	X, Y    int
	Visible bool
	// Shape is the DECSCUSR style: "block", "underline", "bar", "default",
	// or "" when the running tmux does not expose #{cursor_shape} (< 3.5).
	Shape string
}

// Screen is one capture of a pane.
//
// Rows are always addressed in full-screen coordinates (0..Height-1). A
// partial capture (only the bottom N rows) sets Top to the first captured
// row; rows above Top are unavailable, which is distinct from empty.
type Screen struct {
	Width, Height int
	Top           int
	Lines         []string // rendered text, trailing whitespace stripped
	Cells         [][]Cell // nil unless captured with colour (-e)
	Cursor        Cursor
	AltScreen     bool
	Command       string // pane_current_command
	Title         string // pane_title
}

// Line returns the rendered text of full-screen row r and whether that row
// was captured.
func (s *Screen) Line(r int) (string, bool) {
	if r < s.Top || r >= s.Top+len(s.Lines) {
		return "", false
	}
	return s.Lines[r-s.Top], true
}

// Cell returns the cell at full-screen row r, column c.
func (s *Screen) Cell(r, c int) (Cell, bool) {
	if s.Cells == nil || r < s.Top || r-s.Top >= len(s.Cells) {
		return Cell{}, false
	}
	row := s.Cells[r-s.Top]
	if c < 0 {
		return Cell{}, false
	}
	if c >= len(row) {
		// Beyond the last emitted cell: blank with default colours.
		return Cell{Ch: ' '}, true
	}
	return row[c], true
}

// Resolve converts a possibly-negative row index to a full-screen row.
func (s *Screen) Resolve(r int) int {
	if r < 0 {
		return s.Height + r
	}
	return r
}

// ResolveRange converts an inclusive [a,b] range with negative indices to
// full-screen rows, clipped to the screen. ok is false when empty.
func (s *Screen) ResolveRange(a, b int) (int, int, bool) {
	a, b = s.Resolve(a), s.Resolve(b)
	if a < 0 {
		a = 0
	}
	if b > s.Height-1 {
		b = s.Height - 1
	}
	return a, b, a <= b
}

// FromText builds a Screen from plain captured text.
func FromText(text string, width, height int) *Screen {
	s := &Screen{Width: width, Height: height}
	s.Lines = splitLines(text, height)
	if s.Width == 0 {
		for _, l := range s.Lines {
			if w := StringWidth(l); w > s.Width {
				s.Width = w
			}
		}
	}
	if s.Height == 0 {
		s.Height = len(s.Lines)
	}
	return s
}

func splitLines(text string, height int) []string {
	text = strings.TrimSuffix(text, "\n")
	var lines []string
	if text != "" || height > 0 {
		lines = strings.Split(text, "\n")
	}
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
	}
	for height > 0 && len(lines) < height {
		lines = append(lines, "")
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// StringWidth returns the number of terminal columns s occupies.
func StringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// ColumnOf converts a byte offset in s to a terminal column.
func ColumnOf(s string, byteOff int) int {
	return StringWidth(s[:byteOff])
}

// RuneWidth is a deliberately small East-Asian-wide approximation: enough
// to keep column arithmetic right for CJK text in process lists and file
// names without pulling in a width table dependency.
func RuneWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x1100:
		return 1
	case r <= 0x115f, // Hangul Jamo
		r >= 0x2e80 && r <= 0xa4cf && r != 0x303f,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	}
	return 1
}
