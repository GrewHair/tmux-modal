package spec_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
)

func load(t *testing.T, files map[string]string) *spec.Set {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return spec.Load([]spec.Source{{FS: fsys, Dir: ".", Label: "test"}})
}

func mustSpec(t *testing.T, set *spec.Set, name string) *spec.Spec {
	t.Helper()
	sp := set.Specs[name]
	if sp == nil {
		t.Fatalf("spec %s did not load; warnings: %v", name, set.Warnings)
	}
	return sp
}

func scr(lines ...string) *screen.Screen {
	s := screen.FromText(strings.Join(lines, "\n"), 40, len(lines))
	s.AltScreen = true
	return s
}

const group = `
modes = ["normal", "insert"]
[buckets]
typing = ["insert"]
commanding = ["normal"]
[match]
requires_alt_screen = true
[[mode_when]]
name = "parent-rule"
mode = "insert"
regex = '^PARENT PROMPT:'
rows = [-1, -1]
[keys]
h = "Left"
j = "Down"
`

func TestInheritanceMerge(t *testing.T) {
	set := load(t, map[string]string{
		"groups/base.toml": group,
		"app.toml": `
name = "app"
extends = "base"
[match]
command = ["app"]
[[mode_when]]
name = "child-rule"
mode = "insert"
regex = '^CHILD PROMPT:'
rows = [-1, -1]
[keys]
h = false
k = "Up"
`,
	})
	sp := mustSpec(t, set, "app")
	if len(sp.ModeRules) != 2 || sp.ModeRules[0].Name != "child-rule" || sp.ModeRules[1].Name != "parent-rule" {
		t.Fatalf("rule order: child rules must come first, got %v", ruleNames(sp))
	}
	keys := map[string]string{}
	for _, k := range sp.Keys {
		keys[k.Key] = k.Send
	}
	if _, ok := keys["h"]; ok {
		t.Errorf("h = false must suppress the inherited binding")
	}
	if keys["j"] != "Down" || keys["k"] != "Up" {
		t.Errorf("keys merge: got %v", keys)
	}
	if !sp.Identity.RequiresAlt {
		t.Errorf("requires_alt_screen not inherited through [match] table merge")
	}
	if got := strings.Join(sp.Chain, ">"); got != "app>base" {
		t.Errorf("chain = %s", got)
	}
	if _, ok := set.Specs["base"]; !ok || !set.Specs["base"].Group {
		t.Errorf("group not loaded as group")
	}
	for _, s := range set.Order {
		if s.Group {
			t.Errorf("group %s is matchable", s.Name)
		}
	}
}

func ruleNames(sp *spec.Spec) []string {
	var n []string
	for _, r := range sp.ModeRules {
		n = append(n, r.Name)
	}
	return n
}

func TestExtendsCycleAndErrorsAreSkippedNotFatal(t *testing.T) {
	set := load(t, map[string]string{
		"a.toml":    "extends = \"b\"\nmodes=[\"normal\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"a\"]\n",
		"b.toml":    "extends = \"a\"\nmodes=[\"normal\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"b\"]\n",
		"bad.toml":  "this is = = not toml",
		"nob.toml":  "modes=[\"normal\",\"insert\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"x\"]\n",
		"good.toml": "modes=[\"normal\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"good\"]\n",
	})
	if _, ok := set.Specs["good"]; !ok {
		t.Fatalf("a good spec must load despite broken siblings: %v", set.Warnings)
	}
	joined := strings.Join(set.Warnings, "\n")
	for _, want := range []string{"extends cycle", "bad.toml", `mode "insert" has no bucket`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning %q in:\n%s", want, joined)
		}
	}
}

func TestUserDirShadowsBundledByFilename(t *testing.T) {
	bundled := fstest.MapFS{"htop.toml": {Data: []byte("name=\"htop\"\npriority=1\nmodes=[\"normal\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"htop\"]\n")}}
	user := fstest.MapFS{"htop.toml": {Data: []byte("name=\"htop\"\npriority=99\nmodes=[\"normal\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"htop\"]\n")}}
	set := spec.Load([]spec.Source{{FS: bundled, Dir: ".", Label: "b"}, {FS: user, Dir: ".", Label: "u"}})
	if p := mustSpec(t, set, "htop").Priority; p != 99 {
		t.Errorf("user spec should shadow bundled, priority = %d", p)
	}
}

