package classify_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
)

// TestDebconfTitles checks the debconf spec against every translation of
// its backtitle that debconf-i18n ships (tests/fixtures/debconf/titles.json,
// extracted by scripts/fixtures/debconf-titles.py). The screens are
// synthetic: the backtitle on row 0 and a note dialog as whiptail draws it,
// in box-drawing characters and in VT100 letters. Real captures in a few
// languages are in tests/fixtures/debconf.
func TestDebconfTitles(t *testing.T) {
	data, err := os.ReadFile(fixtureRoot + "/debconf/titles.json")
	if err != nil {
		t.Fatal(err)
	}
	var titles map[string]map[string]string // lang -> image -> title
	if err := json.Unmarshal(data, &titles); err != nil {
		t.Fatal(err)
	}
	set := bundledSet(t)
	sp := set.Specs["debconf"]
	if sp == nil {
		t.Fatal("no debconf spec")
	}
	dialog := func(title string, acs bool) *screen.Screen {
		lines := []string{
			title,
			"",
			"          ┌──────────────┤ Pending kernel upgrade ├──────────────┐",
			"          │                                                       │",
			"          │ Newer kernel available                                │",
			"          │                                                       │",
			"          │                         <Ok>                          │",
			"          │                                                       │",
			"          └───────────────────────────────────────────────────────┘",
			"",
		}
		if acs {
			r := strings.NewReplacer("┌", "l", "─", "q", "┤", "u", "├", "t", "┐", "k", "│", "x", "└", "m", "┘", "j")
			for i := 2; i < len(lines); i++ {
				lines[i] = r.Replace(lines[i])
			}
		}
		s := screen.FromText(strings.Join(lines, "\n"), 80, len(lines))
		s.AltScreen = true
		return s
	}
	check := func(what string, s *screen.Screen, want string) {
		t.Helper()
		if got := classify.Classify(sp, s, false).Result.Mode; got != want {
			t.Errorf("%s: mode %s, want %s", what, got, want)
		}
	}
	all := map[string]bool{"Package configuration": true}
	for _, per := range titles {
		for _, title := range per {
			all[strings.TrimSpace(title)] = true
		}
	}
	for title := range all {
		check(title, dialog(title, false), "normal")
		check(title+" (VT100 letters)", dialog(title, true), "normal")
		entry := dialog(title, false)
		entry.Cursor = screen.Cursor{X: 12, Y: 4, Visible: true}
		check(title+", text entry", entry, "insert")
	}
	// Another application's first row is not debconf's.
	if res := classify.Classify(sp, dialog("Package configuration tool", false), false).Result; res.Mode == "normal" || res.Mode == "insert" {
		t.Errorf("another first row: mode %s, want none", res.Mode)
	}
}
