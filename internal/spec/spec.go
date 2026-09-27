// Package spec loads, merges and compiles application specs, and evaluates
// their rules against captured screens.
package spec

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Buckets.
const (
	BucketTyping     = "typing"
	BucketCommanding = "commanding"
)

// Reserved modes that a spec may not declare.
const (
	ModeNone    = "none"
	ModeUnknown = "unknown"
)

// Rule is a list of weighted clauses combined against a threshold.
type Rule struct {
	Name      string
	Mode      string
	Clauses   []*Clause
	Threshold float64
	Combine   string
	Tier      int  // 1: format variables only; 2: needs a capture
	Colour    bool // needs a -e capture
}

// Corroboration is consulted only when no mode rule fired. It can veto
// the default conclusion and substitute its own, at reduced confidence.
type Corroboration struct {
	Rule
	VetoModes  []string
	ThenMode   string
	Confidence string
}

// Identity decides whether a pane is running this application.
type Identity struct {
	Commands    []string
	Title       *regexp.Regexp
	RequiresAlt bool
	Screen      *Rule // nil when the spec has no screen clauses
}

// KeyBinding is one normal-mode remap.
type KeyBinding struct {
	Key  string // key pressed
	Send string // key sent to the pane
}

// Spec is a fully resolved application spec.
type Spec struct {
	Name     string
	File     string
	Group    bool
	Priority int
	Modes    []string
	Buckets  map[string]string // mode -> bucket
	Always   string

	Identity      Identity
	ModeRules     []*Rule
	Corroborate   []*Corroboration
	DefaultMode   string
	OtherwiseMode string

	Keys         []KeyBinding // empty: hooks-only spec
	EscapeLeader string       // "" means use the global default
	Shadowed     map[string]string
	Hook         string
	PollInterval int // ms, 0 = global
	Experimental bool
	// Translated is false for applications that ship no translations, so
	// the locale audit has nothing to ask of their markers.
	Translated bool

	// CaptureBottom is how many rows from the bottom classification needs
	// once identity is sticky; 0 means the full screen.
	CaptureBottom int
	// NeedsColour: some load-bearing clause reads colour, so the daemon
	// must capture with -e.
	NeedsColour bool

	Chain    []string       // extends chain, child first
	Resolved map[string]any // merged document, for validate
	Warnings []string
}

// HasKeys reports whether this spec remaps keys at all.
func (s *Spec) HasKeys() bool { return len(s.Keys) > 0 }

// Bucket returns the bucket of a mode; none and unknown map to themselves.
func (s *Spec) Bucket(mode string) string {
	if b, ok := s.Buckets[mode]; ok {
		return b
	}
	return mode
}

// Source is a directory of spec files.
type Source struct {
	FS    fs.FS
	Dir   string // root within FS
	Label string // for messages
}

// Set is every loaded spec, keyed by name.
type Set struct {
	Specs    map[string]*Spec
	Order    []*Spec // matchable specs, highest priority first
	Warnings []string
}

type rawSpec struct {
	name  string
	file  string
	group bool
	doc   map[string]any
}

// Load reads specs from sources in order; a later source shadows an
// earlier one by file name (so pass bundled first, user dirs after).
// A spec that fails to parse or compile is skipped with a warning.
func Load(sources []Source) *Set {
	set := &Set{Specs: map[string]*Spec{}}
	raws := map[string]*rawSpec{} // by file stem
	byName := map[string]string{} // name -> stem

	for _, src := range sources {
		for _, sub := range []string{"", "groups"} {
			dir := path.Join(src.Dir, sub)
			entries, err := fs.ReadDir(src.FS, dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
					continue
				}
				p := path.Join(dir, e.Name())
				label := src.Label + ":" + p
				data, err := fs.ReadFile(src.FS, p)
				if err != nil {
					set.warn("%s: %v", label, err)
					continue
				}
				doc := map[string]any{}
				if _, err := toml.Decode(string(data), &doc); err != nil {
					set.warn("%s: skipped: %v", label, err)
					continue
				}
				stem := strings.TrimSuffix(e.Name(), ".toml")
				name, _ := doc["name"].(string)
				if name == "" {
					name = stem
				}
				group := sub == "groups"
				if g, ok := doc["group"].(bool); ok {
					group = g
				}
				key := sub + "/" + stem
				if old, ok := raws[key]; ok {
					delete(byName, old.name)
				}
				raws[key] = &rawSpec{name: name, file: label, group: group, doc: normalise(doc)}
				byName[name] = key
			}
		}
	}

	byNameRaw := map[string]*rawSpec{}
	for name, key := range byName {
		byNameRaw[name] = raws[key]
	}
	names := make([]string, 0, len(byNameRaw))
	for n := range byNameRaw {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		r := byNameRaw[n]
		merged, chain, err := resolve(r, byNameRaw, nil)
		if err != nil {
			set.warn("%s: skipped: %v", r.file, err)
			continue
		}
		sp, err := compile(r, merged, chain)
		if err != nil {
			set.warn("%s: skipped: %v", r.file, err)
			continue
		}
		set.Specs[sp.Name] = sp
		if !sp.Group {
			set.Order = append(set.Order, sp)
		}
	}
	sort.SliceStable(set.Order, func(i, j int) bool {
		return set.Order[i].Priority > set.Order[j].Priority
	})
	return set
}

