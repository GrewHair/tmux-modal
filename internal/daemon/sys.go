package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lock takes a per-server exclusive lock so only one daemon runs per tmux
// server; the returned function releases it.
func lock(id string) (func(), error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, fmt.Sprintf("tmux-modal-%d", os.Getuid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, id+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("already running for this server")
	}
	return func() { f.Close() }, nil
}

type cpuSample struct {
	wall time.Time
	cpu  time.Duration
}

func processCPU() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	tv := func(t syscall.Timeval) time.Duration {
		return time.Duration(t.Sec)*time.Second + time.Duration(t.Usec)*time.Microsecond
	}
	return tv(ru.Utime) + tv(ru.Stime)
}

// checkCPU measures the daemon's own CPU share since the last check and
// stretches the burst and poll intervals to stay inside @modal_cpu_budget.
func (d *Daemon) checkCPU() {
	now, cpu := time.Now(), processCPU()
	prev := d.cpu
	d.cpu = cpuSample{wall: now, cpu: cpu}
	if prev.wall.IsZero() {
		return
	}
	wall := now.Sub(prev.wall)
	if wall < 500*time.Millisecond {
		return
	}
	pct := 100 * float64(cpu-prev.cpu) / float64(wall)
	old := d.throttle
	switch {
	case pct > d.cfg.CPUBudget:
		d.throttle *= 1.5
		if d.throttle > 16 {
			d.throttle = 16
		}
	case pct < d.cfg.CPUBudget/2 && d.throttle > 1:
		d.throttle /= 1.5
		if d.throttle < 1 {
			d.throttle = 1
		}
	}
	if d.throttle != old {
		d.log.Warnf("cpu %.2f%% (budget %.2f%%): interval multiplier %.2f -> %.2f", pct, d.cfg.CPUBudget, old, d.throttle)
	}
	d.lastCPU = pct
}
