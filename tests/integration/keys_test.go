package integration

import (
	"strings"
	"testing"
	"time"
)

// The remap reaches the app as the mapped key; typing mode passes every
// key through literally, including the first one after the transition.
func TestRemapAndPromptPassThrough(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")

	h.typeKeys("j")
	h.expectScreen("main", "last=[Down]")
	h.typeKeys("C-d")
	h.expectScreen("main", "last=[PageDown]")
	h.typeKeys("x") // unmapped keys fall through to the pane
	h.expectScreen("main", "last=[x]")

	h.typeKeys("/")
	h.expectState("main", "keyecho/insert/typing", "root")
	// The first key after the switch must not be eaten (see
	// repointClients), and mapped keys must arrive literally.
	h.typeKeys("j")
	h.typeKeys("k", "g")
	h.expectScreen("main", "PROMPT> jkg")

	h.typeKeys("Escape")
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.typeKeys("k")
	h.expectScreen("main", "last=[Up]")
}

// leader + key sends the key verbatim, exactly once; leader leader sends
// the leader.
func TestEscapeLeader(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")

	h.typeKeys("_", "j")
	h.expectScreen("main", "last=[j]")
	h.typeKeys("j") // one-shot: the next key is remapped again
	h.expectScreen("main", "last=[Down]")
	h.typeKeys("_", "_")
	h.expectScreen("main", "last=[_]")
	h.typeKeys("j")
	h.expectScreen("main", "last=[Down]")
}

// A hooks-only spec is detected and published, but the session key-table
// is never touched, and keys are never remapped.
func TestHooksOnlyNeverTouchesKeyTable(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}, env: []string{"KEYECHO_NAME=KEYHOOK"}})
	h.startDaemon()
	h.expectState("main", "keyhook/normal/commanding", "")
	h.typeKeys("/")
	h.expectState("main", "keyhook/insert/typing", "")
	h.typeKeys("Escape")
	h.expectState("main", "keyhook/normal/commanding", "")
	h.typeKeys("j")
	h.expectScreen("main", "last=[j]")
	if kt := h.keyTable(); kt != "root" {
		t.Fatalf("key-table = %q, want root", kt)
	}
	if out, _ := run(h.inner, "show-options", "-qv", "-t", "main", "key-table"); out != "" {
		t.Fatalf("session-level key-table was set to %q", out)
	}
	if out, _ := run(h.inner, "show-options", "-qv", "-t", "main", "@modal_saved_key_table"); out != "" {
		t.Fatalf("daemon recorded a saved key-table for a hooks-only app: %q", out)
	}
}

// The session key-table follows the focused pane.
func TestKeyTableFollowsFocus(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.tmuxIn("split-window", "-d", "-t", "main", "bash --norc --noprofile")
	h.startDaemon()
	h.expectState("%0", "keyecho/normal/commanding", "modal-keyecho")

	h.tmuxIn("select-pane", "-t", "%1")
	h.expectState("%1", "/none/none", "root")
	h.typeKeys("e", "c", "h", "o", " ", "j", "k")
	h.expectScreen("%1", "echo jk")

	h.tmuxIn("select-pane", "-t", "%0")
	h.expectState("%0", "keyecho/normal/commanding", "modal-keyecho")
	h.typeKeys("j")
	h.expectScreen("%0", "last=[Down]")
}

// @modal_enabled off on one pane disables it for that pane only.
func TestPerPaneDisable(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.tmuxIn("set-option", "-p", "-t", "main", "@modal_enabled", "off")
	h.typeKeys("x") // produce output so the pane is re-examined promptly
	h.expectState("main", "//", "root")
	h.typeKeys("j")
	h.expectScreen("main", "last=[j]")
}

// A session with its own key-table gets exactly that table back.
func TestRestoresSessionKeyTable(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.tmuxIn("set-option", "-t", "main", "key-table", "custom")
	h.tmuxIn("bind-key", "-T", "custom", "F12", "display-message", "custom")
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.typeKeys("/")
	h.expectState("main", "keyecho/insert/typing", "custom")
	h.typeKeys("Escape")
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.stopDaemon()
	if kt := h.keyTable(); kt != "custom" {
		t.Fatalf("after stop key-table = %q, want custom", kt)
	}
}

// Stopping the daemon restores key tables and clears published state; a
// daemon killed without cleanup is repaired by the next one.
func TestStopAndCrashRecovery(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.stopDaemon()
	if kt := h.keyTable(); kt != "root" {
		t.Fatalf("after stop key-table = %q", kt)
	}
	if st := h.state("main"); st != "//" {
		t.Fatalf("after stop state = %q", st)
	}

	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.daemon.Process.Kill() // no cleanup
	h.daemon.Wait()
	h.daemon = nil
	// The stale table is still the default, but the guard in every
	// binding makes it harmless once the pane is not commanding.
	h.startDaemon()
	h.typeKeys("/")
	h.expectState("main", "keyecho/insert/typing", "root")
	h.typeKeys("j")
	h.expectScreen("main", "PROMPT> j")
}

// Mouse and `bind -n` root bindings keep working while the modal table
// is the default.
func TestRootBindingsCopied(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.tmuxIn("bind-key", "-n", "F5", "set-option", "-g", "@it_marker", "pressed")
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	keys := h.tmuxIn("list-keys", "-T", "modal-keyecho")
	for _, want := range []string{"MouseDown1Pane", "WheelUpPane", " F5 "} {
		if !strings.Contains(keys, want) {
			t.Errorf("modal table lacks root binding %q", want)
		}
	}
	h.typeKeys("F5")
	h.waitFor("F5 root binding to fire", 3*time.Second, func() bool {
		out, _ := run(h.inner, "show-options", "-gqv", "@it_marker")
		return out == "pressed"
	})
}

// The daemon's own connection is unmistakable in `tmux list-clients`.
func TestDaemonClientIsLabelled(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	out := h.tmuxIn("list-clients")
	if !strings.Contains(out, "TMUX-MODAL-DAEMON") {
		t.Fatalf("list-clients does not show the daemon label:\n%s", out)
	}
}
