package classify

import (
	"path/filepath"
	"regexp"

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
	// Split is true when inner pane borders divide the screen, so the
	// captured grid mixes several inner panes.
	Split bool
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
	n := Nested{StatusRow: -1}
	cmd := filepath.Base(s.Command)
	if Multiplexers[cmd] {
		n.Evidence = "command"
	} else if !s.AltScreen || commandClaimed(set, cmd) {
		// An inner multiplexer always holds the alternate screen.
		return n
	}
	for _, r := range []int{s.Height - 1, 0} {
		if l, ok := s.Line(r); ok && statusRE.MatchString(l) {
			n.StatusRow = r
			if n.Evidence == "" {
				n.Evidence = "status-line"
			}
			break
		}
	}
	if s.Top == 0 && len(s.Lines) == s.Height && hasBorders(s, n.StatusRow) {
		n.Split = true
		if n.Evidence == "" {
			n.Evidence = "borders"
		}
	}
	if n.Evidence == "" && titleRE.MatchString(s.Title) {
		n.Evidence = "title"
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
func hasBorders(s *screen.Screen, statusRow int) bool {
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
			row[c] = ch
			if hBorder[ch] || vBorder[ch] {
				hcount++
			}
			c += screen.RuneWidth(ch)
		}
		if r > first && r < last && c == s.Width && hcount == s.Width && hPlain[row[0]] && hPlain[row[s.Width-1]] {
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