// vimLike is the §5.2.1 example with one change: the brief gives the
// clauses weights 70/50/40 against a threshold of 100 and says "any two of
// those three clear the threshold", but 50+40 = 90 does not. The weights
// here (70/60/50) make that sentence true.
const vimLike = `
name = "vimlike"
modes = ["normal", "insert"]
[buckets]
typing = ["insert"]
commanding = ["normal"]
[match]
requires_alt_screen = true
combine = "weighted"
threshold = 100
  [[match.clause]]
  regex = '^~$'
  rows = [1, -3]
  col = 0
  min_occurrences = 3
  contiguous = true
  weight = 70
  [[match.clause]]
  regex = '[0-9]+,[0-9-]+ +(All|Top|Bot|[0-9]+%)'
  rows = [-2, -1]
  anchor = "end"
  weight = 60
  [[match.clause]]
  regex = '^".*" +([0-9]+L|\[New\])'
  rows = [-2, -1]
  col = 0
  weight = 50
[[mode_when]]
mode = "insert"
regex = '^-- (INSERT|EINFÜGEN)'
rows = [-3, -1]
col = 0
[[corroborate]]
if_cursor_shape = "bar"
veto_modes = ["normal"]
then_mode = "insert"
`

func TestWeightedIdentityTwoOfThree(t *testing.T) {
	sp := mustSpec(t, load(t, map[string]string{"v.toml": vimLike}), "vimlike")
	cases := []struct {
		name  string
		lines []string
		fire  bool
	}{
		{"tildes+ruler", []string{"text", "~", "~", "~", "~", "", "               1,1           All"}, true},
		{"ruler+message", []string{"text", "more", "more", "more", "", `"f.txt" 12L, 80B`, "               1,1           All"}, true},
		{"ruler only", []string{"text", "more", "more", "more", "", "", "               1,1           All"}, false},
		// A markdown fence and ~/paths must not count as tilde rows.
		{"fences are not tildes", []string{"x", "~~~", "~/src", "~~~", "~", "", "               1,1           All"}, false},
		// Tildes separated by text are not contiguous.
		{"scattered tildes", []string{"x", "~", "a", "~", "b", "~", "", "               1,1           All"}, false},
	}
	for _, c := range cases {
		s := scr(c.lines...)
		r := sp.Identity.Screen.Eval(s)
		if r.Fired != c.fire {
			t.Errorf("%s: fired=%v score=%g, want %v", c.name, r.Fired, r.Score, c.fire)
		}
	}
}

func TestNormalRequiresConfirmedIdentity(t *testing.T) {
	sp := mustSpec(t, load(t, map[string]string{"v.toml": vimLike}), "vimlike")
	confirmed := scr("text", "~", "~", "~", "~", "", "               1,1           All")
	if tr := classify.Classify(sp, confirmed, true); tr.Result.Mode != "normal" || tr.Result.Confidence != "high" {
		t.Errorf("confirmed + no marker: got %s/%s", tr.Result.Mode, tr.Result.Confidence)
	}
	// A degraded capture (mid-redraw: nothing recognisable) must read as
	// unknown, never normal, even though the app is sticky.
	blank := scr("", "", "", "", "", "", "")
	if tr := classify.Classify(sp, blank, true); tr.Result.Mode != spec.ModeUnknown {
		t.Errorf("degraded capture: got %s, want unknown", tr.Result.Mode)
	}
	// A positive marker is trusted on first sight even without anchors.
	ins := scr("", "", "", "", "", "", "-- INSERT --")
	if tr := classify.Classify(sp, ins, true); tr.Result.Mode != "insert" {
		t.Errorf("marker on sticky app: got %s", tr.Result.Mode)
	}
	// Not sticky and not identified: this spec does not claim the pane.
	if tr := classify.Classify(sp, blank, false); tr.Result.App != "" {
		t.Errorf("unidentified pane claimed by %s", tr.Result.App)
	}
}

