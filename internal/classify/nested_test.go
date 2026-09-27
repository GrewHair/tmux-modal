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
