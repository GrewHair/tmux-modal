package classify_test

import (
	"testing"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
)

// How a mode was decided, as the why badge shows it.
func TestModeBasis(t *testing.T) {
	set := bundledSet(t)
	load := func(path string) *screen.Screen {
		f, err := screen.LoadFixture(fixtureRoot + path)
		if err != nil {
			t.Fatal(err)
		}
		return f.Screen
	}
	cases := []struct {
		path, sticky, mode, basis, rule, conf string
	}{
		{"/htop/3.2.2/80x24/normal.txt", "", "normal", "absence", "", "high"},
		{"/htop/3.2.2/80x24/search-typed.txt", "", "insert", "marker", "search", "high"},
		{"/htop/3.2.2/80x24/filter-typed.txt", "", "insert", "marker", "filter", "high"},
		{"/fzf/0.44.1/80x24/typed.txt", "", "insert", "always", "", "high"},
		{"/nested/120x36-utf8/status-bottom-normal.txt", "", "normal", "absence", "", "low"},
	}
	for _, c := range cases {
		res, _ := classify.Pane(set, load(c.path), c.sticky)
		if res.Mode != c.mode || res.ModeBasis != c.basis || res.ModeRule != c.rule || res.Confidence != c.conf {
			t.Errorf("%s: %s by %q %q (%s), want %s by %q %q (%s)",
				c.path, res.Mode, res.ModeBasis, res.ModeRule, res.Confidence, c.mode, c.basis, c.rule, c.conf)
		}
	}
	// Remembered only: the screen does not show htop any more.
	s := screen.FromText("nothing of htop's", 80, 24)
	s.AltScreen, s.Command = true, "ssh"
	if res, _ := classify.Pane(set, s, "htop"); res.ModeBasis != "unconfirmed" || res.Confidence != "low" {
		t.Errorf("remembered htop: %s by %q (%s)", res.Mode, res.ModeBasis, res.Confidence)
	}
}
