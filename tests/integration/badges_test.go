package integration

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The badges on a pane border: htop recognised by its command and its
// fingerprint, remapped (MAP), and MAP turning into "MAP _" while the
// escape leader waits for its key (tmux evaluates that part when it draws
// the border). tmux-modal explain shows the same facts.
func TestBadgesHtop(t *testing.T) {
	h := htopHarness(t)
	h.tmuxIn("set-option", "-g", "pane-border-status", "top")
	h.tmuxIn("set-option", "-g", "pane-border-format", "<#{E:@modal_badges}>")
	h.waitFor("htop's facts published", 5*time.Second, func() bool {
		return h.option("main", "@modal_alt") == "on" && h.option("main", "@modal_evidence") == "cmd+fp" &&
			h.option("main", "@modal_remap") == "on" && h.option("main", "@modal_score") != ""
	})
	border := func() string {
		return strings.SplitN(h.tmuxOut("capture-pane", "-p"), "\n", 2)[0]
	}
	h.waitFor("the badges on the border", 5*time.Second, func() bool {
		b := border()
		return strings.Contains(b, "ALT") && strings.Contains(b, "htop cmd+fp") &&
			strings.Contains(b, "NORMAL") && strings.Contains(b, "MAP >")
	})
	h.typeKeys("_")
	h.waitFor("MAP _ after the leader", 3*time.Second, func() bool { return strings.Contains(border(), "MAP _ >") })
	h.typeKeys("j") // sent literally; the one-shot table ends
	h.waitFor("MAP again after the literal key", 3*time.Second, func() bool { return strings.Contains(border(), "MAP >") })

	sock := strings.TrimSpace(h.tmuxIn("display-message", "-p", "#{socket_path}"))
	cmd := exec.Command(binPath, "explain", "%0")
	cmd.Env = append(os.Environ(), "TMUX="+sock+",0,0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, out)
	}
	for _, want := range []string{"@modal_evidence", "cmd+fp", "== htop: identity", "now: app=htop mode=normal"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("explain output lacks %q:\n%s", want, out)
		}
	}
}

// The hook badge: a transition runs the hook, the badge appears on the
// border with the result, and it is gone again after @modal_hook_flash.
func TestHookBadgeFlash(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}, options: map[string]string{
		"@modal_transition_hook": "true", "@modal_hook_flash": "1000",
		"pane-border-status": "top", "pane-border-format": "<#{E:@modal_badges}>",
	}})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	border := func() string { return strings.SplitN(h.tmuxOut("capture-pane", "-p"), "\n", 2)[0] }
	h.waitFor("the startup flash to pass", 3*time.Second, func() bool { return !strings.Contains(border(), "HOOK") })
	h.typeKeys("/")
	start := time.Now()
	h.waitFor("HOOK ✓ insert on the border", 2*time.Second, func() bool { return strings.Contains(border(), "HOOK ✓ insert") })
	h.waitFor("the hook badge gone", 3*time.Second, func() bool { return !strings.Contains(border(), "HOOK") })
	if d := time.Since(start); d < 800*time.Millisecond || d > 2500*time.Millisecond {
		t.Errorf("the badge showed for about %v, want about 1 s", d)
	}
}

// The attach badge: when the daemon attaches to a session, its focused
// pane — a plain shell here — shows MODAL <version> for
// @modal_attach_flash, then it goes. The harness binary is built without
// a version.
func TestAttachBadgeFlash(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{"bash", "--norc", "--noprofile"}, options: map[string]string{
		"@modal_attach_flash": "1000",
		"pane-border-status":  "top", "pane-border-format": "<#{E:@modal_badges}>",
	}})
	border := func() string { return strings.SplitN(h.tmuxOut("capture-pane", "-p"), "\n", 2)[0] }
	start := time.Now()
	h.startDaemon()
	h.waitFor("MODAL dev on the border", 3*time.Second, func() bool { return strings.Contains(border(), "MODAL dev") })
	h.waitFor("the attach badge gone", 3*time.Second, func() bool { return !strings.Contains(border(), "MODAL") })
	if d := time.Since(start); d < 800*time.Millisecond || d > 3*time.Second {
		t.Errorf("the badge was gone after %v, want about 1 s after the attach", d)
	}
}
