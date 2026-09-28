package integration

import (
	"os"
	"path/filepath"
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

// An overlay user spec gives the detection-only tig spec keys (D29): the
// bundled rules still decide the mode, the overlay only adds [keys]. K is
// tig's "previous line"; remapped to Down it must move the selection down
// in normal mode and stay a letter in the search prompt. The file is
// written while the daemon runs: spec files are reloaded when they change.
func TestAppOverlayAddsKeys(t *testing.T) {
	dir := t.TempDir()
	requireRemote(t)
	h := newHarness(t, opts{w: 120, h: 36, cmd: sshCmd("cd repo && tig"),
		options: map[string]string{"@modal_idle_interval": "300"}})
	h.tmuxIn("set-option", "-g", "@modal_spec_paths", specDir+":"+dir)
	h.startDaemon()
	h.expectState("main", "tig/normal/commanding", "root")
	if err := os.WriteFile(filepath.Join(dir, "tig.toml"),
		[]byte("overlay = true\n[keys]\nK = \"Down\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.expectState("main", "tig/normal/commanding", "modal-tig")
	h.expectScreen("main", "[main] Unstaged changes")
	h.typeKeys("K") // natively Up: would stay on the first line
	h.expectScreen("main", "commit 1 of ")
	h.typeKeys("/")
	h.expectState("main", "tig/insert/typing", "root")
	h.typeKeys("K")
	h.waitFor("K typed into the prompt", 5*time.Second, func() bool {
		lines := strings.Split(strings.TrimRight(h.screen("main"), "\n"), "\n")
		return strings.TrimSpace(lines[len(lines)-1]) == "/K"
	})
	h.typeKeys("C-c")
	h.expectState("main", "tig/normal/commanding", "modal-tig")
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

// debconf's questions over SSH, as `sudo apt` shows them (whiptail through
// debconf's dialog frontend): ucf's real modified-conffile prompt, then a
// text question. hjkl move while a list or the buttons have focus; in a
// text entry they are letters, and Tab to the buttons remaps again.
func TestAppDebconf(t *testing.T) {
	h := remoteApp(t, 100, 30, "sudo ucf-probe && echo ucf done && cat /etc/tmux-modal-probe.conf; sleep 60")
	h.expectState("main", "debconf/normal/commanding", "modal-debconf")
	// j j: the third choice, "show the differences".
	h.typeSlowly("j", "j", "Enter")
	h.expectScreen("main", "Line by line differences")
	h.expectState("main", "debconf/normal/commanding", "modal-debconf")
	h.typeSlowly("Enter")
	h.expectScreen("main", "What do you want to do")
	// k k: back to the first, "install the package maintainer's version".
	h.typeSlowly("k", "k", "Enter")
	h.expectScreen("main", "ucf done")
	h.expectState("main", "/none/none", "root")
	if out := h.screen("main"); !strings.Contains(out, "setting = 2") || strings.Contains(out, "local = yes") {
		t.Errorf("the maintainer's version was not installed:\n%s", out)
	}

	h = remoteApp(t, 100, 30, "sudo debconf-probe mailname save; cat /etc/mailname 2>/dev/null; sudo debconf-show tmux-modal-probe | grep mailname; sleep 60")
	h.expectState("main", "debconf/insert/typing", "root")
	h.typeSlowly("j", "k")
	h.expectScreen("main", "example.orgjk")
	h.typeSlowly("Tab")
	h.expectState("main", "debconf/normal/commanding", "modal-debconf")
	// l: Right, to <Cancel>; h: back to <Ok>.
	h.typeSlowly("l", "h", "Enter")
	h.expectScreen("main", "Save current IPv4 rules?")
	h.expectState("main", "debconf/normal/commanding", "modal-debconf")
	h.typeSlowly("Enter")
	h.expectScreen("main", "mailname: example.orgjk")
}

// whiptail from a script over SSH (raspi-config's menu shape): j j picks
// the third entry; an input box takes hjkl as letters.
func TestAppWhiptail(t *testing.T) {
	h := remoteApp(t, 100, 30, `whiptail --title "Configuration Tool" --menu "Setup Options" 20 70 6 `+
		`--ok-button Select --cancel-button Finish "1 System" "a" "2 Display" "b" "3 Interface" "c" 2>/tmp/choice; `+
		`echo "picked $(cat /tmp/choice)"; `+
		`whiptail --inputbox "Hostname" 10 50 2>/tmp/host; echo "host $(cat /tmp/host)"; sleep 60`)
	h.expectState("main", "whiptail/normal/commanding", "modal-whiptail")
	h.typeSlowly("j", "j", "Enter")
	// The input box covers the menu's output until it closes.
	h.expectState("main", "whiptail/insert/typing", "root")
	h.typeSlowly("h", "j", "k", "l")
	h.expectScreen("main", "hjkl")
	h.typeSlowly("Enter")
	h.expectScreen("main", "host hjkl")
	h.expectScreen("main", "picked 3 Interface")
}
