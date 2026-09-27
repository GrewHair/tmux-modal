package classify_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tmuxmodal "github.com/GrewHair/tmux-modal"
	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
)

const fixtureRoot = "../../tests/fixtures"

func bundledSet(t *testing.T) *spec.Set {
	t.Helper()
	set := spec.Load([]spec.Source{{FS: tmuxmodal.Specs, Dir: "specs", Label: "bundled"}})
	for _, w := range set.Warnings {
		t.Errorf("bundled spec failed to load: %s", w)
	}
	return set
}

// TestFixtures runs every golden fixture through the classifier in four
// variants, all of which must agree with the expectation in its header:
//
//	remote  as captured (fixtures are captured inside containers, so the
//	        pane command is not the app: identity must come from the screen)
//	local   with pane_current_command set to the app (the local fast path),
//	        or for nested/ fixtures to tmux (a local nested client)
//	sticky  with the app already identified on an earlier capture
//	mono    with the colour capture stripped
func TestFixtures(t *testing.T) {
	set := bundledSet(t)
	n := 0
	err := filepath.WalkDir(fixtureRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".txt") {
			return err
		}
		f, err := screen.LoadFixture(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return nil
		}
		app, mode := f.Meta["expect_app"], f.Meta["expect_mode"]
		if mode == "" {
			return nil
		}
		n++
		rel, _ := filepath.Rel(fixtureRoot, path)
		check := func(variant string, s *screen.Screen, sticky string) {
			res, _ := classify.Pane(set, s, sticky)
			wantApp := app
			if mode == spec.ModeNone || (mode == spec.ModeUnknown && sticky == "" && variant != "local") {
				wantApp = ""
			}
			if res.Mode != mode || (wantApp != "" && res.App != wantApp) {
				t.Errorf("%s [%s]: got %s/%s, want %s/%s (%s)",
					rel, variant, orDash(res.App), res.Mode, orDash(app), mode, res.Reason)
			}
		}
		t.Run(rel, func(t *testing.T) {
			check("remote", f.Screen, "")
			if app != "" {
				local := *f.Screen
				local.Command = app
				if strings.HasPrefix(rel, "nested"+string(filepath.Separator)) {
					local.Command = "tmux"
				}
				check("local", &local, "")
				check("sticky", f.Screen, app)
			}
			mono := *f.Screen
			mono.Cells = nil
			check("mono", &mono, "")
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no fixtures found")
	}
	t.Logf("%d fixtures", n)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