func (s *Set) warn(format string, args ...any) {
	s.Warnings = append(s.Warnings, fmt.Sprintf(format, args...))
}

// DirSource returns a Source for a directory on disk (~ expanded).
func DirSource(dir string) Source {
	if strings.HasPrefix(dir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	return Source{FS: os.DirFS(dir), Dir: ".", Label: dir}
}

// FileSource wraps a single spec file (for `validate <file>`) as a source
// that shadows any spec with the same file name.
func FileSource(file string) Source {
	dir, base := filepath.Split(file)
	if dir == "" {
		dir = "."
	}
	return Source{FS: singleFileFS{fs: os.DirFS(dir), name: base}, Dir: ".", Label: filepath.Clean(dir)}
}

type singleFileFS struct {
	fs   fs.FS
	name string
}

func (s singleFileFS) Open(name string) (fs.File, error) {
	if name == "." || name == s.name {
		return s.fs.Open(name)
	}
	return nil, fs.ErrNotExist
}

func (s singleFileFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, fs.ErrNotExist
	}
	entries, err := fs.ReadDir(s.fs, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name() == s.name {
			return []fs.DirEntry{e}, nil
		}
	}
	return nil, fs.ErrNotExist
}

// normalise rewrites the sugar forms into the canonical ones before merge:
// [[insert_when]] becomes [[mode_when]] with mode = "insert" (appended after
// any explicit mode_when entries), and [[match.screen]] entries become
// [[match.clause]] with screen_mode mapped onto combine.
func normalise(doc map[string]any) map[string]any {
	if iw := tables(doc["insert_when"]); len(iw) > 0 {
		mw := tables(doc["mode_when"])
		for _, r := range iw {
			r = deepCopy(r).(map[string]any)
			r["mode"] = "insert"
			mw = append(mw, r)
		}
		doc["mode_when"] = mw
		delete(doc, "insert_when")
	}
	if m, ok := doc["match"].(map[string]any); ok {
		if sc := tables(m["screen"]); len(sc) > 0 {
			m["clause"] = append(tables(m["clause"]), sc...)
			delete(m, "screen")
		}
		if sm, ok := m["screen_mode"]; ok {
			if _, has := m["combine"]; !has {
				m["combine"] = sm
			}
			delete(m, "screen_mode")
		}
	}
	return doc
}

// resolve merges a spec with its extends chain, depth first.
//
// extends is a string or a list. A list is an ordered mixin chain: the
// first entry has the highest precedence after the child itself.
func resolve(r *rawSpec, all map[string]*rawSpec, stack []string) (map[string]any, []string, error) {
	for _, s := range stack {
		if s == r.name {
			return nil, nil, fmt.Errorf("extends cycle: %s -> %s", strings.Join(stack, " -> "), r.name)
		}
	}
	stack = append(stack, r.name)
	doc := deepCopy(r.doc).(map[string]any)
	chain := []string{r.name}
	for _, parentName := range toStrings(r.doc["extends"]) {
		p, ok := all[parentName]
		if !ok {
			return nil, nil, fmt.Errorf("extends unknown spec %q", parentName)
		}
		pdoc, pchain, err := resolve(p, all, stack)
		if err != nil {
			return nil, nil, err
		}
		doc = merge(doc, pdoc)
		chain = append(chain, pchain...)
	}
	delete(doc, "extends")
	return doc, chain, nil
}

// Keys never inherited from a parent.
var notInherited = map[string]bool{"name": true, "group": true, "extends": true}

