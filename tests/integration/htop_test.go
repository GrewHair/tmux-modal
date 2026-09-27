package integration

import (
	"strings"
	"testing"
)

func requireHtop(t *testing.T) {
	t.Helper()
	if !haveHtop {
		t.Skip("htop not installed")
	}
}

func htopHarness(t *testing.T) *harness {
	requireHtop(t)
	// HTOPRC=/dev/null: default layout regardless of the user's config.
	h := newHarness(t, opts{w: 160, h: 40, cmd: []string{"htop"}, env: []string{"HTOPRC=/dev/null"}})
	h.startDaemon()
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	return h
}

func (h *harness) bottom() string {
	lines := strings.Split(h.screen("main"), "\n")
	return lines[len(lines)-1]
}

func (h *harness) expectBottom(text string) {
	h.t.Helper()
	h.waitFor("bottom row to contain "+text+" (have "+h.bottom()+")", 5e9, func() bool {
		return strings.Contains(h.bottom(), text)
	})
}

// Enter search, type into it (hjkl must arrive literally), cancel with Esc.
func TestHtopSearchTypeCancel(t *testing.T) {
	h := htopHarness(t)
	h.typeKeys("/")
	h.expectState("main", "htop/insert/typing", "root")
	h.typeKeys("h", "j", "k", "l")
	h.expectBottom("Search: hjkl")
	h.typeKeys("Escape")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	h.expectBottom("F1Help")
}

// Confirming the search with Enter returns to normal mode.
func TestHtopSearchConfirm(t *testing.T) {
	h := htopHarness(t)
	h.typeKeys("/")
	h.expectState("main", "htop/insert/typing", "root")
	h.typeKeys("h", "t", "o", "p")
	h.expectBottom("Search: htop")
	h.typeKeys("Enter")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
}

// The filter prompt is insert; after Enter the filter stays applied but
// htop is navigable again.
func TestHtopFilter(t *testing.T) {
	h := htopHarness(t)
	h.typeKeys(`\`)
	h.expectState("main", "htop/insert/typing", "root")
	h.typeKeys("k")
	h.expectBottom("Filter: k")
	h.typeKeys("Enter")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
}

// Resizing mid-search keeps insert mode; the daemon re-examines on resize.
func TestHtopResizeMidSearch(t *testing.T) {
	h := htopHarness(t)
	h.typeKeys("/")
	h.expectState("main", "htop/insert/typing", "root")
	h.tmuxOut("resize-window", "-x", "90", "-y", "24")
	h.waitFor("inner pane resized", 5e9, func() bool {
		out, _ := run(h.inner, "display-message", "-p", "-t", "main", "#{pane_width}x#{pane_height}")
		return out == "90x24"
	})
	h.expectBottom("Search:")
	h.typeKeys("j")
	h.expectBottom("Search: j")
	if st := h.state("main"); st != "htop/insert/typing" {
		t.Fatalf("after resize: %s", st)
	}
	h.typeKeys("Escape")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	h.tmuxOut("resize-window", "-x", "160", "-y", "40")
	h.typeKeys("j") // remapped: moves the selection, htop stays normal
	h.expectState("main", "htop/normal/commanding", "modal-htop")
}

// The navigable panels stay normal mode; the help screen (no anchors) is
// unknown, so keys pass through, and the literal leader reaches htop's h.
func TestHtopPanelsAndLeader(t *testing.T) {
	h := htopHarness(t)
	h.typeKeys("F9")
	h.expectBottom("Cancel")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	h.typeKeys("Escape")
	h.expectBottom("F1Help")

	h.typeKeys("_", "h") // literal h: htop's help
	h.expectScreen("main", "Press any key to return")
	h.expectState("main", "htop/unknown/unknown", "root")
	h.typeKeys("q")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
}
