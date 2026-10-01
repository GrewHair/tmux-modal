package classify

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GrewHair/tmux-modal/internal/screen"
	"github.com/GrewHair/tmux-modal/internal/spec"
)

// Nested is the evidence that a pane shows another terminal multiplexer
// (typically tmux on a remote host reached over ssh). The inner
// multiplexer is a terminal emulator of its own: it swallows the
// application's alternate-screen and cursor state, may draw a status line
// and pane borders over the application, and interprets its own prefix
// key. Key remapping is therefore never done in a nested pane (§3.8).
type Nested struct {
	// Evidence is "" when the pane does not look nested, otherwise the
	// strongest signal: "command", "status-line", "borders" or "title".
	Evidence string
	// StatusRow is the full-screen row of the inner status line, or -1.
	StatusRow int
	// Window is the current window's name on that status line ("" when
	// there is none or it can't be read): by default, the command of its
	// active pane (D42).
	Window string
	// Split is true when inner pane borders divide the screen, so the
	// captured grid mixes several inner panes.
	Split bool
	// Focus is the inner pane that has the keyboard, in the coordinates of
	// the screen without the inner status line; nil when Split and the
	// screen does not show which one it is.
	Focus *Rect
	// FocusBy is what showed the focus: "border colour" or "cursor".
	FocusBy string
	// Panes is the number of inner panes when Split.
	Panes int
	// Kind is the inner multiplexer: the local command's name when that
	// gave it away (tmux, tmate, screen, zellij, byobu), otherwise "tmux":
	// the status line, borders and title recognised are tmux's.
	Kind string
	// Via is the remote transport the pane's command is (ssh, docker,
	// kubectl, ...; Transports), or "".
	Via string
	// Off: the screen showed an inner multiplexer, but the pane's command
	// is neither a transport nor a multiplexer, so it cannot be one (tmux
	// does not nest locally unless told to, and a local client's command
	// is tmux itself). Reported, not acted on (D36).
	Off bool
}

// Transports are the commands that reach another machine or container,
// where a nested multiplexer can run. @modal_transports adds more.
var Transports = map[string]bool{
	"ssh": true, "autossh": true, "mosh-client": true, "et": true, "telnet": true,
	"docker": true, "podman": true, "nerdctl": true, "kubectl": true, "oc": true,
	"lxc": true, "incus": true, "multipass": true, "machinectl": true,
	"distrobox": true, "toolbox": true, "session-manager-plugin": true,
}

// Rect is an area of the screen: an inner pane.
type Rect struct{ X, Y, W, H int }

func (r Rect) contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Multiplexers are local commands that mean the pane shows another
// multiplexer's rendering.
var Multiplexers = map[string]bool{
	"tmux": true, "tmate": true, "screen": true, "zellij": true, "byobu": true,
}

var (
	// tmux's default status-left and window list: "[session] 0:name*".
	statusRE = regexp.MustCompile(`^\[[^\]]{1,64}\] [0-9]+:\S`)
	// tmux's default set-titles-string, `#S:#I:#W - "#T" #{session_alerts}`.
	// set-titles is off by default, so this is supporting evidence only.
	titleRE = regexp.MustCompile(`^[^:]+:[0-9]+:.* - ".*"`)
)

// Border characters tmux draws between panes (pane-border-lines single,
// heavy and double), including the junctions where borders meet. An inner
// tmux without a UTF-8 locale draws them with the VT100 line-drawing
// charset instead, which the outer grid stores as the plain letters
// x (vertical), q (horizontal) and t u w v n (junctions).
var (
	vBorder = runeSet("│┃║├┤┼┣┫╋╠╣╬" + "xtun")
	hBorder = runeSet("─━═┬┴┼┳┻╋╦╩╬" + "qwvn")
	// tmux draws no border along the window edge, so a border spanning
	// the window ends in plain lines there, never in a junction. A TUI's
	// own full-width separator (mc's ├────┤├────┤) does not.
	vPlain = runeSet("│┃║x")
	hPlain = runeSet("─━═q")
)

func runeSet(s string) map[rune]bool {
	m := map[rune]bool{}
	for _, r := range s {
		m[r] = true
	}
	return m
}

