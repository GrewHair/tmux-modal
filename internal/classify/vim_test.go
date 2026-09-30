package classify_test

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
)

// TestVimMarkers checks the vim-family rules against every translation vim
// and neovim ship (tests/fixtures/vim/markers.json, extracted from their
// message catalogues by scripts/fixtures/vim-markers.py). The screens are
// synthetic: the showmode string on the last row as vim composes it, and
// the ruler 18 columns from the right edge with its translated position
// word. Real captures in a few languages are in tests/fixtures/vim.
func TestVimMarkers(t *testing.T) {
	data, err := os.ReadFile(fixtureRoot + "/vim/markers.json")
	if err != nil {
		t.Fatal(err)
	}
	var markers map[string]map[string]map[string]string // version -> lang -> msgid -> text
	if err := json.Unmarshal(data, &markers); err != nil {
		t.Fatal(err)
	}
	set := bundledSet(t)
	vim := set.Specs["vim"]
	if vim == nil {
		t.Fatal("no vim spec")
	}

	const w, h = 80, 10
	// screenWith: a short buffer, the tilde column, and a last row holding
	// left (the mode line) and, unless ruler is empty, the ruler.
	screenWith := func(left, ruler string, tildes bool) *screen.Screen {
		rows := []string{"first line", "second line", "third line"}
		for len(rows) < h-1 {
			if tildes {
				rows = append(rows, "~")
			} else {
				rows = append(rows, "more text")
			}
		}
		last := left
		if ruler != "" {
			last += strings.Repeat(" ", max(1, w-18-screen.StringWidth(left))) + ruler
		}
		s := screen.FromText(strings.Join(append(rows, last), "\n"), w, h)
		s.AltScreen = true
		return s
	}
	versions := make([]string, 0, len(markers))
	for v := range markers {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	for _, ver := range versions {
		langs := markers[ver]
		langs["C"] = map[string]string{} // the untranslated strings
		for lang, tr := range langs {
			text := func(msgid string) string {
				if v, ok := tr[msgid]; ok {
					return strings.TrimSpace(v)
				}
				return strings.TrimSpace(msgid)
			}
			ruler := "1,1" + strings.Repeat(" ", 10) + text("All")
			check := func(what string, s *screen.Screen, sticky bool, want string) {
				t.Helper()
				got := classify.Classify(vim, s, sticky).Result.Mode
				if got != want {
					last, _ := s.Line(-1)
					t.Errorf("%s %s: %s: mode %s, want %s\n  last row %q", ver, lang, what, got, want, last)
				}
			}
			// Normal mode from the ruler alone (a buffer that fills the
			// window) and from tildes plus the file message.
			check("ruler only", screenWith("", ruler, false), false, "normal")
			check("tildes + file message", screenWith(`"notes.txt" 12L, 280B`, "", true), false, "normal")
			check("tildes only (sticky)", screenWith("", "", true), true, "unknown")

			marker := func(msgid string) string { return "-- " + text(msgid) + " --" }
			for msgid, want := range map[string]string{
				" INSERT": "insert", " REPLACE": "replace", " VREPLACE": "replace",
				" VISUAL": "visual", " VISUAL LINE": "visual", " VISUAL BLOCK": "visual",
				" SELECT": "select", " SELECT LINE": "select", " SELECT BLOCK": "select",
				" (insert)": "normal", " (replace)": "normal", " (vreplace)": "normal",
			} {
				if want == "select" && text(msgid) == text(" VISUAL") {
					want = "visual" // zh_TW: one word for both; see vim-family.toml
				}
				check(msgid, screenWith(marker(msgid), ruler, true), false, want)
			}
			if _, ok := tr[" TERMINAL"]; ok || lang == "C" {
				check("terminal", screenWith(marker(" TERMINAL"), "", true), true, "terminal")
			}
			check("(insert) VISUAL", screenWith("-- "+text(" (insert)")+" "+text(" VISUAL")+" --", ruler, true), false, "visual")
			check("completion", screenWith("-- "+text(" Keyword completion (^N^P)")+" match 1 of 3", "", true), true, "insert")
			check("more prompt", screenWith(text("-- More --"), "", false), false, "normal")
			check("hit-enter prompt", screenWith(text("Press ENTER or type command to continue"), "", false), false, "normal")
		}
	}
}

// A capture taken mid-redraw, after vim drew the buffer but before the mode
// line, must read as unknown, never normal (brief §9): the ruler is on the
// mode line, and tildes alone do not confirm vim. A capture of only the
// bottom rows (the daemon's capture band) still has everything it needs.
func TestVimPartialCaptures(t *testing.T) {
	set := bundledSet(t)
	vim := set.Specs["vim"]
	for _, size := range []string{"80x24", "120x40"} {
		dir := fixtureRoot + "/vim/9.1/" + size + "/"
		for _, c := range []struct{ file, want string }{
			{"normal.txt", "normal"},
			{"insert.txt", "insert"},
			{"visual-line.txt", "visual"},
		} {
			f, err := screen.LoadFixture(dir + c.file)
			if err != nil {
				t.Fatal(err)
			}
			blank := *f.Screen
			blank.Lines = append([]string{}, f.Screen.Lines...)
			blank.Lines[len(blank.Lines)-1] = ""
			blank.Cells = nil
			if got := classify.Classify(vim, &blank, true).Result.Mode; got != "unknown" {
				t.Errorf("%s %s with the mode line not yet drawn: %s, want unknown", size, c.file, got)
			}
			band := *f.Screen
			band.Top = band.Height - 3
			band.Lines = f.Screen.Lines[band.Top:]
			band.Cells = nil
			if got := classify.Classify(vim, &band, true).Result.Mode; got != c.want {
				t.Errorf("%s %s, bottom 3 rows only: %s, want %s", size, c.file, got, c.want)
			}
		}
	}
}

// Where tmux reports the cursor shape, neovim's bar cursor in insert vetoes
// the "normal" that showmode off would otherwise give; a block cursor, or
// a tmux that reports nothing, leaves the screen's answer alone.
func TestVimCursorShapeVeto(t *testing.T) {
	set := bundledSet(t)
	f, err := screen.LoadFixture(fixtureRoot + "/nvim/0.12.5/80x24/noshowmode-insert.txt")
	if err != nil {
		t.Fatal(err)
	}
	for shape, want := range map[string]string{"bar": "insert", "block": "normal", "": "normal"} {
		s := *f.Screen
		s.Cursor.Shape = shape
		res := classify.Classify(set.Specs["nvim"], &s, true).Result
		if res.Mode != want {
			t.Errorf("cursor shape %q: %s, want %s", shape, res.Mode, want)
		}
		if shape == "bar" && (res.Confidence != classify.Low || res.ModeBasis != "veto") {
			t.Errorf("a vetoed normal is low confidence, by veto: got %s by %q", res.Confidence, res.ModeBasis)
		}
	}
}
