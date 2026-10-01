package classify_test

import (
	"strings"
	"testing"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
)

// How a mode was decided, as the mode badge shows it after the mode.
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
		{"/htop/3.2.2/80x24/search-typed.txt", "", "insert", "fp", "search", "high"},
		{"/htop/3.2.2/80x24/filter-typed.txt", "", "insert", "fp", "filter", "high"},
		{"/fzf/0.44.1/80x24/typed.txt", "", "insert", "always", "", "high"},
		// A rule with no name of its own is labelled with its mode.
		{"/vim/9.1/80x24/insert.txt", "vim", "insert", "fp", "insert", "high"},
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
	if res, _ := classify.Pane(set, s, "htop"); res.ModeBasis != "mem" || res.Confidence != "low" {
		t.Errorf("remembered htop: %s by %q (%s)", res.Mode, res.ModeBasis, res.Confidence)
	}

	// The nested policy drops the mode it read; the badge shows it struck.
	nested := load("/nested/120x36-utf8/status-bottom-normal.txt")
	if res, _ := classify.PaneWith(set, nested, "", classify.Options{}); res.ModeBasis != "policy" || res.Dropped != "NORMAL absence" {
		t.Errorf("nested, remap off: %s by %q, dropped %q", res.Mode, res.ModeBasis, res.Dropped)
	}
	if res, _ := classify.Pane(set, nested, ""); res.Dropped != "" {
		t.Errorf("nested, remap on: dropped %q", res.Dropped)
	}
}

// An app that almost matched: htop's screen with the PID header and the
// Tasks line gone scores 30 of 40 and is not recognised.
func TestNearApp(t *testing.T) {
	set := bundledSet(t)
	f, err := screen.LoadFixture(fixtureRoot + "/htop/3.2.2/80x24/normal.txt")
	if err != nil {
		t.Fatal(err)
	}
	s := *f.Screen
	s.Command = "ssh"
	s.Lines = append([]string{}, s.Lines...)
	for i := range s.Lines {
		for _, cut := range []string{"PID", "Tasks"} {
			s.Lines[i] = strings.ReplaceAll(s.Lines[i], cut, strings.Repeat(" ", len(cut)))
		}
	}
	if res, _ := classify.Pane(set, &s, ""); res.App != "" || res.NearApp != "htop" || res.NearScore != "30/40" {
		t.Errorf("app %q, near %q %q; want none, near htop 30/40", res.App, res.NearApp, res.NearScore)
	}
	// Recognised, or a plain unknown screen: no near miss.
	if res, _ := classify.Pane(set, f.Screen, ""); res.NearApp != "" {
		t.Errorf("htop recognised, near %q", res.NearApp)
	}
	blank := screen.FromText("hello", 80, 24)
	blank.AltScreen, blank.Command = true, "ssh"
	if res, _ := classify.Pane(set, blank, ""); res.NearApp != "" {
		t.Errorf("blank screen, near %q %q", res.NearApp, res.NearScore)
	}
}
