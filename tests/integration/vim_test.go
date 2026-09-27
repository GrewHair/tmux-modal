package integration

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// vim and neovim over SSH (the sshd container ships both): hooks-only, so
// the key table stays the user's throughout, and every mode reaches the
// transition hook. Over SSH the name comes from the layout (vim-family.toml).
func TestAppVim(t *testing.T) {
	for _, app := range []string{"vim", "nvim"} {
		t.Run(app, func(t *testing.T) {
			log, hook := hookLog(t)
			requireRemote(t)
			h := newHarness(t, opts{w: 100, h: 30,
				cmd:     sshCmd(`printf 'alpha\nalpine\nalps\n' >notes.txt && ` + app + ` notes.txt`),
				options: map[string]string{"@modal_transition_hook": hook}})
			h.startDaemon()
			h.expectState("main", app+"/normal/commanding", "root")
			for _, step := range []struct{ keys, state string }{
				{"i", "insert/typing"},
				{"Escape", "normal/commanding"},
				{"v", "visual/commanding"},
				{"Escape", "normal/commanding"},
				{"R", "replace/typing"},
				{"Escape", "normal/commanding"},
				{":", "command/typing"},
				{"Escape", "normal/commanding"},
			} {
				h.typeKeys(step.keys)
				h.expectState("main", app+"/"+step.state, "root")
			}
			h.expectHookLine(log, "mode %0 "+app+" normal>insert typing=1")
			h.expectHookLine(log, "mode %0 "+app+" visual>normal typing=0")
			h.expectHookLine(log, "mode %0 "+app+" normal>command typing=1")
		})
	}
}

