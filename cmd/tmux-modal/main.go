// Command tmux-modal detects and publishes the modal state of whatever runs
// in each tmux pane, including applications on the far side of SSH.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tmuxmodal "github.com/GrewHair/tmux-modal"
	"github.com/GrewHair/tmux-modal/internal/classify"
	"github.com/GrewHair/tmux-modal/internal/daemon"
	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
	"github.com/GrewHair/tmux-modal/internal/validate"
)

var version = "dev"

const usage = `usage: tmux-modal <command> [flags]

commands:
  daemon     run the detector for the tmux server in $TMUX (started by modal.tmux)
  stop       stop the running daemon and restore key tables
  validate   lint a spec and score it against a screen
  explain    show why the daemon sees a pane the way it does
  capture    save a pane as a test fixture
  lint       load every spec and report warnings and errors
  version    print the version

run 'tmux-modal <command> -h' for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "daemon":
		err = daemon.Main(args, bundled())
	case "stop":
		err = daemon.Stop(args)
	case "validate":
		err = cmdValidate(args)
	case "capture":
		err = cmdCapture(args)
	case "explain":
		err = cmdExplain(args)
	case "lint":
		err = cmdLint(args)
	case "version", "--version", "-v":
		fmt.Println("tmux-modal", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "tmux-modal: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmux-modal %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func bundled() spec.Source {
	return spec.Source{FS: tmuxmodal.Specs, Dir: "specs", Label: "bundled"}
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// sources returns bundled specs followed by user directories: explicit
// --spec-path flags, else @modal_spec_paths from the running tmux, else the
// default directory.
func sources(paths []string, noBundled bool) []spec.Source {
	var src []spec.Source
	if !noBundled {
		src = append(src, bundled())
	}
	if len(paths) == 0 {
		paths = daemon.DefaultSpecPaths(tmux.Exec{Server: tmux.FromEnv()})
	}
	for _, p := range paths {
		src = append(src, spec.DirSource(p))
	}
	return src
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	var paths multiFlag
	fs.Var(&paths, "spec-path", "extra spec directory (repeatable)")
	capture := fs.String("capture", "", "score against a live pane (tmux target, e.g. %3)")
	fixture := fs.String("fixture", "", "score against a saved fixture file")
	colour := fs.Bool("colour", true, "include colour when capturing a live pane")
	sticky := fs.Bool("sticky", false, "evaluate as if the app were already identified on this pane")
	resolved := fs.Bool("resolved", true, "print the resolved spec after inheritance")
	locale := fs.Bool("locale-audit", false, "list modes whose markers have no non-English alternates")
	noBundled := fs.Bool("no-bundled", false, "do not load the bundled specs")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: tmux-modal validate [flags] [spec.toml | spec-name]\n\n"+
			"With a spec and no screen: lint it and print the resolved spec.\n"+
			"With --capture or --fixture: also print the per-clause score sheet.\n"+
			"With a screen and no spec: run full identification across every spec.\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	target := fs.Arg(0)
	isFile := strings.HasSuffix(target, ".toml")
	srcs := sources(paths, *noBundled)
	if isFile {
		if _, err := os.Stat(target); err != nil {
			return err
		}
		// A file already in a spec path is loaded with it; loading it
		// twice would apply an overlay twice.
		if !inSpecPath(target, srcs) {
			srcs = append(srcs, spec.FileSource(target))
		}
	}
	set := spec.Load(srcs)
	out := os.Stdout
	for _, w := range set.Warnings {
		if target == "" || strings.Contains(w, filepath.Base(target)) {
			fmt.Fprintf(out, "load: %s\n", w)
		}
	}

	var sp *spec.Spec
	if target != "" {
		name := target
		if isFile {
			name = specNameOf(target, set)
		}
		sp = set.Specs[name]
		if sp == nil {
			return fmt.Errorf("spec %q did not load (see warnings above)", name)
		}
		validate.Spec(out, sp, validate.Options{ShowResolved: *resolved, LocaleAudit: *locale})
	}

	var s *screen.Screen
	var src string
	switch {
	case *fixture != "":
		f, err := screen.LoadFixture(*fixture)
		if err != nil {
			return err
		}
		s, src = f.Screen, "fixture "+*fixture
	case *capture != "":
		var err error
		s, _, _, err = tmux.Capture(tmux.Exec{Server: tmux.FromEnv()}, *capture, *colour)
		if err != nil {
			return err
		}
		src = "live pane " + *capture
	default:
		if sp == nil {
			return fmt.Errorf("nothing to do: give a spec, --capture or --fixture")
		}
		return nil
	}
	fmt.Fprintln(out)
	validate.Screen(out, src, s)
	if sp != nil {
		validate.Trace(out, classify.Classify(sp, s, *sticky))
		return nil
	}
	res, traces := classify.Pane(set, s, "")
	for _, t := range traces {
		validate.Trace(out, t)
	}
	if res.Nested != "" {
		fmt.Fprintf(out, "\nnested multiplexer detected (%s); any inner status line was removed before the rules above ran\n", res.Nested)
	}
	fmt.Fprintf(out, "\nfinal: app=%s mode=%s bucket=%s confidence=%s (%s)\n",
		orDash(res.App), res.Mode, res.Bucket, res.Confidence, res.Reason)
	return nil
}

// cmdExplain prints what the daemon published for a pane (its @modal_*
// options) and a fresh classification of the pane with the full score
// sheet: meant for a popup bound to a key while dogfooding.
func cmdExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ExitOnError)
	var paths multiFlag
	fs.Var(&paths, "spec-path", "extra spec directory (repeatable)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: tmux-modal explain [flags] [pane]   (default: $TMUX_PANE)\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	target := fs.Arg(0)
	if target == "" {
		target = os.Getenv("TMUX_PANE")
	}
	if target == "" {
		return fmt.Errorf("no pane: give one (e.g. %%3) or run inside tmux")
	}
	r := tmux.Exec{Server: tmux.FromEnv()}
	out := os.Stdout

	opts := daemon.PublishedOptions()
	parts := make([]string, len(opts))
	for i, o := range opts {
		parts[i] = "#{" + o + "}"
	}
	lines, err := r.Run("display-message", "-p", "-t", target, strings.Join(parts, "\t"))
	if err != nil {
		return err
	}
	vals := strings.Split(strings.Join(lines, "\n"), "\t")
	fmt.Fprintf(out, "published for %s:\n", target)
	for i, o := range opts {
		if i < len(vals) && vals[i] != "" && !strings.HasPrefix(o, "@modal_badge") && o != "@modal_indicator" {
			fmt.Fprintf(out, "  %-20s %s\n", o, vals[i])
		}
	}
	sticky := ""
	if len(vals) > 0 {
		sticky = vals[0] // @modal_app
	}
	remap := "on"
	if l, err := r.Run("show-options", "-gqv", "@modal_nested_remap"); err == nil && len(l) > 0 && l[0] != "" {
		remap = l[0]
	}
	var transports []string
	if l, err := r.Run("show-options", "-gqv", "@modal_transports"); err == nil && len(l) > 0 {
		transports = strings.Fields(l[0])
	}

	set := spec.Load(sources(paths, false))
	s, _, _, err := tmux.Capture(r, target, true)
	if err != nil {
		return err
	}
	fmt.Fprintln(out)
	validate.Screen(out, "live pane "+target, s)
	res, traces := classify.PaneWith(set, s, sticky, classify.Options{NestedRemap: remap != "off", Transports: transports})
	for _, t := range traces {
		validate.Trace(out, t)
	}
	fmt.Fprintf(out, "\nnow: app=%s mode=%s bucket=%s confidence=%s evidence=%s score=%s",
		orDash(res.App), res.Mode, res.Bucket, res.Confidence, orDash(res.Evidence), orDash(res.Score))
	if res.Via != "" {
		fmt.Fprintf(out, " via=%s", res.Via)
	}
	if res.Nested != "" {
		fmt.Fprintf(out, " nested=%s(%s)", res.NestedKind, res.Nested)
	}
	if res.NestedOff != "" {
		fmt.Fprintf(out, " nested=%s(%s; off: the command is no transport)", res.NestedKind, res.NestedOff)
	}
	if res.InnerPanes > 0 {
		fmt.Fprintf(out, " split=%d focus=%s", res.InnerPanes, orDash(res.FocusBy))
	}
	fmt.Fprintf(out, "\n     %s\n", res.Reason)
	if sticky != "" {
		fmt.Fprintf(out, "     (classified as the daemon does, with %s remembered)\n", sticky)
	}
	return nil
}

// specNameOf finds the spec a file defines, or overlays, in a loaded set.
func specNameOf(file string, set *spec.Set) string {
	suffix := ":" + filepath.Base(file)
	for name, sp := range set.Specs {
		for _, f := range strings.Split(sp.File, " + ") {
			if strings.HasSuffix(f, suffix) && sameFile(filepath.Dir(file), strings.TrimSuffix(f, suffix)) {
				return name
			}
		}
	}
	return strings.TrimSuffix(filepath.Base(file), ".toml")
}

// inSpecPath reports whether a spec file sits in one of the loaded
// directories.
func inSpecPath(file string, srcs []spec.Source) bool {
	for _, s := range srcs {
		if s.Dir == "." && sameFile(filepath.Dir(file), s.Label) {
			return true
		}
	}
	return false
}

func sameFile(a, b string) bool {
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func cmdCapture(args []string) error {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	out := fs.String("o", "", "output fixture path (.txt); a .ansi sibling is written with colour")
	colour := fs.Bool("colour", true, "also save the colour capture")
	app := fs.String("expect-app", "", "expected app, recorded in the fixture header")
	mode := fs.String("expect-mode", "", "expected mode, recorded in the fixture header")
	note := fs.String("note", "", "free-text note recorded in the header")
	var metas multiFlag
	fs.Var(&metas, "meta", "extra header line key=value (repeatable), e.g. expect_mode_mono=insert")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: tmux-modal capture [flags] <pane>\n\nflags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 || *out == "" {
		fs.Usage()
		os.Exit(2)
	}
	s, plain, ansi, err := tmux.Capture(tmux.Exec{Server: tmux.FromEnv()}, fs.Arg(0), *colour)
	if err != nil {
		return err
	}
	extra := map[string]string{}
	if *app != "" {
		extra["expect_app"] = *app
	}
	if *mode != "" {
		extra["expect_mode"] = *mode
	}
	if *note != "" {
		extra["note"] = *note
	}
	for _, m := range metas {
		k, v, ok := strings.Cut(m, "=")
		if !ok {
			return fmt.Errorf("-meta %q: want key=value", m)
		}
		extra[k] = v
	}
	return screen.WriteFixture(*out, s, strings.Join(plain, "\n"), strings.Join(ansi, "\n"), extra)
}

func cmdLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	var paths multiFlag
	fs.Var(&paths, "spec-path", "extra spec directory (repeatable)")
	locale := fs.Bool("locale-audit", false, "include the locale audit")
	fs.Parse(args)
	set := spec.Load(sources(paths, false))
	bad := len(set.Warnings)
	for _, w := range set.Warnings {
		fmt.Printf("ERROR %s\n", w)
	}
	for _, name := range sortedNames(set) {
		sp := set.Specs[name]
		ws := append([]string{}, sp.Warnings...)
		if *locale {
			ws = append(ws, sp.LocaleAudit()...)
		}
		status := "ok"
		if len(ws) > 0 {
			status = fmt.Sprintf("%d warning(s)", len(ws))
		}
		fmt.Printf("%-14s %s\n", name, status)
		for _, w := range ws {
			fmt.Printf("    - %s\n", w)
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d spec(s) failed to load", bad)
	}
	return nil
}

func sortedNames(set *spec.Set) []string {
	var names []string
	for n := range set.Specs {
		names = append(names, n)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}
