package tmux

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GrewHair/tmux-modal/internal/screen"
)

// PaneInfo is the tier-1 snapshot of one pane: O(1) format reads only.
type PaneInfo struct {
	ID            string
	SessionID     string
	WindowID      string
	WindowActive  bool // window is its session's current window
	PaneActive    bool // pane is its window's active pane
	SessionHumans int  // attached clients (including ours)
	Alt           bool
	CursorVisible bool
	CursorX       int
	CursorY       int
	CursorShape   string
	ScrollLower   int
	Width         int
	Height        int
	HistorySize   int
	InMode        bool
	Dead          bool
	Command       string
	Enabled       string // @modal_enabled, resolved pane > window > global
	KeyTable      string // session key-table option
	Title         string
}

// Focused is true for the pane that receives the session's keystrokes.
func (p *PaneInfo) Focused() bool { return p.WindowActive && p.PaneActive }

// paneFormat is evaluated once per pane by a single list-panes -a; fields
// are tab-separated and the title, which may contain anything, is last.
var paneFields = []string{
	"pane_id", "session_id", "window_id", "window_active", "pane_active",
	"session_attached", "alternate_on", "cursor_flag", "cursor_x", "cursor_y",
	"cursor_shape", "scroll_region_lower", "pane_width", "pane_height",
	"history_size", "pane_in_mode", "pane_dead", "pane_current_command",
	"@modal_enabled", "key-table", "pane_title",
}

// PaneFormat is the packed tier-1 format string.
var PaneFormat = func() string {
	parts := make([]string, len(paneFields))
	for i, f := range paneFields {
		parts[i] = "#{" + f + "}"
	}
	return strings.Join(parts, "\t")
}()

// ListPanes reads the tier-1 snapshot of every pane in one command.
func ListPanes(r Runner) ([]PaneInfo, error) {
	lines, err := r.Run("list-panes", "-a", "-F", PaneFormat)
	if err != nil {
		return nil, err
	}
	out := make([]PaneInfo, 0, len(lines))
	for _, l := range lines {
		p, err := parsePaneLine(l)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// PaneInfoOf reads the tier-1 snapshot of one pane.
func PaneInfoOf(r Runner, target string) (PaneInfo, error) {
	lines, err := r.Run("display-message", "-p", "-t", target, PaneFormat)
	if err != nil {
		return PaneInfo{}, err
	}
	if len(lines) == 0 {
		return PaneInfo{}, fmt.Errorf("no such pane %s", target)
	}
	return parsePaneLine(lines[0])
}

func parsePaneLine(l string) (PaneInfo, error) {
	f := strings.SplitN(l, "\t", len(paneFields))
	if len(f) != len(paneFields) {
		return PaneInfo{}, fmt.Errorf("short pane line")
	}
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	return PaneInfo{
		ID: f[0], SessionID: f[1], WindowID: f[2],
		WindowActive: f[3] == "1", PaneActive: f[4] == "1",
		SessionHumans: atoi(f[5]),
		Alt:           f[6] == "1", CursorVisible: f[7] == "1",
		CursorX: atoi(f[8]), CursorY: atoi(f[9]), CursorShape: f[10],
		ScrollLower: atoi(f[11]), Width: atoi(f[12]), Height: atoi(f[13]),
		HistorySize: atoi(f[14]), InMode: f[15] == "1", Dead: f[16] == "1",
		Command: f[17], Enabled: f[18], KeyTable: f[19], Title: f[20],
	}, nil
}

// Screen returns a Screen skeleton carrying the tier-1 state, with no
// captured text yet.
func (p *PaneInfo) Screen() *screen.Screen {
	return &screen.Screen{
		Width: p.Width, Height: p.Height,
		Cursor: screen.Cursor{X: p.CursorX, Y: p.CursorY, Visible: p.CursorVisible,
			Shape: normaliseShape(p.CursorShape)},
		AltScreen: p.Alt, Command: p.Command, Title: p.Title,
	}
}

func normaliseShape(s string) string {
	switch s {
	case "block", "underline", "bar", "default":
		return s
	}
	return ""
}

// CaptureArgs builds a capture-pane command for the bottom `bottom` rows
// (0 = the whole visible screen), with SGR sequences when colour is set.
func CaptureArgs(pane string, height, bottom int, colour bool) []string {
	args := []string{"capture-pane", "-p", "-t", pane}
	if colour {
		args = append(args, "-e")
	}
	if bottom > 0 && bottom < height {
		args = append(args, "-S", strconv.Itoa(height-bottom), "-E", strconv.Itoa(height-1))
	}
	return args
}

// Fill completes a Screen from a capture made with CaptureArgs.
func Fill(s *screen.Screen, lines []string, bottom int, colour bool) {
	top := 0
	if bottom > 0 && bottom < s.Height {
		top = s.Height - bottom
	}
	s.Top = top
	text := strings.Join(lines, "\n")
	if colour {
		s.Cells = screen.ParseANSI(text, s.Height-top)
		s.Lines = make([]string, len(s.Cells))
		for i, row := range s.Cells {
			var b strings.Builder
			for _, c := range row {
				if c.Ch != 0 {
					b.WriteRune(c.Ch)
				}
			}
			s.Lines[i] = strings.TrimRight(b.String(), " \t")
		}
		return
	}
	full := screen.FromText(text, s.Width, s.Height-top)
	s.Lines = full.Lines
}

// Capture captures a pane into a complete Screen (tier 1 + tier 2) using
// separate commands; one-shot CLI helper.
func Capture(r Runner, target string, colour bool) (*screen.Screen, []string, []string, error) {
	info, err := PaneInfoOf(r, target)
	if err != nil {
		return nil, nil, nil, err
	}
	s := info.Screen()
	plain, err := r.Run(CaptureArgs(info.ID, info.Height, 0, false)...)
	if err != nil {
		return nil, nil, nil, err
	}
	Fill(s, plain, 0, false)
	var ansi []string
	if colour {
		ansi, err = r.Run(CaptureArgs(info.ID, info.Height, 0, true)...)
		if err != nil {
			return nil, nil, nil, err
		}
		s.Cells = screen.ParseANSI(strings.Join(ansi, "\n"), s.Height)
	}
	return s, plain, ansi, nil
}
