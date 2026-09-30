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

// The border colour shows the focused inner pane even with the cursor
// hidden (htop), but only in tmux's default colours; anything else, or a
// cursor in another pane, and nothing is guessed.
func TestNestedFocusColour(t *testing.T) {
	set := bundledSet(t)
	load := func(name string) *screen.Screen {
		f, err := screen.LoadFixture(fixtureRoot + "/nested/tmux-3.7c/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return f.Screen
	}
	recolour := func(s *screen.Screen, from, to screen.Color) {
		for _, row := range s.Cells {
			for i := range row {
				if row[i].Fg == from && strings.ContainsRune("│─├┤┬┴┼", row[i].Ch) {
					row[i].Fg = to
				}
			}
		}
	}
	green := screen.Color{Kind: screen.ColorIndexed, Index: 2}
	yellow := screen.Color{Kind: screen.ColorIndexed, Index: 3}

	s := load("three-htop.txt")
	n := classify.DetectNested(set, s)
	if n.Focus == nil || n.FocusBy != "border colour" || n.Focus.Y != 0 {
		t.Fatalf("three panes, htop on top: focus %v by %q", n.Focus, n.FocusBy)
	}
	top := *n.Focus

	themed := load("three-htop.txt")
	recolour(themed, green, yellow)
	if n := classify.DetectNested(set, themed); n.Focus != nil {
		t.Errorf("a themed border colour: focus %v by %q, want none", *n.Focus, n.FocusBy)
	}

	elsewhere := load("three-htop.txt")
	elsewhere.Cursor = screen.Cursor{X: 2, Y: top.H + 2, Visible: true}
	if n := classify.DetectNested(set, elsewhere); n.Focus != nil {
		t.Errorf("the cursor in another pane than the green border: focus %v by %q, want none", *n.Focus, n.FocusBy)
	}

	agree := load("three-htop.txt")
	agree.Cursor = screen.Cursor{X: 2, Y: 2, Visible: true}
	if n := classify.DetectNested(set, agree); n.Focus == nil || *n.Focus != top {
		t.Errorf("cursor and border agree: focus %v, want %+v", n.Focus, top)
	}

	// vim hides the cursor for ~100 ms while it redraws: its pane is still
	// known by the border, so it does not flicker to unknown.
	for _, dir := range []string{"120x36-utf8", "80x24-acs"} {
		f, err := screen.LoadFixture(fixtureRoot + "/nested/" + dir + "/split-vim-normal.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Screen.Cursor.Visible = false
		if res, _ := classify.Pane(set, f.Screen, "vim"); res.Mode != "normal" {
			t.Errorf("%s: split vim with the cursor hidden: %s (%s)", dir, res.Mode, res.Reason)
		}
	}

	for _, name := range []string{"two-side-htop.txt", "two-stacked-htop.txt"} {
		s := load(name)
		n := classify.DetectNested(set, s)
		if n.Focus == nil || n.Focus.X != 0 || n.Focus.Y != 0 {
			t.Errorf("%s: focus %v, want the first pane", name, n.Focus)
		}
		all := load(name)
		recolour(all, screen.Color{}, green)
		if n := classify.DetectNested(set, all); n.Focus != nil {
			t.Errorf("%s, the whole border green: focus %v, want none", name, *n.Focus)
		}
	}
}

// What the badges show: the inner multiplexer's kind, the split and how
// the focus was found, and how the app was identified.
func TestNestedBadgeFacts(t *testing.T) {
	set := bundledSet(t)
	for cmd, want := range map[string]string{"tmux": "tmux", "screen": "screen", "zellij": "zellij", "tmate": "tmate"} {
		s := screen.FromText("hello", 20, 3)
		s.Command = cmd
		if n := classify.DetectNested(set, s); n.Kind != want {
			t.Errorf("command %s: kind %q, want %q", cmd, n.Kind, want)
		}
	}
	f, err := screen.LoadFixture(fixtureRoot + "/nested/tmux-3.7c/three-htop.txt")
	if err != nil {
		t.Fatal(err)
	}
	res, _ := classify.Pane(set, f.Screen, "")
	if res.NestedKind != "tmux" || res.InnerPanes != 3 || res.FocusBy != "border colour" ||
		res.Evidence != "fp" || res.Score == "" {
		t.Errorf("three-htop: kind %q panes %d focus %q evidence %q score %q",
			res.NestedKind, res.InnerPanes, res.FocusBy, res.Evidence, res.Score)
	}
	local := *f.Screen
	local.Command = "tmux"
	if res, _ := classify.Pane(set, &local, "htop"); res.Evidence != "fp" || res.NestedKind != "tmux" {
		t.Errorf("three-htop, local tmux, sticky htop: evidence %q kind %q", res.Evidence, res.NestedKind)
	}
	hidden := *f.Screen
	hidden.Lines = append([]string{}, hidden.Lines...)
	for i := range hidden.Lines {
		hidden.Lines[i] = strings.ReplaceAll(hidden.Lines[i], "F1Help", "      ") // the required bottom bar
	}
	if res, _ := classify.Pane(set, &hidden, "htop"); res.Evidence != "mem" {
		t.Errorf("htop's bottom bar gone, sticky: evidence %q (%s), want mem", res.Evidence, res.Reason)
	}
}