// TestVimHookLatency measures keypress -> transition hook -> a warm FIFO
// listener, the path an outer keyboard layer (AutoHotkey, kanata) takes,
// with vim local (if installed; started with its defaults only, never the
// user's vimrc) and over SSH. Leaving insert includes vim's own Escape
// timeout (ttimeoutlen=100 in defaults.vim). Run with -v for the numbers.
func TestVimHookLatency(t *testing.T) {
	requireTmux(t)
	type where struct {
		name string
		cmd  func(t *testing.T) []string
	}
	for _, w := range []where{
		{"local", func(t *testing.T) []string {
			vim, err := exec.LookPath("vim")
			if err != nil {
				t.Skip("vim not installed")
			}
			f := filepath.Join(t.TempDir(), "notes.txt")
			os.WriteFile(f, []byte("alpha\n"), 0o644)
			return []string{vim, "-u", "DEFAULTS", "-i", "NONE", f}
		}},
		{"ssh", func(t *testing.T) []string {
			requireRemote(t)
			return sshCmd(`printf 'alpha\n' >notes.txt && vim notes.txt`)
		}},
	} {
		for _, profile := range []string{"balanced", "snappy"} {
			t.Run(w.name+"/"+profile, func(t *testing.T) {
				cmd := w.cmd(t)
				fifo := filepath.Join(t.TempDir(), "modal.fifo")
				if err := syscall.Mkfifo(fifo, 0o600); err != nil {
					t.Fatal(err)
				}
				// Read-write, so the open neither blocks nor sees EOF between hooks.
				f, err := os.OpenFile(fifo, os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				type event struct {
					mode string
					at   time.Time
				}
				events := make(chan event, 64)
				go func() {
					sc := bufio.NewScanner(f)
					for sc.Scan() {
						// <typing> <bucket> <mode> <app> <pane> <event>
						if fs := strings.Fields(sc.Text()); len(fs) >= 3 {
							events <- event{fs[2], time.Now()}
						}
					}
				}()
				wd, _ := os.Getwd()
				hook := "MODAL_FIFO=" + fifo + " " + filepath.Join(wd, "..", "..", "examples", "hooks", "fifo.sh")
				h := newHarness(t, opts{w: 100, h: 30, cmd: cmd,
					options: map[string]string{"@modal_transition_hook": hook,
						"@modal_profile": profile, "@modal_log_level": "warn"}})
				h.startDaemon()
				h.expectState("main", "vim/normal/commanding", "root")
				wait := func(mode string, start time.Time) time.Duration {
					timeout := time.After(3 * time.Second)
					for {
						select {
						case e := <-events:
							if e.mode == mode {
								return e.at.Sub(start)
							}
						case <-timeout:
							t.Fatalf("no hook event for %s", mode)
						}
					}
				}
				var enter, leave []time.Duration
				for i := 0; i < 15; i++ {
					start := time.Now()
					h.typeKeys("i")
					enter = append(enter, wait("insert", start))
					start = time.Now()
					h.typeKeys("Escape")
					leave = append(leave, wait("normal", start))
					time.Sleep(50 * time.Millisecond)
				}
				t.Logf("%s %s: keypress -> hook -> FIFO listener: enter insert p50=%v p90=%v max=%v | leave (incl. vim's Escape timeout) p50=%v p90=%v max=%v",
					w.name, profile, pct(enter, 50), pct(enter, 90), pct(enter, 100), pct(leave, 50), pct(leave, 90), pct(leave, 100))
				if pct(enter, 90) > time.Second || pct(leave, 90) > time.Second {
					t.Errorf("hook latency far too high")
				}
			})
		}
	}
}

// vim and neovim over SSH inside a remote tmux. With a status line (bottom
// or top) the daemon removes it and reads vim's own last row: every mode,
// low confidence. With the status line off the remote tmux is invisible
// and it is plain SSH. With the remote window split the mode is unknown:
// the outer screen cannot tell which inner pane has the keyboard. Escape
// is slow here (the inner tmux's escape-time, 500 ms on 3.4) but within
// expectState's wait. The remote tmux outlives the SSH connection, so each
// case runs its own remote tmux server on a file of its own (a second vim
// on one file stops at the swap-file prompt), killed afterwards.
func TestNestedTmuxVim(t *testing.T) {
	requireRemote(t)
	// remoteTmux: a tmux command line on a private remote socket that the
	// test kills when it ends; and a file name unique to the run.
	remoteTmux := func(t *testing.T) (string, string) {
		id := strings.NewReplacer("/", "-", " ", "").Replace(t.Name()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		t.Cleanup(func() {
			exec.Command("ssh", append(sshArgs(false), "tmux -L "+id+" kill-server; rm -f "+id+".txt ."+id+".txt.sw?")...).Run()
		})
		return "tmux -L " + id, id + ".txt"
	}
	for _, c := range []struct {
		name, app, tmuxArgs, nested, confidence string
	}{
		{"status-bottom", "vim", "", "status-line", "low"},
		{"status-top", "vim", ` \; set status-position top`, "status-line", "low"},
		{"status-off", "vim", ` \; set status off`, "", "high"},
		{"nvim-status-bottom", "nvim", "", "status-line", "low"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tmux, file := remoteTmux(t)
			h := newHarness(t, opts{w: 100, h: 30,
				cmd: sshCmd(`printf 'alpha\nalpine\n' >` + file + ` && ` + tmux + ` new-session ` + c.app + ` ` + file + c.tmuxArgs)})
			h.startDaemon()
			h.expectState("main", c.app+"/normal/commanding", "root")
			if n := h.option("main", "@modal_nested"); n != c.nested {
				t.Errorf("@modal_nested = %q, want %q", n, c.nested)
			}
			if conf := h.option("main", "@modal_confidence"); conf != c.confidence {
				t.Errorf("confidence %q, want %q", conf, c.confidence)
			}
			for _, step := range []struct{ keys, state string }{
				{"i", "insert/typing"},
				{"Escape", "normal/commanding"},
				{"v", "visual/commanding"},
				{"Escape", "normal/commanding"},
				{":", "command/typing"},
				{"Escape", "normal/commanding"},
			} {
				h.typeKeys(step.keys)
				h.expectState("main", c.app+"/"+step.state, "root")
			}
		})
	}
	t.Run("split", func(t *testing.T) {
		tmux, file := remoteTmux(t)
		h := newHarness(t, opts{w: 100, h: 30,
			cmd: sshCmd(`printf 'alpha\n' >` + file + ` && ` + tmux + ` new-session vim ` + file + ` \; split-window -h -d`)})
		h.startDaemon()
		h.waitFor("unknown with the inner split seen", 8*time.Second, func() bool {
			return strings.HasSuffix(h.state("main"), "/unknown/unknown") && h.option("main", "@modal_nested") != ""
		})
		h.typeKeys("i")
		time.Sleep(500 * time.Millisecond)
		if st := h.state("main"); !strings.HasSuffix(st, "/unknown/unknown") {
			t.Errorf("inner split, after i: %s, want unknown", st)
		}
	})
}
