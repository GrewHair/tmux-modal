package integration

import (
	"strings"
	"testing"
	"time"
)

// The bundled specs against the real applications, run on the remote
// container over SSH (tests/docker/sshd.Dockerfile installs them, with a
// small git repository in ~/repo and a directory tree in ~/tree). The pane
// command is ssh, so identity comes from the screen alone: the harder case,
// and no application has to be installed on the machine running the tests.
//
// Hooks-only specs never touch the key table ("root" throughout); btop
// remaps, so its table follows the mode.

func remoteApp(t *testing.T, w, h int, cmd string) *harness {
	t.Helper()
	requireRemote(t)
	hr := newHarness(t, opts{w: w, h: h, cmd: sshCmd(cmd)})
	hr.startDaemon()
	return hr
}

// typeSlowly sends keys one at a time: some applications (btop) drop keys
// that arrive in one burst.
func (h *harness) typeSlowly(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		h.typeKeys(k)
		time.Sleep(150 * time.Millisecond)
	}
}

func TestAppLess(t *testing.T) {
	h := remoteApp(t, 100, 30, "less sample.txt")
	h.expectScreen("main", "Sample text line 1")
	// Over SSH the first screen (a file name) and the idle ":" do not
	// identify less; its end-of-file prompt does, and then it is sticky.
	h.typeKeys("G")
	h.expectState("main", "less/normal/commanding", "root")
	h.typeKeys("/")
	h.expectState("main", "less/insert/typing", "root")
	h.typeKeys("line 5", "Enter")
	h.expectState("main", "less/normal/commanding", "root")
	h.typeKeys("&")
	h.expectState("main", "less/insert/typing", "root")
	h.typeKeys("C-c")
	h.expectState("main", "less/normal/commanding", "root")
}

func TestAppMan(t *testing.T) {
	h := remoteApp(t, 100, 30, "man ls")
	h.expectState("main", "man/normal/commanding", "root")
	h.typeKeys("/")
	h.expectState("main", "man/insert/typing", "root")
	h.typeKeys("sort", "Enter")
	h.expectState("main", "man/normal/commanding", "root")
}

func TestAppTig(t *testing.T) {
	h := remoteApp(t, 120, 36, "cd repo && tig")
	h.expectState("main", "tig/normal/commanding", "root")
	h.typeKeys(":")
	h.expectState("main", "tig/command/typing", "root")
	h.typeKeys("C-c")
	h.expectState("main", "tig/normal/commanding", "root")
	h.typeKeys("/")
	h.expectState("main", "tig/insert/typing", "root")
	h.typeKeys("C-c")
	h.expectState("main", "tig/normal/commanding", "root")
}

// btop is the one new spec that remaps: hjkl move the process selection,
// but typed into the filter they stay letters.
func TestAppBtop(t *testing.T) {
	h := remoteApp(t, 120, 40, "btop")
	h.expectState("main", "btop/normal/commanding", "modal-btop")
	h.typeSlowly("f")
	h.expectState("main", "btop/insert/typing", "root")
	h.typeSlowly("j", "k")
	h.waitFor("filter text jk", 5*time.Second, func() bool {
		return strings.Contains(h.screen("main"), "f jk")
	})
	h.typeSlowly("Enter")
	h.expectState("main", "btop/normal/commanding", "modal-btop")
	h.typeSlowly("Delete")
	h.typeSlowly("o")
	h.expectState("main", "btop/insert/typing", "root")
	h.typeSlowly("Escape")
	h.expectState("main", "btop/normal/commanding", "modal-btop")
}

func TestAppLazygit(t *testing.T) {
	h := remoteApp(t, 120, 30, "cd repo && lazygit")
	h.expectScreen("main", "Press <enter> to get started")
	h.typeKeys("Enter")
	h.expectState("main", "lazygit/normal/commanding", "root")
	h.typeKeys("/")
	h.expectState("main", "lazygit/insert/typing", "root")
	h.typeKeys("Escape")
	h.expectState("main", "lazygit/normal/commanding", "root")
	h.typeKeys("?")
	h.expectScreen("main", "(Type to filter)")
	h.expectState("main", "lazygit/normal/commanding", "root")
	h.typeKeys("Escape")
	// Escape followed at once by a key reads as Alt+key to the app.
	h.waitFor("menu closed", 5*time.Second, func() bool {
		return !strings.Contains(h.screen("main"), "(Type to filter)")
	})
	h.typeKeys("3")
	h.expectScreen("main", "New branch: n")
	h.typeKeys("n")
	h.expectState("main", "lazygit/insert/typing", "root")
	h.typeKeys("Escape")
	h.expectState("main", "lazygit/normal/commanding", "root")
}

func TestAppFileManagers(t *testing.T) {
	for _, c := range []struct{ app, prompt, mode string }{
		{"ranger", ":", "command/typing"},
		{"lf", ":", "command/typing"},
		{"lf", "/", "insert/typing"},
		{"nnn", "/", "insert/typing"},
	} {
		t.Run(c.app+c.prompt, func(t *testing.T) {
			h := remoteApp(t, 100, 30, "cd tree && "+c.app)
			h.expectState("main", c.app+"/normal/commanding", "root")
			h.typeKeys(c.prompt)
			h.expectState("main", c.app+"/"+c.mode, "root")
			h.typeKeys("Escape")
			h.expectState("main", c.app+"/normal/commanding", "root")
		})
	}
}

func TestAppNcduMcFzf(t *testing.T) {
	for _, c := range []struct{ cmd, state string }{
		{"ncdu tree", "ncdu/normal/commanding"},
		{"mc", "mc/insert/typing"},
		{"seq 1000 | fzf", "fzf/insert/typing"},
	} {
		t.Run(strings.Fields(c.cmd)[0], func(t *testing.T) {
			h := remoteApp(t, 100, 30, c.cmd)
			h.expectState("main", c.state, "root")
		})
	}
}