func TestCorroborationVetoAndUnavailableShape(t *testing.T) {
	sp := mustSpec(t, load(t, map[string]string{"v.toml": vimLike}), "vimlike")
	s := scr("text", "~", "~", "~", "~", "", "               1,1           All")
	s.Cursor.Shape = "bar"
	tr := classify.Classify(sp, s, true)
	if tr.Result.Mode != "insert" || tr.Result.Confidence != "low" {
		t.Errorf("bar cursor should veto normal at low confidence, got %s/%s", tr.Result.Mode, tr.Result.Confidence)
	}
	// tmux < 3.5 reports no shape: the clause is unavailable, contributes
	// nothing either way, and normal stands.
	s.Cursor.Shape = ""
	if tr := classify.Classify(sp, s, true); tr.Result.Mode != "normal" {
		t.Errorf("unreported shape must not veto, got %s", tr.Result.Mode)
	}
}

func clauseSpec(t *testing.T, clause string) *spec.Rule {
	t.Helper()
	body := "modes=[\"normal\",\"insert\"]\n[buckets]\ntyping=[\"insert\"]\ncommanding=[\"normal\"]\n" +
		"[match]\ncommand=[\"x\"]\n[[mode_when]]\nmode=\"insert\"\n" + clause
	sp := mustSpec(t, load(t, map[string]string{"x.toml": body}), "x")
	return sp.ModeRules[0]
}

func TestGeometry(t *testing.T) {
	s := scr("Filter: top row", "", "log line mentioning Filter: here", "EnterDone  EscClear    Filter: abc")
	cases := []struct {
		clause string
		want   bool
	}{
		{"regex='Filter:'\nrows=[-1,-1]", true},
		{"regex='Filter:'\nrows=[-1,-1]\ncol=0", false},             // match starts at col 23
		{"regex='Filter:'\nrows=[-1,-1]\ncols=[20,30]", true},       // within range
		{"regex='Filter: abc'\nrows=[-1,-1]\nanchor=\"end\"", true}, // ends at last non-blank
		{"regex='Filter:'\nrows=[-1,-1]\nanchor=\"end\"", false},
		{"regex='Filter:'\nrows=[1,2]\nanchor=\"start\"", false}, // row 2 has it mid-line
		{"regex='Filter:'\nrow=0\nanchor=\"start\"", true},
		{"regex='^$'\nrows=[1,1]", true},
		{"regex='Filter:'\nrows=[-1,-1]\nnegate=true", false},
	}
	for _, c := range cases {
		r := clauseSpec(t, c.clause)
		if got := r.Eval(s).Fired; got != c.want {
			t.Errorf("%q: fired=%v want %v", c.clause, got, c.want)
		}
	}
}

func TestPartialCaptureIsUnavailableNotAbsent(t *testing.T) {
	r := clauseSpec(t, "regex='^PROMPT'\nrows=[0,3]")
	s := scr("PROMPT", "", "", "")
	s.Top, s.Lines = 2, s.Lines[2:] // only the bottom two rows captured
	res := r.Eval(s)
	if res.Clauses[0].State != spec.Unavailable {
		t.Errorf("clause over uncaptured rows: state %v, want n/a", res.Clauses[0].State)
	}
	neg := clauseSpec(t, "regex='^PROMPT'\nrows=[0,3]\nnegate=true")
	if st := neg.Eval(s).Clauses[0].State; st != spec.Unavailable {
		t.Errorf("negation must not turn unavailable into evidence: %v", st)
	}
}