// merge overlays child on parent: scalars and scalar arrays in the child
// win, tables merge key by key, arrays of tables concatenate child first.
func merge(child, parent map[string]any) map[string]any {
	out := deepCopy(child).(map[string]any)
	for k, pv := range parent {
		if notInherited[k] {
			continue
		}
		cv, ok := out[k]
		if !ok {
			out[k] = deepCopy(pv)
			continue
		}
		cm, cIsMap := cv.(map[string]any)
		pm, pIsMap := pv.(map[string]any)
		if cIsMap && pIsMap {
			out[k] = merge(cm, pm)
			continue
		}
		if ct := tables(cv); ct != nil && isTableArray(cv) && isTableArray(pv) {
			out[k] = append(ct, deepCopy(tables(pv)).([]map[string]any)...)
		}
	}
	return out
}

func isTableArray(v any) bool {
	switch x := v.(type) {
	case []map[string]any:
		return true
	case []any:
		if len(x) == 0 {
			return false
		}
		_, ok := x[0].(map[string]any)
		return ok
	}
	return false
}

var topKeys = map[string]bool{
	"name": true, "priority": true, "extends": true, "modes": true,
	"group": true, "always": true, "match": true, "mode_when": true,
	"corroborate": true, "default_mode": true, "buckets": true,
	"keys": true, "escape": true, "shadowed": true, "hook": true,
	"poll_interval": true, "experimental": true, "description": true,
	"translated": true,
}

