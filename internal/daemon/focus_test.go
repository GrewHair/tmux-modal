package daemon

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// focusRig is two sessions, $0 (pane %0) and $1 (pane %1), each with its
// focused pane published, and a recorder for the hook events.
type focusRig struct {
	d      *Daemon
	panes  []tmux.PaneInfo
	events []string
}

func newFocusRig(focusEvents, blur bool) *focusRig {
	r := &focusRig{}
	r.d = &Daemon{
		log:   newLogger(io.Discard, "error"),
		set:   &spec.Set{Specs: map[string]*spec.Spec{}},
		cfg:   Config{Hook: "true", FocusEvents: focusEvents, HookBlur: blur},
		panes: map[string]*paneState{},
		focus: newFocusTracker(),
	}
	r.d.sink = func(e *HookEvent) {
		active := 0
		if e.Active {
			active = 1
		}
		r.events = append(r.events, fmt.Sprintf("%s %s %s active=%d %s", e.Event, e.Pane, e.ModeTo, active, e.Client))
	}
	for i, sid := range []string{"$0", "$1"} {
		p := tmux.PaneInfo{ID: fmt.Sprintf("%%%d", i), SessionID: sid, WindowActive: true, PaneActive: true}
		r.panes = append(r.panes, p)
		r.d.panes[p.ID] = &paneState{id: p.ID, info: p, published: true, cur: modeState{App: "vim", Mode: "normal"}}
	}
	return r
}

// step runs the focus part of one cycle with these clients ("name@sid",
// "+" suffix = focused) and returns the hook events it produced.
func (r *focusRig) step(clients ...string) []string {
	r.d.clients = r.d.clients[:0]
	for _, c := range clients {
		focused := c[len(c)-1] == '+'
		if focused {
			c = c[:len(c)-1]
		}
		name, sid, _ := strings.Cut(c, "@")
		r.d.clients = append(r.d.clients, clientInfo{Name: name, SessionID: sid, Focused: focused})
	}
	// every cycle has the daemon's own control client, always "focused"
	r.d.clients = append(r.d.clients, clientInfo{Name: "client-1", SessionID: "$0", Control: true})
	r.events = nil
	r.d.resolveFocus()
	r.d.sweepFocus(r.panes)
	return r.events
}

func (r *focusRig) mode(pane, mode string) {
	r.d.panes[pane].cur.Mode = mode
}

func expectEvents(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(want) == 0 {
		want = nil
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got  %q\n want %q", what, got, want)
	}
}

func TestFocusReplayOnReturn(t *testing.T) {
	r := newFocusRig(true, false)
	expectEvents(t, "attach", r.step("/dev/pts/3@$0+"), "focus %0 normal active=1 /dev/pts/3")
	expectEvents(t, "nothing changed", r.step("/dev/pts/3@$0+"))
	expectEvents(t, "to the browser (blur off)", r.step("/dev/pts/3@$0"))
	r.mode("%0", "insert")
	expectEvents(t, "mode change while away", r.step("/dev/pts/3@$0"), "mode %0 insert active=1 /dev/pts/3")
	expectEvents(t, "back from the browser", r.step("/dev/pts/3@$0+"), "focus %0 insert active=1 /dev/pts/3")
	expectEvents(t, "settled", r.step("/dev/pts/3@$0+"))
}

func TestFocusTwoTerminals(t *testing.T) {
	r := newFocusRig(true, false)
	r.step("A@$0+", "B@$1")
	r.mode("%1", "insert")
	expectEvents(t, "the other terminal's mode change", r.step("A@$0+", "B@$1"))
	expectEvents(t, "switch to B", r.step("A@$0", "B@$1+"), "focus %1 insert active=1 B")
	expectEvents(t, "back to A", r.step("A@$0+", "B@$1"), "focus %0 normal active=1 A")
	// Both flagged for a moment (the old one's focus-out not in yet): the
	// one that just gained focus wins.
	expectEvents(t, "B gains, A still flagged", r.step("A@$0+", "B@$1+"), "focus %1 insert active=1 B")
	expectEvents(t, "A's focus-out arrives", r.step("A@$0", "B@$1+"))
}

// The pane typed into is left to sweepFocus; transition reports the rest
// with MODAL_PANE_ACTIVE=0, including the other terminal's pane.
func TestFocusTypedInto(t *testing.T) {
	r := newFocusRig(true, false)
	r.step("A@$0+", "B@$1")
	if !r.d.typedInto(&r.panes[0]) || r.d.typedInto(&r.panes[1]) {
		t.Errorf("focus known: %%0 typed into, %%1 not")
	}
	r = newFocusRig(false, false)
	r.step("A@$0+", "B@$1+")
	if !r.d.typedInto(&r.panes[0]) || !r.d.typedInto(&r.panes[1]) {
		t.Errorf("focus not known: both typed into")
	}
}

func TestFocusSessionSwitchAndReattach(t *testing.T) {
	r := newFocusRig(true, false)
	r.step("A@$0+")
	expectEvents(t, "switch-client to $1", r.step("A@$1+"), "focus %1 normal active=1 A")
	expectEvents(t, "switch-client back", r.step("A@$0+"), "focus %0 normal active=1 A")
	expectEvents(t, "detached", r.step())
	expectEvents(t, "reattached (same name)", r.step("A@$0+"), "focus %0 normal active=1 A")
}

func TestFocusBlur(t *testing.T) {
	r := newFocusRig(true, true)
	r.step("A@$0+")
	expectEvents(t, "focus lost", r.step("A@$0"), "blur %0 none active=1 A")
	r.mode("%0", "insert")
	expectEvents(t, "no reports while blurred", r.step("A@$0"))
	expectEvents(t, "focus back", r.step("A@$0+"), "focus %0 insert active=1 A")
}

// focus-events off: every client stays flagged, so focus is not known and
// each terminal's pane is reported, as before focus tracking.
func TestFocusUnknown(t *testing.T) {
	r := newFocusRig(false, true)
	expectEvents(t, "attach", r.step("A@$0+", "B@$1+"),
		"focus %0 normal active=1 A", "focus %1 normal active=1 B")
	r.mode("%1", "insert")
	expectEvents(t, "mode change", r.step("A@$0+", "B@$1+"), "mode %1 insert active=1 B")
	expectEvents(t, "no blur without focus", r.step("A@$0", "B@$1"))
	expectEvents(t, "two new terminals on one session: one report", r.step("A@$0+", "B@$1+", "C@$1+", "D@$1+"),
		"focus %1 insert active=1 C")
	expectEvents(t, "switch-client", r.step("A@$1+", "B@$1+"), "focus %1 insert active=1 A")
}