func TestColourNormalisation(t *testing.T) {
	ansi := "\x1b[30m\x1b[46mX\x1b[0m\x1b[38;2;0;205;205mY\x1b[38;5;6mZ\n"
	s := scr("XYZ")
	s.Height, s.Lines = 1, []string{"XYZ"}
	s.Cells = screen.ParseANSI(ansi, 1)
	cases := []struct {
		clause string
		want   bool
	}{
		{"[[mode_when.color]]\nrow=0\ncol=0\nfg=\"black\"\nbg=\"cyan\"\nweight=100", true},
		{"[[mode_when.color]]\nrow=0\ncol=0\nbg=6\nweight=100", true},
		{"[[mode_when.color]]\nrow=0\ncol=1\nfg=\"cyan\"\nweight=100", true}, // truecolour (0,205,205) == xterm cyan
		{"[[mode_when.color]]\nrow=0\ncol=1\nfg=\"#00c8c8\"\ntolerance=10\nweight=100", true},
		{"[[mode_when.color]]\nrow=0\ncol=1\nfg=\"#ff0000\"\nweight=100", false},
		{"[[mode_when.color]]\nrow=0\ncol=2\nfg=\"cyan\"\nweight=100", true}, // 256-colour index 6
		{"[[mode_when.color]]\nrow=0\ncol=0\nfg=\"black\"\nattrs=[\"bold\"]\nweight=100", false},
	}
	for _, c := range cases {
		r := clauseSpec(t, "regex='XYZ'\nrow=0\n"+c.clause)
		res := r.Eval(s)
		if got := res.Clauses[1].State == spec.Satisfied; got != c.want {
			t.Errorf("%q: satisfied=%v want %v (%v)", c.clause, got, c.want, res.Clauses[1].Hits)
		}
	}
}

func TestBonusColourNeverBlocksAndNeverCaptures(t *testing.T) {
	r := clauseSpec(t, "regex='^PROMPT:'\nrow=0\n[[mode_when.color]]\nrow=0\ncol=0\nbg=\"red\"")
	s := scr("PROMPT: x")
	if !r.Eval(s).Fired {
		t.Errorf("absent bonus colour (no colour capture) must not fail the rule")
	}
	body := "modes=[\"normal\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"x\"]\n" +
		"[[match.clause]]\nrow=0\ncol=0\nbg=\"red\"\nweight=\"bonus\"\n[[match.clause]]\nregex='^PROMPT: *x'\nrows=[0,0]\n"
	sp := mustSpec(t, load(t, map[string]string{"x.toml": body}), "x")
	if sp.NeedsColour {
		t.Errorf("a bonus-only colour clause must not force -e captures")
	}
}

func TestTierDerivedFromClauses(t *testing.T) {
	if r := clauseSpec(t, "cursor_rows=[-1,-1]\ncursor_visible=true"); r.Tier != 1 {
		t.Errorf("pure cursor rule tier = %d, want 1", r.Tier)
	}
	if r := clauseSpec(t, "regex='^: +'\nrow=-1\ncursor_rows=[-1,-1]"); r.Tier != 2 {
		t.Errorf("regex rule tier = %d, want 2", r.Tier)
	}
}

func TestLintUnderSpecified(t *testing.T) {
	body := "modes=[\"normal\",\"insert\"]\n[buckets]\ntyping=[\"insert\"]\ncommanding=[\"normal\"]\n" +
		"[match]\ncommand=[\"x\"]\n[[insert_when]]\nregex='Filter:'\n"
	sp := mustSpec(t, load(t, map[string]string{"x.toml": body}), "x")
	w := strings.Join(sp.Warnings, "\n")
	if !strings.Contains(w, "no geometry") || !strings.Contains(w, "under-specified") {
		t.Errorf("expected geometry and under-specification warnings, got:\n%s", w)
	}
}

func TestLintTrailingSpace(t *testing.T) {
	body := "modes=[\"normal\",\"insert\"]\n[buckets]\ntyping=[\"insert\"]\ncommanding=[\"normal\"]\n" +
		"[match]\ncommand=[\"x\"]\n[[insert_when]]\nregex='^PROMPT NAME HERE> '\nrow=-1\ncol=0\n"
	sp := mustSpec(t, load(t, map[string]string{"x.toml": body}), "x")
	if !strings.Contains(strings.Join(sp.Warnings, "\n"), "ends in whitespace") {
		t.Errorf("expected trailing-whitespace warning, got %v", sp.Warnings)
	}
}

