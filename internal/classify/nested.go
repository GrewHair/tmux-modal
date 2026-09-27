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
		for _, c := range sp.Identity.Commands {
			if c == cmd {
				return true
			}
		}
	}
	return false
}

// hasBorders reports a tmux pane border: an interior column made only of
// vertical border characters on every row, or an interior row made only
// of horizontal ones across the full width. tmux's top-level split always
// spans the whole window, so any split layout has one. Box-drawing TUIs
// close their frames with corners, which are not in either set.
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
	var vcols []bool // columns still made of vertical border characters
	rows := 0
	for r, line := range s.Lines {
		if r == statusRow {
			continue
		}
		cols := make([]bool, s.Width)
		c, hcount := 0, 0
		for _, ch := range line {
			if c >= s.Width {
				break
			}
			if vBorder[ch] {
				cols[c] = true
			}
			if hBorder[ch] || vBorder[ch] {
				hcount++
			}
			c += screen.RuneWidth(ch)
		}
		if r > first && r < last && c == s.Width && hcount == s.Width && hasHorizontal(line) {
			return true
		}
		if vcols == nil {
			vcols = cols
		} else {
			for i := range vcols {
				vcols[i] = vcols[i] && cols[i]
			}
		}
		rows++
	}
	if rows < 2 {
		return false
	}
	for i := 1; i < len(vcols)-1; i++ {
		if vcols[i] {
			return true
		}
	}
	return false
}

func hasHorizontal(line string) bool {
	for _, ch := range line {
		if hBorder[ch] && !vBorder[ch] {
			return true
		}
	}
	return false
}
