package classify_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

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
//
// expect_mode_<variant> overrides the expectation for one variant, where
// the evidence really differs: e.g. mono where a spec needs colour to be
// sure (it must then fail towards a typing mode or unknown, never
// commanding), or remote where only the command identifies the app.
// expect_app_<variant> likewise, where only the command tells two apps
// apart (a remote vim with split windows reads as nvim).
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
			mode := mode
			if m := f.Meta["expect_mode_"+variant]; m != "" {
				mode = m
			}
			wantApp := app
			if a := f.Meta["expect_app_"+variant]; a != "" {
				wantApp = a
			}
			if mode == spec.ModeNone || (mode == spec.ModeUnknown && sticky == "" && variant != "local") {
				wantApp = ""
			}
			if res.Mode != mode || (wantApp != "" && res.App != wantApp) {
				t.Errorf("%s [%s]: got %s/%s, want %s/%s (%s)",
					rel, variant, orDash(res.App), res.Mode, orDash(wantApp), mode, res.Reason)
			}
		}
		t.Run(rel, func(t *testing.T) {
			check("remote", f.Screen, "")
			if strings.HasPrefix(rel, "nested"+string(filepath.Separator)) {
				// With @modal_nested_remap off, a remapping app is never
				// given a mode through a nested tmux.
				// Hooks-only specs (vim) keep their modes.
				res, _ := classify.PaneWith(set, f.Screen, "", classify.Options{NestedRemap: false})
				if sp := set.Specs[res.App]; (sp == nil || sp.HasKeys()) && (res.Mode != spec.ModeUnknown || res.Nested == "") {
					t.Errorf("%s [nested_remap off]: got %s/%s nested=%q, want unknown", rel, orDash(res.App), res.Mode, res.Nested)
				}
			}
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

// Bundled specs ship lint-clean: a warning is either fixed or explicitly
// accepted in the spec with lint_ignore and a comment saying why.
func TestBundledSpecsLintClean(t *testing.T) {
	set := bundledSet(t)
	for name, sp := range set.Specs {
		for _, w := range sp.Warnings {
			t.Errorf("%s: %s", name, w)
		}
	}
	// Every bundled app is matchable, and a shell never is.
	for _, sh := range []string{"bash", "zsh", "fish", "sh"} {
		for _, sp := range set.Order {
			if sp.MatchesCommand(sh) {
				t.Errorf("%s claims the shell %s", sp.Name, sh)
			}
		}
	}
}

// TestBundledSpecsRemapReady holds every bundled spec to the remapping lint
// as if a user overlay had given it keys (D29): any app may be remapped
// without first tightening its spec. Always-insert specs never remap, and
// neither does vim-family (brief §6.2, D30): its identity is weighted on
// purpose, with no clause that is always there, so it can never pass;
// an overlay giving vim keys gets the lint warning in the daemon log.
func TestBundledSpecsRemapReady(t *testing.T) {
	entries, err := fs.ReadDir(tmuxmodal.Specs, "specs")
	if err != nil {
		t.Fatal(err)
	}
	overlays := fstest.MapFS{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
			overlays[e.Name()] = &fstest.MapFile{Data: []byte("overlay = true\n[keys]\nF12 = \"F12\"\n")}
		}
	}
	set := spec.Load([]spec.Source{
		{FS: tmuxmodal.Specs, Dir: "specs", Label: "bundled"},
		{FS: overlays, Dir: ".", Label: "overlay"},
	})
	for _, w := range set.Warnings {
		t.Errorf("load: %s", w)
	}
	for _, sp := range set.Order {
		if sp.Always != "" || contains(sp.Chain, "vim-family") {
			continue
		}
		if !sp.HasKeys() {
			t.Errorf("%s: the overlay's keys did not apply", sp.Name)
		}
		for _, w := range sp.Warnings {
			t.Errorf("%s: %s", sp.Name, w)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}
