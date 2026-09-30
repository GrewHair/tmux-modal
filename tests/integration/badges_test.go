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