func compile(r *rawSpec, doc map[string]any, chain []string) (*Spec, error) {
	sp := &Spec{
		Name:     r.name,
		File:     r.file,
		Group:    r.group,
		Buckets:  map[string]string{},
		Shadowed: map[string]string{},
		Chain:    chain,
		Resolved: doc,
	}
	for k := range doc {
		if !topKeys[k] {
			sp.warnf("unknown key %q (typo?)", k)
		}
	}
	if v, ok := doc["priority"]; ok {
		n, ok := toInt(v)
		if !ok {
			return nil, fmt.Errorf("priority must be an integer")
		}
		sp.Priority = n
	}
	sp.Experimental, _ = doc["experimental"].(bool)
	sp.Translated = true
	if tr, ok := doc["translated"].(bool); ok {
		sp.Translated = tr
	}
	sp.Modes = toStrings(doc["modes"])
	sp.Always, _ = doc["always"].(string)
	if sp.Always != "" && !contains(sp.Modes, sp.Always) {
		sp.Modes = append(sp.Modes, sp.Always)
	}
	for _, m := range sp.Modes {
		if m == ModeNone || m == ModeUnknown {
			return nil, fmt.Errorf("mode %q is reserved and cannot be declared", m)
		}
	}

	// Buckets: every declared mode must be assigned exactly once.
	if b, ok := doc["buckets"].(map[string]any); ok {
		for bucket, modes := range b {
			if bucket != BucketTyping && bucket != BucketCommanding {
				return nil, fmt.Errorf("unknown bucket %q (want typing or commanding)", bucket)
			}
			for _, m := range toStrings(modes) {
				if prev, dup := sp.Buckets[m]; dup && prev != bucket {
					return nil, fmt.Errorf("mode %q assigned to both buckets", m)
				}
				sp.Buckets[m] = bucket
			}
		}
	}
	if !sp.Group {
		for _, m := range sp.Modes {
			if _, ok := sp.Buckets[m]; !ok {
				return nil, fmt.Errorf("mode %q has no bucket assignment ([buckets] typing/commanding)", m)
			}
		}
		if len(sp.Modes) == 0 {
			return nil, fmt.Errorf("no modes declared")
		}
	}

	// Identity.
	if m, ok := doc["match"].(map[string]any); ok {
		sp.Identity.Commands = toStrings(m["command"])
		if t, ok := m["title"].(string); ok && t != "" {
			re, err := regexp.Compile(t)
			if err != nil {
				return nil, fmt.Errorf("match.title: %w", err)
			}
			sp.Identity.Title = re
		}
		var err error
		if sp.Identity.RequiresAlt, err = getBool(m, "requires_alt_screen", false); err != nil {
			return nil, fmt.Errorf("match: %w", err)
		}
		rule, err := compileRule(m, "match", "")
		if err != nil {
			return nil, err
		}
		if len(rule.Clauses) > 0 {
			sp.Identity.Screen = rule
		}
	}
	if !sp.Group && len(sp.Identity.Commands) == 0 && sp.Identity.Title == nil && sp.Identity.Screen == nil {
		return nil, fmt.Errorf("no identity: [match] needs command, title or screen clauses")
	}

	// Mode rules, in order.
	for i, m := range tables(doc["mode_when"]) {
		mode, _ := m["mode"].(string)
		name, _ := m["name"].(string)
		label := fmt.Sprintf("mode_when[%d]", i)
		if name != "" {
			label += " " + name
		}
		if mode == "" {
			return nil, fmt.Errorf("%s: missing mode", label)
		}
		if !sp.Group && !contains(sp.Modes, mode) {
			return nil, fmt.Errorf("%s: mode %q is not declared in modes", label, mode)
		}
		rule, err := compileRule(m, label, mode)
		if err != nil {
			return nil, err
		}
		if len(rule.Clauses) == 0 {
			return nil, fmt.Errorf("%s: rule has no clauses", label)
		}
		rule.Name = name
		sp.ModeRules = append(sp.ModeRules, rule)
	}

	// Corroboration.
	for i, m := range tables(doc["corroborate"]) {
		label := fmt.Sprintf("corroborate[%d]", i)
		m = deepCopy(m).(map[string]any)
		if s, ok := m["if_cursor_shape"]; ok {
			m["cursor_shape"] = s
			delete(m, "if_cursor_shape")
		}
		rule, err := compileRule(m, label, "")
		if err != nil {
			return nil, err
		}
		c := &Corroboration{Rule: *rule, VetoModes: toStrings(m["veto_modes"]), Confidence: "low"}
		c.ThenMode, _ = m["then_mode"].(string)
		if conf, ok := m["confidence"].(string); ok {
			c.Confidence = conf
		}
		if c.ThenMode == "" || (!sp.Group && !contains(sp.Modes, c.ThenMode)) {
			return nil, fmt.Errorf("%s: then_mode must name a declared mode", label)
		}
		sp.Corroborate = append(sp.Corroborate, c)
	}

	// Default mode: an affirmative conclusion that always requires the
	// identity to be re-confirmed on the same capture (§4.4).
	sp.OtherwiseMode = ModeUnknown
	if contains(sp.Modes, "normal") {
		sp.DefaultMode = "normal"
	}
	if dm, ok := doc["default_mode"].(map[string]any); ok {
		if m, ok := dm["mode"].(string); ok {
			sp.DefaultMode = m
		}
		if o, ok := dm["otherwise"].(string); ok {
			sp.OtherwiseMode = o
		}
		if req, ok := dm["requires_identity_confirmed"].(bool); ok && !req {
			sp.warnf("default_mode.requires_identity_confirmed = false is ignored: " +
				"concluding a mode from the absence of a marker always requires confirmed identity")
		}
	}
	if !sp.Group && sp.Always == "" {
		if sp.DefaultMode == "" {
			return nil, fmt.Errorf("no default mode: declare \"normal\" or set [default_mode] mode")
		}
		if !contains(sp.Modes, sp.DefaultMode) {
			return nil, fmt.Errorf("default mode %q is not declared in modes", sp.DefaultMode)
		}
	}
	if sp.OtherwiseMode != ModeUnknown && !contains(sp.Modes, sp.OtherwiseMode) {
		return nil, fmt.Errorf("default_mode.otherwise %q is not declared", sp.OtherwiseMode)
	}

	// Keys.
	if km, ok := doc["keys"].(map[string]any); ok {
		names := make([]string, 0, len(km))
		for k := range km {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			switch v := km[k].(type) {
			case bool:
				if v {
					return nil, fmt.Errorf("keys.%s: true is meaningless; use a key name or false", k)
				}
				// false suppresses an inherited binding
			case string:
				if v == "" {
					return nil, fmt.Errorf("keys.%s: empty target", k)
				}
				sp.Keys = append(sp.Keys, KeyBinding{Key: k, Send: v})
			default:
				return nil, fmt.Errorf("keys.%s: want a key name or false", k)
			}
		}
	}
	if e, ok := doc["escape"].(map[string]any); ok {
		sp.EscapeLeader, _ = e["leader"].(string)
	}
	if sh, ok := doc["shadowed"].(map[string]any); ok {
		for k, v := range sh {
			s, _ := v.(string)
			sp.Shadowed[k] = s
		}
	}
	sp.Hook, _ = doc["hook"].(string)
	if v, ok := doc["poll_interval"]; ok {
		n, ok := toInt(v)
		if !ok || n < 0 {
			return nil, fmt.Errorf("poll_interval must be a non-negative integer (ms)")
		}
		sp.PollInterval = n
	}
	if sp.HasKeys() {
		if !contains(sp.Modes, "normal") && !sp.Group {
			sp.warnf("has [keys] but no normal mode; keys apply to every commanding mode")
		}
		if _, ok := sp.Buckets[sp.Always]; ok && sp.Buckets[sp.Always] == BucketTyping {
			sp.warnf("always = %q is a typing mode, so [keys] will never apply", sp.Always)
		}
	}

	sp.computeCapture()
	sp.lint()
	return sp, nil
}

