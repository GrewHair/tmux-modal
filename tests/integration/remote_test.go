package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The remote tier: a container running sshd (tests/docker/sshd.Dockerfile)
// stands in for a remote host with nothing of tmux-modal on it. The local
// pane runs `ssh -t demo@container ...`, so the daemon sees pane command
// "ssh" and must identify the application from the screen alone.
//
// Needs docker; skipped without it (and with -short). The image is built
// on first use and the container is shared by all tests of the run.

const remoteImage = "tmux-modal-sshd"

var remote struct {
	once      sync.Once
	err       error
	container string
	port      string
	key       string
	dir       string
}

func requireRemote(t *testing.T) {
	t.Helper()
	requireTmux(t)
	remote.once.Do(startRemote)
	if remote.err != nil {
		if os.Getenv("TMUX_MODAL_REQUIRE_REMOTE") != "" {
			t.Fatalf("remote tier unavailable: %v", remote.err)
		}
		t.Skipf("remote tier unavailable: %v", remote.err)
	}
}

func startRemote() {
	fail := func(format string, args ...any) { remote.err = fmt.Errorf(format, args...) }
	if _, err := exec.LookPath("docker"); err != nil {
		fail("docker not installed")
		return
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		fail("ssh not installed")
		return
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		fail("docker info: %v: %s", err, firstLine(out))
		return
	}
	wd, _ := os.Getwd()
	root := filepath.Join(wd, "..", "docker")
	build := exec.Command("docker", "build", "-q", "-t", remoteImage, "-f", filepath.Join(root, "sshd.Dockerfile"), root)
	if out, err := build.CombinedOutput(); err != nil {
		fail("docker build: %v: %s", err, out)
		return
	}
	dir, err := os.MkdirTemp("", "tmux-modal-remote-")
	if err != nil {
		fail("%v", err)
		return
	}
	remote.dir = dir
	remote.key = filepath.Join(dir, "key")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", remote.key).CombinedOutput(); err != nil {
		fail("ssh-keygen: %v: %s", err, out)
		return
	}
	// The key must be readable by the container's root, which then
	// installs it for demo.
	os.Chmod(remote.key+".pub", 0o644)
	os.Chmod(dir, 0o755)
	out, err := exec.Command("docker", "run", "-d", "--rm", "--hostname", "remote",
		"-p", "127.0.0.1::22",
		"-v", remote.key+".pub:/authorized_keys:ro",
		"-v", binDir+":/testbin:ro",
		remoteImage).Output()
	if err != nil {
		fail("docker run: %v", err)
		return
	}
	remote.container = strings.TrimSpace(string(out))
	out, err = exec.Command("docker", "port", remote.container, "22/tcp").Output()
	if err != nil {
		fail("docker port: %v", err)
		return
	}
	addr := strings.Fields(string(out))
	if len(addr) == 0 {
		fail("docker port: no mapping")
		return
	}
	remote.port = addr[0][strings.LastIndex(addr[0], ":")+1:]
	deadline := time.Now().Add(20 * time.Second)
	for {
		probe := exec.Command("ssh", append(sshArgs(false), "true")...)
		out, err := probe.CombinedOutput()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			fail("ssh never came up: %v: %s", err, out)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func stopRemote() {
	if remote.container != "" {
		exec.Command("docker", "rm", "-f", remote.container).Run()
	}
	if remote.dir != "" {
		os.RemoveAll(remote.dir)
	}
}

func sshArgs(tty bool) []string {
	args := []string{"-p", remote.port, "-i", remote.key,
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR", "-o", "BatchMode=yes"}
	if tty {
		args = append(args, "-t")
	}
	return append(args, "demo@127.0.0.1")
}

// sshCmd is a pane command that runs remoteCmd on the remote host in a
// terminal.
func sshCmd(remoteCmd string) []string {
	return append(append([]string{"ssh"}, sshArgs(true)...), remoteCmd)
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func (h *harness) option(target, name string) string {
	out, _ := run(h.inner, "display-message", "-p", "-t", target, "#{"+name+"}")
	return out
}

// remoteKeyecho is the keyecho test application on the remote host (the
// directory the tests build it into is mounted at /testbin).
const remoteKeyecho = "/testbin/keyecho"

// ---- plain SSH: detection and remapping exactly as locally ----------------

func TestSSHRemapAndPrompt(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{cmd: sshCmd(remoteKeyecho)})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	if cmd := h.option("main", "pane_current_command"); cmd != "ssh" {
		t.Fatalf("pane command %q, want ssh", cmd)
	}
	h.typeKeys("j")
	h.expectScreen("main", "last=[Down]")
	h.typeKeys("/")
	h.expectState("main", "keyecho/insert/typing", "root")
	h.typeKeys("j", "k")
	h.expectScreen("main", "PROMPT> jk")
	h.typeKeys("Escape")
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.typeKeys("k")
	h.expectScreen("main", "last=[Up]")
	if n := h.option("main", "@modal_nested"); n != "" {
		t.Fatalf("plain ssh reported as nested (%s)", n)
	}
}

func TestSSHHtop(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{w: 160, h: 40, cmd: sshCmd("env HTOPRC=/dev/null htop")})
	h.startDaemon()
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	h.typeKeys("/")
	h.expectState("main", "htop/insert/typing", "root")
	h.typeKeys("h", "j", "k", "l")
	h.expectBottom("Search: hjkl")
	h.typeKeys("Escape")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	h.typeKeys(`\`)
	h.expectState("main", "htop/insert/typing", "root")
	h.typeKeys("Enter")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
}

// Leaving the remote application for the remote shell drops the identity:
// the pane is still "ssh", but no spec matches the shell's screen.
func TestSSHAppExit(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{cmd: sshCmd("bash --norc")})
	h.startDaemon()
	h.waitFor("remote shell prompt", 10*time.Second, func() bool {
		return strings.Contains(h.screen("main"), "$")
	})
	h.expectState("main", "/none/none", "")
	h.typeKeys(remoteKeyecho, "Enter")
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.typeKeys("q")
	h.expectState("main", "/none/none", "root")
	h.typeKeys("j")
	h.expectScreen("main", "$ j")
}

// ---- nested tmux ---------------------------------------------------------------

// The remote runs tmux with its default status line: the daemon removes
// that line and remaps as usual (@modal_nested_remap defaults to on).
func TestNestedTmuxRemaps(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{cmd: sshCmd("tmux new-session " + remoteKeyecho)})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	if n := h.option("main", "@modal_nested"); n != "status-line" {
		t.Fatalf("@modal_nested = %q, want status-line", n)
	}
	if c := h.option("main", "@modal_confidence"); c != "low" {
		t.Fatalf("confidence %q, want low", c)
	}
	h.typeKeys("j")
	h.expectScreen("main", "last=[Down]")
	// The prompt is one row above the inner status line.
	h.typeKeys("/")
	h.expectState("main", "keyecho/insert/typing", "root")
	h.typeKeys("j")
	h.expectScreen("main", "PROMPT> j")
	h.typeKeys("Escape")
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.typeKeys("k")
	h.expectScreen("main", "last=[Up]")
}

// With @modal_nested_remap off the daemon never remaps through a nested
// tmux, in any state.
func TestNestedTmuxRemapOff(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{cmd: sshCmd("tmux new-session " + remoteKeyecho),
		options: map[string]string{"@modal_nested_remap": "off"}})
	h.startDaemon()
	h.expectScreen("main", "KEYECHO TEST APPLICATION")
	h.expectState("main", "keyecho/unknown/unknown", "root")
	if n := h.option("main", "@modal_nested"); n != "status-line" {
		t.Fatalf("@modal_nested = %q, want status-line", n)
	}
	h.typeKeys("j")
	h.expectScreen("main", "last=[j]")
	h.typeKeys("/", "j")
	h.expectScreen("main", "PROMPT> j")
	h.typeKeys("Escape")
	h.expectScreen("main", "last=[Escape]")
	h.typeKeys("k")
	h.expectScreen("main", "last=[k]")
	if st := h.state("main"); st != "keyecho/unknown/unknown" {
		t.Fatalf("state %s", st)
	}
}

// htop under a remote tmux whose status line covers the last row: the
// search prompt is one row up and must still be seen, or htop would read
// as normal while the user types.
func TestNestedTmuxHtop(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{w: 160, h: 40, cmd: sshCmd("tmux new-session env HTOPRC=/dev/null htop")})
	h.startDaemon()
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	h.typeKeys("/")
	h.expectState("main", "htop/insert/typing", "root")
	h.typeKeys("j")
	h.expectScreen("main", "Search: j")
	h.typeKeys("Escape")
	h.expectState("main", "htop/normal/commanding", "modal-htop")
}

// Inner splits: the keyboard belongs to whichever inner pane is active,
// which the outer screen cannot tell. Even with the inner status line off,
// the borders give the nesting away.
func TestNestedTmuxSplit(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{cmd: sshCmd("tmux new-session " + remoteKeyecho + ` \; set status off \; split-window -h -d`)})
	h.startDaemon()
	h.expectScreen("main", "KEYECHO TEST APPLICATION")
	h.waitFor("inner split", 5*time.Second, func() bool { return strings.Contains(h.screen("main"), "│") })
	// The banner row now ends in the border and the right pane, so the
	// app may not even be identified; either way the mode is unknown.
	// Wait for both together: a capture taken while the inner tmux is
	// still drawing the split can already be unknown without showing the
	// whole border yet; the next capture has it.
	h.waitFor("unknown mode with border evidence", 5*time.Second, func() bool {
		return strings.HasSuffix(h.state("main"), "/unknown/unknown") && h.keyTable() == "root" &&
			h.option("main", "@modal_nested") == "borders"
	})
	h.typeKeys("j")
	h.expectScreen("main", "last=[j]")
}

// A hooks-only spec still reports modes through a nested tmux (they drive
// hooks, not keys), with low confidence, read from the screen minus the
// inner status line.
func TestNestedTmuxHooksOnly(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{cmd: sshCmd("tmux new-session env KEYECHO_NAME=KEYHOOK " + remoteKeyecho)})
	h.startDaemon()
	h.expectState("main", "keyhook/normal/commanding", "root")
	if c := h.option("main", "@modal_confidence"); c != "low" {
		t.Fatalf("confidence %q, want low", c)
	}
	h.typeKeys("/")
	h.expectState("main", "keyhook/insert/typing", "root")
	h.typeKeys("Escape")
	h.expectState("main", "keyhook/normal/commanding", "root")
}

// Documented limitation: a remote tmux with its status line off and a
// single pane draws exactly what the application draws, so it cannot be
// told apart from plain ssh, and keys are remapped as they would be
// without it. Keys still reach the inner active pane — the only one — so
// this is correct except after the inner prefix key (see README).
func TestNestedTmuxInvisible(t *testing.T) {
	requireRemote(t)
	h := newHarness(t, opts{cmd: sshCmd("tmux new-session " + remoteKeyecho + ` \; set status off`)})
	h.startDaemon()
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	h.typeKeys("j")
	h.expectScreen("main", "last=[Down]")
}

// A local tmux client inside a pane (tmux in tmux on one machine) is
// nested by its command alone.
func TestNestedLocalTmux(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{"bash", "--norc"}})
	third := strings.Replace(h.inner, "mi-", "mt-", 1)
	t.Cleanup(func() { exec.Command("tmux", "-L", third, "kill-server").Run() })
	if out, err := run(third, "-f", "/dev/null", "new-session", "-d", "-x", "120", "-y", "40", keyecho); err != nil {
		t.Fatalf("third server: %v: %s", err, out)
	}
	run(third, "set-option", "-g", "status", "off")
	h.startDaemon()
	h.expectState("main", "/none/none", "")
	h.typeKeys(fmt.Sprintf("exec tmux -L %s attach", third), "Enter")
	h.expectScreen("main", "KEYECHO TEST APPLICATION")
	h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")
	if n := h.option("main", "@modal_nested"); n != "command" {
		t.Fatalf("@modal_nested = %q, want command", n)
	}
	h.typeKeys("j")
	h.expectScreen("main", "last=[Down]")
}
