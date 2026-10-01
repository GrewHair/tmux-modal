package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// Stopping and resuming a busy pane's output for a control client that is
// behind on reading must not take the server down (F43). Before tmux 3.7,
// `refresh-client -A '%N:off'` kept the output already queued for the
// client while the pane buffer behind it was freed (a second, fast client
// keeps tmux reading the pane, as the human's terminal does); the server
// died within a few toggles. The daemon's own gate (pause/continue) is
// toggled here the same way. TMUX_MODAL_TEST_GATE_OFF=1 uses off/on
// instead, to check that this test catches the crash on tmux < 3.7.
func TestOutputGateSlowReader(t *testing.T) {
	requireTmux(t)
	socketSeq++
	sock := fmt.Sprintf("mg-%d-%d", os.Getpid(), socketSeq)
	run := func(args ...string) error {
		return exec.Command("tmux", append([]string{"-L", sock, "-f", "/dev/null"}, args...)...).Run()
	}
	if err := run("new-session", "-d", "-s", "s", "-x", "200", "-y", "50", "while :; do seq 1 200000; done"); err != nil {
		t.Fatal(err)
	}
	pid := serverPID(sock)
	var clients []*exec.Cmd
	t.Cleanup(func() {
		for _, c := range clients {
			c.Process.Kill()
			c.Wait()
		}
		reapServer(sock, pid)
	})
	attach := func() (io.WriteCloser, io.ReadCloser, *exec.Cmd) {
		cmd := exec.Command("tmux", "-L", sock, "-f", "/dev/null", "-C", "attach-session", "-f", "ignore-size", "-t", "s")
		in, _ := cmd.StdinPipe()
		out, _ := cmd.StdoutPipe()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		clients = append(clients, cmd)
		return in, out, cmd
	}
	gate := func(watched bool) string { return tmux.Command("refresh-client", "-A", tmux.OutputGate("%0", watched)) }
	if os.Getenv("TMUX_MODAL_TEST_GATE_OFF") == "1" {
		gate = func(watched bool) string {
			if watched {
				return "refresh-client -A '%0:on'"
			}
			return "refresh-client -A '%0:off'"
		}
	}

	_, human, _ := attach()
	go io.Copy(io.Discard, human)

	toggles := 0
	for round := 0; round < 3; round++ {
		in, out, cmd := attach()
		var reading atomic.Bool
		go func() {
			buf := make([]byte, 1<<16)
			for {
				if !reading.Load() {
					time.Sleep(2 * time.Millisecond)
					continue
				}
				if _, err := out.Read(buf); err != nil {
					return
				}
			}
		}()
		for i := 0; i < 20; i++ {
			time.Sleep(60 * time.Millisecond) // not reading: output queues up for the client
			io.WriteString(in, gate(false)+"\n")
			time.Sleep(60 * time.Millisecond) // the fast client drains the pane buffer
			reading.Store(true)
			time.Sleep(20 * time.Millisecond)
			reading.Store(false)
			io.WriteString(in, gate(true)+"\n")
			toggles++
		}
		reading.Store(true)
		in.Close()
		cmd.Wait()
		// Let tmux deliver this client's %client-detached before the next
		// control client connects: tmux < 3.7 crashes when a broadcast
		// notification reaches a control client still identifying (F49),
		// a different bug from the one this test is about.
		time.Sleep(300 * time.Millisecond)
		if err := run("has-session", "-t", "s"); err != nil {
			t.Fatalf("the tmux server died after %d output toggles", toggles)
		}
	}
}