// compileRule builds a rule from a table holding explicit [[clause]]
// entries and/or inline matcher keys (the single-rule sugar form).
func compileRule(m map[string]any, label, mode string) (*Rule, error) {
	r := &Rule{Mode: mode}
	for i, cm := range tables(m["clause"]) {
		for k := range cm {
			if !clauseKeys[k] {
				return nil, fmt.Errorf("%s.clause[%d]: unknown key %q", label, i, k)
			}
		}
		kinds := 0
		kind := KindRegex
		if hasAny(cm, regexKeys...) {
			kinds++
		}
		if hasAny(cm, cursorKeys...) {
			kinds++
			kind = KindCursor
		}
		if hasAny(cm, colorKeys...) {
			kinds++
			kind = KindColor
		}
		if kinds != 1 {
			return nil, fmt.Errorf("%s.clause[%d]: a clause needs exactly one matcher (regex, cursor_* or fg/bg/attrs)", label, i)
		}
		c, err := compileClause(cm, kind, fmt.Sprintf("%s.clause[%d]", label, i), WeightNumeric)
		if err != nil {
			return nil, err
		}
		r.Clauses = append(r.Clauses, c)
	}
	// Inline form: the rule's own regex and cursor keys become clauses
	// that must all hold.
	inline := 0
	if hasAny(m, regexKeys...) {
		c, err := compileClause(m, KindRegex, label+" (regex)", WeightNumeric)
		if err != nil {
			return nil, err
		}
		r.Clauses = append(r.Clauses, c)
		inline++
	}
	if hasAny(m, cursorKeys...) {
		cm := map[string]any{}
		for _, k := range cursorKeys {
			if v, ok := m[k]; ok {
				cm[k] = v
			}
		}
		c, err := compileClause(cm, KindCursor, label+" (cursor)", WeightNumeric)
		if err != nil {
			return nil, err
		}
		r.Clauses = append(r.Clauses, c)
		inline++
	}
	// Colour sub-tables default to bonus weight.
	for i, cm := range tables(m["color"]) {
		c, err := compileClause(cm, KindColor, fmt.Sprintf("%s.color[%d]", label, i), WeightBonus)
		if err != nil {
			return nil, err
		}
		r.Clauses = append(r.Clauses, c)
	}

	r.Combine, _ = m["combine"].(string)
	var sum, min float64 = 0, -1
	for _, c := range r.Clauses {
		if c.WeightKind == WeightNumeric {
			sum += c.Weight
			if min < 0 || c.Weight < min {
				min = c.Weight
			}
		}
	}
	if min < 0 {
		min = 0
	}
	if t, ok := m["threshold"]; ok {
		f, ok := toFloat(t)
		if !ok {
			return nil, fmt.Errorf("%s: bad threshold", label)
		}
		r.Threshold = f
		if r.Combine == "" {
			r.Combine = "weighted"
		}
	} else {
		switch r.Combine {
		case "", "all":
			r.Combine = "all"
			r.Threshold = sum
		case "any":
			r.Threshold = min
		case "weighted":
			r.Threshold = 100
		default:
			return nil, fmt.Errorf("%s: combine must be all, any or weighted", label)
		}
	}

	r.Tier = 1
	for _, c := range r.Clauses {
		if c.Tier() > r.Tier {
			r.Tier = c.Tier()
		}
		if c.Kind == KindColor {
			r.Colour = true
		}
	}
	return r, nil
}

// computeCapture works out how much of the screen classification needs
// once identity is sticky: the bottom N rows when every row-addressed
// clause counts from the bottom, otherwise the full screen.
func (sp *Spec) computeCapture() {
	bottom := 0
	full := false
	visit := func(r *Rule) {
		if r == nil {
			return
		}
		for _, c := range r.Clauses {
			// Bonus colour clauses cannot change a result, so they never
			// justify the cost of a -e capture on the hot path; validate
			// still evaluates them.
			if c.Kind == KindColor && c.WeightKind != WeightBonus {
				sp.NeedsColour = true
			}
			var top int
			switch c.Kind {
			case KindRegex:
				top = c.RowA
			case KindColor:
				top = c.Row
			default:
				continue
			}
			if top >= 0 {
				full = true
			} else if -top > bottom {
				bottom = -top
			}
		}
	}
	visit(sp.Identity.Screen)
	for _, r := range sp.ModeRules {
		visit(r)
	}
	for _, c := range sp.Corroborate {
		visit(&c.Rule)
	}
	if !full {
		sp.CaptureBottom = bottom
	}
}

func (sp *Spec) warnf(format string, args ...any) {
	sp.Warnings = append(sp.Warnings, fmt.Sprintf(format, args...))
}

func contains(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}
