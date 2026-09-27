package integration

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDetectionLatency measures keypress -> published mode change through
// a real client, both directions, and reports the daemon's CPU use. It
// asserts only a loose bound; run with -v to see the numbers.
func TestDetectionLatency(t *testing.T) {
	for _, profile := range []string{"balanced", "snappy"} {
		t.Run(profile, func(t *testing.T) {
			h := newHarness(t, opts{cmd: []string{keyecho}, options: map[string]string{
				"@modal_profile": profile, "@modal_log_level": "warn"}})
			h.startDaemon()
			h.expectState("main", "keyecho/normal/commanding", "modal-keyecho")

			poll := func(want string) time.Duration {
				start := time.Now()
				for time.Since(start) < 3*time.Second {
					if h.state("main") == want {
						return time.Since(start)
					}
					time.Sleep(2 * time.Millisecond)
				}
				t.Fatalf("no transition to %s", want)
				return 0
			}
			cpu0 := procCPU(h.daemon.Process.Pid)
			wall0 := time.Now()
			var enter, leave []time.Duration
			for i := 0; i < 15; i++ {
				h.typeKeys("/")
				enter = append(enter, poll("keyecho/insert/typing"))
				h.typeKeys("Escape")
				leave = append(leave, poll("keyecho/normal/commanding"))
				time.Sleep(50 * time.Millisecond)
			}
			cpuBusy := 100 * float64(procCPU(h.daemon.Process.Pid)-cpu0) / float64(time.Since(wall0))
			cpu1 := procCPU(h.daemon.Process.Pid)
			time.Sleep(3 * time.Second)
			cpuIdle := 100 * float64(procCPU(h.daemon.Process.Pid)-cpu1) / float64(3*time.Second)

			t.Logf("%s: enter typing p50=%v p90=%v max=%v | leave to commanding (needs %s confirmation) p50=%v p90=%v max=%v",
				profile, pct(enter, 50), pct(enter, 90), pct(enter, 100), "2-capture",
				pct(leave, 50), pct(leave, 90), pct(leave, 100))
			t.Logf("%s: daemon CPU while toggling %.2f%%, idle %.3f%% of one core (1 pane)", profile, cpuBusy, cpuIdle)
			if pct(enter, 90) > time.Second || pct(leave, 90) > time.Second {
				t.Errorf("detection far too slow")
			}
		})
	}
}

func pct(ds []time.Duration, p int) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := (len(s) - 1) * p / 100
	return s[i].Round(time.Millisecond)
}

// procCPU returns user+system CPU time of a process from /proc (Linux).
func procCPU(pid int) time.Duration {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	f := strings.Fields(s[strings.LastIndexByte(s, ')')+2:])
	if len(f) < 13 {
		return 0
	}
	ut, _ := strconv.ParseInt(f[11], 10, 64)
	st, _ := strconv.ParseInt(f[12], 10, 64)
	return time.Duration(ut+st) * (time.Second / 100) // USER_HZ = 100
}