// A remapping spec must not conclude its commanding default from the
// absence of a bottom-row prompt unless an identity clause it cannot do
// without vouches for that row (a remote tmux status line covers it).
func TestLintAbsenceAnchor(t *testing.T) {
	body := func(weight string) string {
		return `name = "app"
modes = ["normal", "insert"]
[buckets]
typing = ["insert"]
commanding = ["normal"]
[match]
requires_alt_screen = true
combine = "weighted"
threshold = 40
  [[match.clause]]
  regex = '^APP HEADER LINE TITLE'
  row = 0
  weight = 50
  [[match.clause]]
  regex = '^F1 Help +F2 Quit bottom bar'
  row = -1
  weight = ` + weight + `
[[insert_when]]
regex = '^Search prompt here:( |$)'
row = -1
[keys]
j = "Down"
`
	}
	warned := func(weight string) bool {
		sp := mustSpec(t, load(t, map[string]string{"app.toml": body(weight)}), "app")
		return strings.Contains(strings.Join(sp.Warnings, "\n"), "no required identity clause anchors those rows")
	}
	if !warned("30") {
		t.Error("an optional bottom-bar anchor must be flagged")
	}
	if warned(`"required"`) {
		t.Error("a required bottom-bar anchor must satisfy the lint")
	}
	// A weighted clause the threshold cannot do without binds like a
	// required one: 50 + 50 against 100 (ranger, lf, nnn).
	w := func(sub ...string) bool {
		b := body("50")
		for i := 0; i < len(sub); i += 2 {
			if !strings.Contains(b, sub[i]) {
				t.Fatalf("test body lacks %q", sub[i])
			}
		}
		b = strings.NewReplacer(sub...).Replace(b)
		sp := mustSpec(t, load(t, map[string]string{"app.toml": b}), "app")
		return strings.Contains(strings.Join(sp.Warnings, "\n"), "no required identity clause anchors those rows")
	}
	if w("threshold = 40", "threshold = 100") {
		t.Error("a weighted anchor the threshold needs must satisfy the lint")
	}
	// An anchor pinned to one row pins the app to that edge: row -2 vouches
	// for a prompt on row -1 (tig), row 0 does not (htop).
	if w("threshold = 40", "threshold = 100", "row = -1\n  weight", "row = -2\n  weight") {
		t.Error("an anchor pinned on row -2 must vouch for row -1")
	}
	if !w("threshold = 40", "threshold = 100", "'^F1 Help +F2 Quit bottom bar'\n  row = -1", "'^F1 Help +F2 Quit bottom bar'\n  row = 1") {
		t.Error("anchors pinned at the top must not vouch for the bottom row")
	}

	// Mixed ranges ([0, -2]: all but the last row, as btop's filter rule
	// reads) are compared by resolving them: the anchor must cover them.
	mixed := func(idRows string) bool {
		sp := mustSpec(t, load(t, map[string]string{"m.toml": `name = "m"
modes = ["normal", "insert"]
[buckets]
typing = ["insert"]
commanding = ["normal"]
[match]
  [[match.clause]]
  regex = 'BOX TITLE ANCHOR TEXT'
  rows = ` + idRows + `
  weight = "required"
[[insert_when]]
regex = 'BOX TITLE ANCHOR TEXT filter:'
rows = [0, -2]
[keys]
j = "Down"
`}), "m")
		return strings.Contains(strings.Join(sp.Warnings, "\n"), "no required identity clause anchors those rows")
	}
	if mixed("[0, -2]") {
		t.Error("an anchor over the same mixed range must satisfy the lint")
	}
	if !mixed("[0, 3]") {
		t.Error("an anchor over only the top rows must not cover [0, -2]")
	}
}

func TestCommandGlobAndLintIgnore(t *testing.T) {
	set := load(t, map[string]string{"py.toml": `
always = "insert"
modes = ["insert"]
lint_ignore = ["under-specified"]
[buckets]
typing = ["insert"]
[match]
command = ["python3", "python3.*"]
  [[match.clause]]
  regex = '^>>>'
  row = -1
`})
	sp := mustSpec(t, set, "py")
	for cmd, want := range map[string]bool{"python3": true, "python3.12": true, "python": false, "python3x": false} {
		if got := sp.MatchesCommand(cmd); got != want {
			t.Errorf("MatchesCommand(%q) = %v, want %v", cmd, got, want)
		}
	}
	if len(sp.Warnings) != 0 {
		t.Errorf("lint_ignore did not suppress: %v", sp.Warnings)
	}
	bad := load(t, map[string]string{"bad.toml": `
always = "insert"
modes = ["insert"]
[buckets]
typing = ["insert"]
[match]
command = ["[python"]
`})
	if bad.Specs["bad"] != nil || !strings.Contains(strings.Join(bad.Warnings, "\n"), "match.command") {
		t.Errorf("a malformed command glob must fail the spec: %v", bad.Warnings)
	}
}

