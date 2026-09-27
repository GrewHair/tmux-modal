// Package integration drives real tmux servers, real applications and a
// real attached client.
//
// Keystrokes must go through a client's key tables, which send-keys
// bypasses, so every test runs two servers: an inner one holding the
// session under test, and an outer one whose pane runs `tmux attach` to the
// inner session. Sending keys to the outer pane types into a genuine inner
// client. Every server uses its own -L socket and -f /dev/null.
package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	binPath   string
	binDir    string
	specDir   string
	keyecho   string
	haveTmux  bool
	haveHtop  bool
	socketSeq int
)

func TestMain(m *testing.M) {
	_, err := exec.LookPath("tmux")
	haveTmux = err == nil
	_, err = exec.LookPath("htop")
	haveHtop = err == nil
	dir, err := os.MkdirTemp("", "tmux-modal-it-")
	if err != nil {
		panic(err)
	}
	// Readable by the SSH container, which mounts it to run keyecho.
	os.Chmod(dir, 0o755)
	binDir = dir
	binPath = filepath.Join(dir, "tmux-modal")
	keyecho = filepath.Join(dir, "keyecho")
	for pkg, out := range map[string]string{"../../cmd/tmux-modal": binPath, "./keyecho": keyecho} {
		build := exec.Command("go", "build", "-o", out, pkg)
		build.Env = append(os.Environ(), "CGO_ENABLED=0") // static: also runs in the container
		build.Stdout, build.Stderr = os.Stdout, os.Stderr
		if err := build.Run(); err != nil {
			panic(err)
		}
	}
	wd, _ := os.Getwd()
	specDir = filepath.Join(wd, "testdata", "specs")
	code := m.Run()
	stopRemote()
	os.RemoveAll(dir)
	os.Exit(code)
}

type harness struct {
	t            *testing.T
	inner, outer string
	logPath      string
	daemon       *exec.Cmd
}

type opts struct {
	w, h    int
	cmd     []string
	env     []string
	options map[string]string // extra global options set before the daemon starts
}

func requireTmux(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test (-short)")
	}
	if !haveTmux {
		t.Skip("tmux not installed")
	}
}

func newHarness(t *testing.T, o opts) *harness {
	t.Helper()
	requireTmux(t)
	socketSeq++
	id := fmt.Sprintf("%d-%d", os.Getpid(), socketSeq)
	h := &harness{t: t, inner: "mi-" + id, outer: "mo-" + id,
		logPath: filepath.Join(t.TempDir(), "daemon.log")}
	if o.w == 0 {
		o.w, o.h = 120, 40
	}
	args := []string{"-f", "/dev/null", "new-session", "-d", "-s", "main", "-x", fmt.Sprint(o.w), "-y", fmt.Sprint(o.h)}
	for _, e := range o.env {
		args = append(args, "-e", e)
	}
	h.tmuxIn(append(args, o.cmd...)...)
	h.tmuxIn("set-option", "-g", "status", "off")
	// tmux holds a bare Escape for escape-time (default 500ms) to see if a
	// sequence follows; that would dominate every Esc transition measured.
	h.tmuxIn("set-option", "-s", "escape-time", "5")
	h.tmuxIn("set-option", "-g", "@modal_spec_paths", specDir)
	h.tmuxIn("set-option", "-g", "@modal_log_level", "debug")
	for k, v := range o.options {
		h.tmuxIn("set-option", "-g", k, v)
	}
	h.tmuxOut("-f", "/dev/null", "new-session", "-d", "-x", fmt.Sprint(o.w), "-y", fmt.Sprint(o.h),
		fmt.Sprintf("tmux -L %s attach -t main", h.inner))
	h.tmuxOut("set-option", "-g", "status", "off")
	h.tmuxOut("set-option", "-s", "escape-time", "5")
	h.waitFor("inner client attached", 5*time.Second, func() bool {
		return strings.Contains(h.tmuxIn("list-clients", "-F", "#{client_control_mode}"), "0")
	})
	t.Cleanup(h.close)
	return h
}

func (h *harness) startDaemon() {
	h.t.Helper()
	h.daemon = exec.Command(binPath, "daemon", "-L", h.inner, "--log", h.logPath)
	if f, err := os.OpenFile(h.logPath+".stderr", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		h.daemon.Stdout, h.daemon.Stderr = f, f
	}
	if err := h.daemon.Start(); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) stopDaemon() {
	if h.daemon == nil {
		return
	}
	h.daemon.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { h.daemon.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		h.daemon.Process.Kill()
		<-done
	}
	h.daemon = nil
}

func (h *harness) close() {
	h.stopDaemon()
	if h.t.Failed() {
		if b, err := os.ReadFile(h.logPath); err == nil {
			h.t.Logf("daemon log:\n%s", b)
		}
		if b, err := os.ReadFile(h.logPath + ".stderr"); err == nil && len(b) > 0 {
			h.t.Logf("daemon stderr:\n%s", b)
		}
		h.t.Logf("final screen:\n%s", h.tmuxIn("capture-pane", "-p", "-t", "main"))
	}
	exec.Command("tmux", "-L", h.outer, "kill-server").Run()
	exec.Command("tmux", "-L", h.inner, "kill-server").Run()
}

func run(label string, args ...string) (string, error) {
	out, err := exec.Command("tmux", append([]string{"-L", label}, args...)...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

func (h *harness) tmuxIn(args ...string) string {
	h.t.Helper()
	out, err := run(h.inner, args...)
	if err != nil {
		h.t.Fatalf("tmux -L %s %v: %v: %s", h.inner, args, err, out)
	}
	return out
}

func (h *harness) tmuxOut(args ...string) string {
	h.t.Helper()
	out, err := run(h.outer, args...)
	if err != nil {
		h.t.Fatalf("tmux -L %s %v: %v: %s", h.outer, args, err, out)
	}
	return out
}

// typeKeys types through the real attached client.
func (h *harness) typeKeys(keys ...string) {
	h.t.Helper()
	h.tmuxOut(append([]string{"send-keys", "-t", "%0"}, keys...)...)
}

func (h *harness) state(target string) string {
	out, _ := run(h.inner, "display-message", "-p", "-t", target, "#{@modal_app}/#{@modal_mode}/#{@modal_bucket}")
	return out
}

func (h *harness) keyTable() string {
	out, _ := run(h.inner, "display-message", "-p", "-t", "main", "#{key-table}")
	return out
}

func (h *harness) screen(target string) string {
	out, _ := run(h.inner, "capture-pane", "-p", "-t", target)
	return out
}

func (h *harness) waitFor(what string, timeout time.Duration, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

// expectState waits for a pane's published state and the session
// key-table to reach the given values.
func (h *harness) expectState(target, state, table string) {
	h.t.Helper()
	h.waitFor(fmt.Sprintf("%s state %q key-table %q (have %q / %q)", target, state, table,
		h.state(target), h.keyTable()), 5*time.Second, func() bool {
		return h.state(target) == state && (table == "" || h.keyTable() == table)
	})
}

// expectScreen waits until the pane shows text.
func (h *harness) expectScreen(target, text string) {
	h.t.Helper()
	h.waitFor(fmt.Sprintf("screen of %s to contain %q", target, text), 5*time.Second, func() bool {
		return strings.Contains(h.screen(target), text)
	})
}
