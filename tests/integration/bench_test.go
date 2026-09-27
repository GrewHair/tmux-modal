package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestBenchmarkCPU measures what the daemon costs with 5 and 20 panes of
// htop (a TUI redrawing every 1.5 s) per profile, for the default scope
// (only the focused pane is examined) and for scope "all": the daemon's own
// CPU, and the extra CPU the tmux server spends serving it (captures,
// list-panes, option writes), against a baseline without the daemon.
//
// Slow (about five minutes), so it runs only with TMUX_MODAL_BENCH=1:
//
//	TMUX_MODAL_BENCH=1 go test ./tests/integration/ -run TestBenchmarkCPU -v -timeout 30m
//
// Then one vim pane typed into at 10 keys a second, leaving and entering
// insert every two seconds (skipped without a local vim): every keystroke
// redraws, so the pane is examined at the burst cadence throughout.
func TestBenchmarkCPU(t *testing.T) {
	if os.Getenv("TMUX_MODAL_BENCH") == "" {
		t.Skip("set TMUX_MODAL_BENCH=1 to run the CPU benchmark")
	}
	window := 20 * time.Second
	if s := os.Getenv("TMUX_MODAL_BENCH_SECONDS"); s != "" {
		n, _ := strconv.Atoi(s)
		window = time.Duration(n) * time.Second
	}
	var report []string
	for _, panes := range []int{5, 20} {
		for _, scope := range []string{"active", "all"} {
			for _, profile := range []string{"frugal", "balanced", "snappy"} {
				name := fmt.Sprintf("%d-panes/%s/%s", panes, scope, profile)
				t.Run(name, func(t *testing.T) {
					d, s, base := benchOnce(t, panes, scope, profile, window)
					line := fmt.Sprintf("%2d panes  scope=%-6s  %-8s  daemon %5.2f%%  tmux server +%5.2f%% (baseline %.2f%%)",
						panes, scope, profile, d, s-base, base)
					t.Log(line)
					report = append(report, line)
				})
			}
		}
	}
	if vim, err := exec.LookPath("vim"); err == nil {
		for _, profile := range []string{"frugal", "balanced", "snappy"} {
			t.Run("vim-typing/"+profile, func(t *testing.T) {
				d, s, base := benchVim(t, vim, profile, window)
				line := fmt.Sprintf("vim typed into  %-8s  daemon %5.2f%%  tmux server +%5.2f%% (baseline %.2f%%)",
					profile, d, s-base, base)
				t.Log(line)
				report = append(report, line)
			})
		}
	}
	t.Logf("CPU, %% of one core, %v per measurement:\n%s", window, strings.Join(report, "\n"))
}

// benchVim: vim with its defaults, typed into through the attached client
// for the baseline and again with the daemon running.
func benchVim(t *testing.T, vim, profile string, window time.Duration) (daemon, server, baseline float64) {
	f := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(f, []byte("alpha\n"), 0o644)
	h := newHarness(t, opts{w: 120, h: 40, cmd: []string{vim, "-u", "DEFAULTS", "-i", "NONE", "-n", f},
		options: map[string]string{"@modal_profile": profile, "@modal_log_level": "warn"}})
	pid, err := strconv.Atoi(h.tmuxIn("display-message", "-p", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	// Twenty keys every two seconds: "o", eighteen letters, Escape.
	keys := append(append([]string{"o"}, strings.Split("the quick brown fo", "")...), "Escape")
	measure := func(pids ...int) []float64 {
		start := make([]time.Duration, len(pids))
		for i, p := range pids {
			start[i] = procCPU(p)
		}
		t0 := time.Now()
		for i := 0; time.Since(t0) < window; i++ {
			h.typeKeys(keys[i%len(keys)])
			time.Sleep(100 * time.Millisecond)
		}
		el := time.Since(t0)
		out := make([]float64, len(pids))
		for i, p := range pids {
			out[i] = 100 * float64(procCPU(p)-start[i]) / float64(el)
		}
		return out
	}
	baseline = measure(pid)[0]
	h.typeKeys("Escape")
	h.startDaemon()
	h.expectState("main", "vim/normal/commanding", "root")
	r := measure(h.daemon.Process.Pid, pid)
	return r[0], r[1], baseline
}

func benchOnce(t *testing.T, panes int, scope, profile string, window time.Duration) (daemon, server, baseline float64) {
	h := newHarness(t, opts{w: 200, h: 60, cmd: []string{"env", "HTOPRC=/dev/null", "htop"},
		options: map[string]string{"@modal_profile": profile, "@modal_scope": scope, "@modal_log_level": "warn"}})
	for i := 1; i < panes; i++ {
		h.tmuxIn("split-window", "-d", "-t", "main", "env", "HTOPRC=/dev/null", "htop")
		h.tmuxIn("select-layout", "-t", "main", "tiled")
	}
	pid, err := strconv.Atoi(h.tmuxIn("display-message", "-p", "#{pid}"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second) // let every htop draw its first screen
	measure := func(pids ...int) []float64 {
		start := make([]time.Duration, len(pids))
		for i, p := range pids {
			start[i] = procCPU(p)
		}
		t0 := time.Now()
		time.Sleep(window)
		el := time.Since(t0)
		out := make([]float64, len(pids))
		for i, p := range pids {
			out[i] = 100 * float64(procCPU(p)-start[i]) / float64(el)
		}
		return out
	}
	baseline = measure(pid)[0]
	h.startDaemon()
	h.expectState("main", "htop/normal/commanding", "modal-htop")
	time.Sleep(2 * time.Second)
	r := measure(h.daemon.Process.Pid, pid)
	return r[0], r[1], baseline
}