// DetectNested looks for an inner multiplexer. The screen checks run only
// when no spec claims the pane's command: a local htop is never nested,
// while ssh, mosh, docker and friends may be showing anything. All checks
// are cheap: two regexes on two rows, and one pass over the grid for
// borders, which needs a full-screen capture.
func DetectNested(set *spec.Set, s *screen.Screen) Nested {
	return DetectNestedWith(set, s, nil)
}

// DetectNestedWith is DetectNested with extra transport commands.
func DetectNestedWith(set *spec.Set, s *screen.Screen, extra []string) Nested {
	n := Nested{StatusRow: -1}
	cmd := filepath.Base(s.Command)
	if Transports[cmd] || contains(extra, cmd) {
		n.Via = cmd
	}
	if Multiplexers[cmd] {
		n.Evidence = "command"
	} else if !s.AltScreen || commandClaimed(set, cmd) {
		// An inner multiplexer always holds the alternate screen.
		return n
	}
	for _, r := range []int{s.Height - 1, 0} {
		if l, ok := s.Line(r); ok && statusRE.MatchString(l) {
			n.StatusRow = r
			n.Window = windowName(l)
			if n.Evidence == "" {
				n.Evidence = "status-line"
			}
			break
		}
	}
	// Borders as the only evidence must also be in tmux's default border
	// colours: a TUI's own full-width rule (Claude Code's grey one under
	// its prompt) is otherwise indistinguishable from a split.
	strict := n.Evidence == ""
	if s.Top == 0 && len(s.Lines) == s.Height && hasBorders(s, n.StatusRow, strict) {
		n.Split = true
		if n.Evidence == "" {
			n.Evidence = "borders"
		}
		n.Focus, n.FocusBy, n.Panes = focusedPane(s.WithoutRow(n.StatusRow))
	}
	if n.Evidence == "" && titleRE.MatchString(s.Title) {
		n.Evidence = "title"
	}
	switch {
	case n.Evidence == "command":
		n.Kind = cmd
	case n.Evidence != "":
		n.Kind = "tmux"
		n.Off = n.Via == ""
	}
	return n
}

func commandClaimed(set *spec.Set, cmd string) bool {
	if set == nil {
		return false
	}
	for _, sp := range set.Order {
		if sp.MatchesCommand(cmd) {
			return true
		}
	}
	return false
}

// hasBorders reports a tmux pane border: an interior column made only of
// vertical border characters on every row, or an interior row made only
// of horizontal ones across the full width. tmux's top-level split always
// spans the whole window, so any split layout has one. Box-drawing TUIs
// close their frames with corners, which are not in either set.
//
// VT100 line-drawing borders arrive as plain letters, and ordinary text
// can line letters up too ("Sample text line 1", "Sample text line 2" puts
// an x and a t in the same columns on every row). A column that contains
// any such letter must also look like a drawn border: junctions (t, u, n)
// only where a horizontal border (q) meets them, and blank cells beside
// it on at least half of the rows, which words never have.
//
// Either way the border must end in a plain line at the window edges.
//
// With defaultColours (and a colour capture), a border cell must also be in
// tmux's default border colours: default or green foreground (the active
// border) on the default background (F46).
func hasBorders(s *screen.Screen, statusRow int, defaultColours bool) bool {
	if s.Width < 3 || s.Height < 3 {
		return false
	}
	first, last := 0, s.Height-1 // content rows, without the status line
	if statusRow == first {
		first++
	} else if statusRow == last {
		last--
	}
	var grid [][]rune // content rows, one rune per column (wide: rune, then 0)
	for r, line := range s.Lines {
		if r == statusRow {
			continue
		}
		row := make([]rune, s.Width)
		c, hcount := 0, 0
		for _, ch := range line {
			if c >= s.Width {
				break
			}
			if (hBorder[ch] || vBorder[ch]) && defaultColours && !tmuxBorderColour(s, r, c) {
				ch = ' '
			}
			row[c] = ch
			if hBorder[ch] || vBorder[ch] {
				hcount++
			}
			c += screen.RuneWidth(ch)
		}
		if r > first && r < last && c == s.Width && hcount == s.Width && borderRow(row) {
			return true
		}
		grid = append(grid, row)
	}
	if len(grid) < 2 {
		return false
	}
	for c := 1; c < s.Width-1; c++ {
		if borderColumn(grid, c) {
			return true
		}
	}
	return false
}

