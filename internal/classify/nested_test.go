package classify_test

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
)

// Every non-nested fixture is a real application screen: none may look
// nested, or its keys would silently stop being remapped.
func TestNestedNoFalsePositives(t *testing.T) {
	set := bundledSet(t)
	n := 0
	filepath.WalkDir(fixtureRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".txt") || strings.Contains(path, "/nested/") {
			return err
		}
		f, err := screen.LoadFixture(path)
		if err != nil {
			t.Fatal(err)
		}
		n++
		if got := classify.DetectNested(set, f.Screen); got.Evidence != "" {
			t.Errorf("%s: detected as nested (%s)", path, got.Evidence)
		}
		return nil
	})
	if n == 0 {
		t.Fatal("no fixtures")
	}
}

func TestNestedSignals(t *testing.T) {
	set := bundledSet(t)
	mk := func(cmd, title string, lines ...string) *screen.Screen {
		s := screen.FromText(strings.Join(lines, "\n"), 20, len(lines))
		s.Command, s.Title, s.AltScreen = cmd, title, true
		return s
	}
	frame := []string{
		"┌────────┐┌───────┐",
		"│ left   ││ right │",
		"│        ││       │",
		"└────────┘└───────┘",
	}
	split := []string{
		"left      │right",
		"          │",
		"──────────┼─────────",
		"          │",
	}
	cases := []struct {
		name  string
		s     *screen.Screen
		want  string
		split bool
		row   int
	}{
		{"local tmux client", mk("tmux", "", "x"), "command", false, -1},
		{"status line bottom", mk("ssh", "", "top", "", "[main] 0:vim* 1:bash-"), "status-line", false, 2},
		{"status line top", mk("ssh", "", "[0] 0:htop*", "", "bottom"), "status-line", false, 0},
		{"vertical and horizontal borders", mk("ssh", "", split...), "borders", true, -1},
		{"acs borders", mk("ssh", "", "ab  x cd", "    x", "    x"), "borders", true, -1},
		{"acs junctions meet a horizontal border", mk("ssh", "", "ab  x cd", "    tqqq", "    x"), "borders", true, -1},
		{"framed tui is not nested", mk("ssh", "", frame...), "", false, -1},
		// Aligned text puts letters in one column on every row: an x and
		// a t under each other are not a border.
		{"aligned text is not acs borders", mk("ssh", "",
			"Sample text line 1", "Sample text line 2", "Sample text line 3", "sample.txt"), "", false, -1},
		{"a junction without a horizontal border is text", mk("ssh", "", "ab  x cd", "    t   ", "    x"), "", false, -1},
		{"set-titles title", mk("ssh", `main:0:vim - "notes"`, "x"), "title", false, -1},
		{"primary screen never nested", func() *screen.Screen {
			s := mk("ssh", "", "[main] 0:vim*")
			s.AltScreen = false
			return s
		}(), "", false, -1},
		{"spec-claimed command never nested", mk("htop", "", split...), "", false, -1},
	}
	for _, c := range cases {
		got := classify.DetectNested(set, c.s)
		if got.Evidence != c.want || got.Split != c.split || got.StatusRow != c.row {
			t.Errorf("%s: got %+v, want evidence %q split %v row %d", c.name, got, c.want, c.split, c.row)
		}
	}
}

// Across an inner split, the inner pane with the visible cursor is the one
// that has the keyboard; it is cut out along the borders, splits within
// splits included, and read as a screen of its own.
func TestNestedFocus(t *testing.T) {
	set := bundledSet(t)
	layout := []string{
		"left      │top",
		"          │",
		"          ├─────────",
		"          │bottom",
		"          │",
		"[main] 0:bash*",
	}
	acs := []string{
		"left      xtop",
		"          x",
		"          tqqqqqqqqq",
		"          xbottom",
		"          x",
	}
	cases := []struct {
		name    string
		lines   []string
		x, y    int
		visible bool
		want    *classify.Rect // nil: no focus
	}{
		{"left", layout, 2, 1, true, &classify.Rect{X: 0, Y: 0, W: 10, H: 5}},
		{"top right", layout, 14, 0, true, &classify.Rect{X: 11, Y: 0, W: 9, H: 2}},
		{"bottom right", layout, 17, 3, true, &classify.Rect{X: 11, Y: 3, W: 9, H: 2}},
		{"bottom right, acs", acs, 17, 3, true, &classify.Rect{X: 11, Y: 3, W: 9, H: 2}},
		{"hidden cursor", layout, 2, 1, false, nil},
		{"cursor on the inner status line", layout, 3, 5, true, nil},
		{"cursor on a border", layout, 10, 1, true, nil},
	}
	for _, c := range cases {
		s := screen.FromText(strings.Join(c.lines, "\n"), 20, len(c.lines))
		s.Command, s.AltScreen = "ssh", true
		s.Cursor = screen.Cursor{X: c.x, Y: c.y, Visible: c.visible}
		n := classify.DetectNested(set, s)
		if !n.Split {
			t.Fatalf("%s: not split", c.name)
		}
		switch {
		case c.want == nil && n.Focus != nil:
			t.Errorf("%s: focus %+v, want none", c.name, *n.Focus)
		case c.want != nil && (n.Focus == nil || *n.Focus != *c.want):
			t.Errorf("%s: focus %v, want %+v", c.name, n.Focus, *c.want)
		}
	}
}

func TestScreenSub(t *testing.T) {
	s := screen.FromText("ab│日本語x\ncd│ef", 10, 2)
	s.Cursor = screen.Cursor{X: 4, Y: 1, Visible: true}
	sub := s.Sub(3, 0, 4, 2)
	if sub.Width != 4 || sub.Height != 2 || sub.Lines[0] != "日本" || sub.Lines[1] != "ef" {
		t.Errorf("sub: %dx%d %q", sub.Width, sub.Height, sub.Lines)
	}
	if !sub.Cursor.Visible || sub.Cursor.X != 1 || sub.Cursor.Y != 1 {
		t.Errorf("cursor %+v", sub.Cursor)
	}
	if cut := s.Sub(4, 0, 3, 1); cut.Lines[0] != " 本" {
		t.Errorf("a wide character cut at the edge: %q", cut.Lines[0])
	}
	if out := s.Sub(0, 0, 2, 2); out.Cursor.Visible {
		t.Error("cursor outside the rectangle must be hidden")
	}
}