// An overlay user file merges onto the bundled spec of the same file name
// instead of replacing it: bundled rules keep applying, the user's come
// first, [keys] merge key by key and false removes one.
func TestOverlayMergesOntoBundled(t *testing.T) {
	bundled := fstest.MapFS{
		"groups/g.toml": {Data: []byte(group)},
		"app.toml": {Data: []byte(`
extends = "g"
priority = 30
lint_ignore = ["under-specified", "no screen identity"]
[match]
command = ["app"]
[[mode_when]]
name = "bundled-rule"
mode = "insert"
regex = '^Find:'
row = -1
col = 0
[keys]
k = "Up"
`)},
	}
	user := fstest.MapFS{
		"app.toml": {Data: []byte(`
overlay = true
name = "renamed"
[[mode_when]]
name = "user-rule"
mode = "insert"
regex = '^Jump to:'
row = -1
col = 0
[keys]
k = false
l = "Right"
`)},
		"nothing.toml": {Data: []byte("overlay = true\n[keys]\nh = \"Left\"\n")},
	}
	set := spec.Load([]spec.Source{{FS: bundled, Dir: ".", Label: "b"}, {FS: user, Dir: ".", Label: "u"}})
	sp := mustSpec(t, set, "app")
	if set.Specs["renamed"] != nil {
		t.Error("an overlay must not rename the spec")
	}
	if sp.Priority != 30 || len(sp.Identity.Commands) != 1 {
		t.Errorf("bundled scalars lost: priority %d, commands %v", sp.Priority, sp.Identity.Commands)
	}
	if !strings.Contains(sp.File, "b:") || !strings.Contains(sp.File, "u:") {
		t.Errorf("File should name both files: %q", sp.File)
	}
	var rules []string
	for _, r := range sp.ModeRules {
		rules = append(rules, r.Name)
	}
	if got := strings.Join(rules, ","); got != "user-rule,bundled-rule,parent-rule" {
		t.Errorf("rule order = %s", got)
	}
	keys := map[string]string{}
	for _, k := range sp.Keys {
		keys[k.Key] = k.Send
	}
	if len(keys) != 3 || keys["h"] != "Left" || keys["j"] != "Down" || keys["l"] != "Right" {
		t.Errorf("keys = %v (want h j from the group, l from the overlay, k removed)", keys)
	}
	if len(sp.Warnings) != 0 {
		t.Errorf("the bundled lint_ignore should still apply: %v", sp.Warnings)
	}
	w := strings.Join(set.Warnings, "\n")
	if !strings.Contains(w, "no nothing.toml to overlay") || !strings.Contains(w, `name "renamed" ignored`) {
		t.Errorf("warnings = %v", set.Warnings)
	}

	// keys = false drops the whole inherited key map.
	user["app.toml"] = &fstest.MapFile{Data: []byte("overlay = true\nkeys = false\n")}
	set = spec.Load([]spec.Source{{FS: bundled, Dir: ".", Label: "b"}, {FS: user, Dir: ".", Label: "u"}})
	if sp := mustSpec(t, set, "app"); sp.HasKeys() || len(sp.ModeRules) != 2 {
		t.Errorf("keys = false: keys %v, %d rules", sp.Keys, len(sp.ModeRules))
	}

	// Without overlay the user file still shadows the bundled one outright.
	user["app.toml"] = &fstest.MapFile{Data: []byte("modes=[\"normal\"]\n[buckets]\ncommanding=[\"normal\"]\n[match]\ncommand=[\"app\"]\n")}
	set = spec.Load([]spec.Source{{FS: bundled, Dir: ".", Label: "b"}, {FS: user, Dir: ".", Label: "u"}})
	if sp := mustSpec(t, set, "app"); len(sp.ModeRules) != 0 || sp.HasKeys() {
		t.Errorf("a plain user file should replace the bundled spec, got %d rules", len(sp.ModeRules))
	}
}