// tmuxBorderColour reports whether a cell has one of tmux's default border
// colours; true when the capture has no colour.
func tmuxBorderColour(s *screen.Screen, r, c int) bool {
	cell, ok := s.Cell(r, c)
	if !ok || s.Cells == nil {
		return true
	}
	fg := cell.Fg.Kind == screen.ColorDefault || (cell.Fg.Kind == screen.ColorIndexed && cell.Fg.Index == 2)
	return fg && cell.Bg.Kind == screen.ColorDefault
}

// borderRow reports a horizontal border across a whole area: border
// characters only, plain lines at both ends.
func borderRow(row []rune) bool {
	if len(row) < 2 || !hPlain[row[0]] || !hPlain[row[len(row)-1]] {
		return false
	}
	for _, ch := range row {
		if !hBorder[ch] && !vBorder[ch] {
			return false
		}
	}
	return true
}

// focusedPane finds the inner pane that has the keyboard, and says how.
//
// The border colour first. tmux's default pane-active-border-style is
// green and pane-border-style the default colour: every border cell beside
// the active pane is green and every other one is not (F44; the same on
// tmux 3.0a to 3.7c). With exactly two panes tmux colours only the half of
// the shared border on the active pane's side: the top or left half for
// the first pane, the bottom or right half for the second. Any other
// colour on a border means a theme, and the colour then says nothing.
//
// Then the cursor: tmux draws the terminal cursor only in the active pane,
// and only while that pane shows it, so a visible cursor is inside the
// focused pane. Needed when the capture has no colour.
//
// If the two disagree, or neither says anything (no colour and a hidden
// cursor, or the cursor on the inner command prompt, removed from s),
// nothing is guessed.
func focusedPane(s *screen.Screen) (*Rect, string, int) {
	grid := runeGrid(s)
	panes := innerPanes(grid, s.Width)
	byColour := colourFocus(s, grid, panes)
	var byCursor *Rect
	if s.Cursor.Visible {
		for i := range panes {
			if panes[i].contains(s.Cursor.X, s.Cursor.Y) {
				byCursor = &panes[i]
			}
		}
	}
	switch {
	case byColour != nil && byCursor != nil && *byColour != *byCursor:
		return nil, "", len(panes)
	case byColour != nil:
		return byColour, "border colour", len(panes)
	case byCursor != nil:
		return byCursor, "cursor", len(panes)
	}
	return nil, "", len(panes)
}

type cell struct{ x, y int }

// colourFocus reads the active pane from the border colours (see
// focusedPane); nil when the capture has no colour or the colours are not
// tmux's defaults.
func colourFocus(s *screen.Screen, grid [][]rune, panes []Rect) *Rect {
	if s.Cells == nil || len(panes) < 2 {
		return nil
	}
	// The border cells along each pane's sides, corners excluded (a
	// junction touches several panes).
	sides := make([][]cell, len(panes))
	green := map[cell]bool{}
	for i, p := range panes {
		add := func(x, y int) {
			if y < 0 || y >= len(grid) || x < 0 || x >= s.Width {
				return
			}
			if r := grid[y][x]; !vBorder[r] && !hBorder[r] {
				return
			}
			sides[i] = append(sides[i], cell{x, y})
		}
		for y := p.Y; y < p.Y+p.H; y++ {
			add(p.X-1, y)
			add(p.X+p.W, y)
		}
		for x := p.X; x < p.X+p.W; x++ {
			add(x, p.Y-1)
			add(x, p.Y+p.H)
		}
	}
	for _, side := range sides {
		for _, c := range side {
			if _, seen := green[c]; seen {
				continue
			}
			sc, ok := s.Cell(c.y, c.x)
			if !ok || sc.Bg.Kind != screen.ColorDefault {
				return nil
			}
			switch {
			case sc.Fg.Kind == screen.ColorIndexed && sc.Fg.Index == 2:
				green[c] = true
			case sc.Fg.Kind == screen.ColorDefault:
				green[c] = false
			default:
				return nil
			}
		}
	}
	ngreen := 0
	for _, g := range green {
		if g {
			ngreen++
		}
	}
	if ngreen == 0 {
		return nil
	}
	// The active pane: its sides are exactly the green cells.
	var found *Rect
	for i := range panes {
		all := len(sides[i]) == ngreen
		for _, c := range sides[i] {
			all = all && green[c]
		}
		if all {
			if found != nil {
				return nil
			}
			found = &panes[i]
		}
	}
	if found != nil || len(panes) != 2 {
		return found
	}
	// Two panes: half of the one border between them. sides lists it top
	// to bottom or left to right.
	shared := sides[0]
	n := 0
	for n < len(shared) && green[shared[n]] {
		n++
	}
	if n > 0 && n < len(shared) && n == ngreen {
		return &panes[0]
	}
	n = 0
	for n < len(shared) && !green[shared[n]] {
		n++
	}
	if n > 0 && n < len(shared) && len(shared)-n == ngreen {
		return &panes[1]
	}
	return nil
}

// runeGrid is a full-screen grid, one rune per column (a wide character's
// second column is 0), padded with blanks.
func runeGrid(s *screen.Screen) [][]rune {
	grid := make([][]rune, len(s.Lines))
	for r, line := range s.Lines {
		row := make([]rune, s.Width)
		c := 0
		for _, ch := range line {
			if c >= s.Width {
				break
			}
			row[c] = ch
			c += screen.RuneWidth(ch)
		}
		for ; c < s.Width; c++ {
			row[c] = ' '
		}
		grid[r] = row
	}
	return grid
}

// innerPanes cuts a full-screen grid without the inner status line into
// the inner panes the way tmux lays them out: every split spans the whole
// of the area it divides (the window first, then each part), so a border
// running across an area cuts it in two and each part is cut again. The
// first part of each cut (left or top) comes first.
func innerPanes(grid [][]rune, width int) []Rect {
	var out []Rect
	var cut func(a Rect, depth int)
	cut = func(a Rect, depth int) {
		if depth < 8 && a.W >= 3 && a.H >= 3 {
			for y := a.Y + 1; y < a.Y+a.H-1; y++ {
				if borderRow(grid[y][a.X : a.X+a.W]) {
					cut(Rect{a.X, a.Y, a.W, y - a.Y}, depth+1)
					cut(Rect{a.X, y + 1, a.W, a.Y + a.H - y - 1}, depth+1)
					return
				}
			}
			area := make([][]rune, a.H)
			for i := range area {
				area[i] = grid[a.Y+i][a.X : a.X+a.W]
			}
			for x := 1; x < a.W-1; x++ {
				if borderColumn(area, x) {
					cut(Rect{a.X, a.Y, x, a.H}, depth+1)
					cut(Rect{a.X + x + 1, a.Y, a.W - x - 1, a.H}, depth+1)
					return
				}
			}
		}
		out = append(out, a)
	}
	cut(Rect{0, 0, width, len(grid)}, 0)
	return out
}

// acsLetter reports the VT100 line-drawing letters that double as text.
func acsLetter(r rune) bool { return r >= 'a' && r <= 'z' }

func borderColumn(grid [][]rune, c int) bool {
	if !vPlain[grid[0][c]] || !vPlain[grid[len(grid)-1][c]] {
		return false
	}
	letters, blankSide := false, 0
	for _, row := range grid {
		ch := row[c]
		if !vBorder[ch] {
			return false
		}
		left, right := row[c-1], row[c+1]
		if left == 0 || left == ' ' || right == 0 || right == ' ' {
			blankSide++
		}
		if !acsLetter(ch) {
			continue
		}
		letters = true
		switch ch {
		case 't': // ├
			if right != 'q' {
				return false
			}
		case 'u': // ┤
			if left != 'q' {
				return false
			}
		case 'n': // ┼
			if left != 'q' || right != 'q' {
				return false
			}
		}
	}
	return !letters || 2*blankSide >= len(grid)
}

// windowEntryRE is one window in tmux's default window list: index, name,
// flags (current *, last -, activity #, bell !, silence ~, marked M,
// zoomed Z).
var windowEntryRE = regexp.MustCompile(`^[0-9]+:(.+?)([-*#!~MZ]*)$`)

// windowName reads the current window's name from a status line in tmux's
// default format ("[0] 0:bash- 1:htop*  ..."): the one entry flagged *.
// Names from automatic-rename have no spaces; a renamed window's might,
// and is then simply not found.
func windowName(line string) string {
	name, found := "", 0
	for _, f := range strings.Fields(line) {
		m := windowEntryRE.FindStringSubmatch(f)
		if m != nil && strings.Contains(m[2], "*") {
			name = m[1]
			found++
		}
	}
	if found != 1 {
		return ""
	}
	return name
}
