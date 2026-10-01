package daemon

import (
	"io"
	"testing"
	"time"

	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// Control clients attach one at a time, each after 250 ms without a
// notification or an attach of our own (F49); after 2 s of waiting,
// regardless.
func TestAttachQuiet(t *testing.T) {
	var attached []string
	orig := attachControl
	attachControl = func(_ tmux.Server, sid string, _ func(tmux.Notification)) (*tmux.Control, error) {
		attached = append(attached, sid)
		return &tmux.Control{}, nil
	}
	defer func() { attachControl = orig }()

	t0 := time.Now()
	d := &Daemon{log: newLogger(io.Discard, "error"), conns: map[string]*tmux.Control{},
		ev: &events{output: map[string]time.Time{}, wake: make(chan struct{}, 1)}}
	d.ev.lastNote = t0 // a human just attached: a burst of notifications

	step := func(at time.Duration, want ...string) {
		t.Helper()
		var pending []string
		for _, sid := range []string{"$0", "$1"} {
			if _, ok := d.conns[sid]; !ok {
				pending = append(pending, sid)
			}
		}
		attached = nil
		d.attachQuiet(pending, t0.Add(at))
		if len(attached) != len(want) || (len(want) == 1 && attached[0] != want[0]) {
			t.Errorf("at %v: attached %v, want %v", at, attached, want)
		}
		for _, sid := range attached {
			d.conns[sid] = &tmux.Control{}
		}
	}
	step(10 * time.Millisecond) // the burst is still on
	if want := t0.Add(attachQuietFor); !d.reconcileAt.Equal(want) {
		t.Errorf("retry at %v, want %v", d.reconcileAt.Sub(t0), attachQuietFor)
	}
	step(260*time.Millisecond, "$0") // quiet: one
	step(300 * time.Millisecond)     // our own attach is noise too
	step(520*time.Millisecond, "$1") // then the next

	// Notifications that never stop: attach anyway after 2 s.
	delete(d.conns, "$0")
	delete(d.conns, "$1")
	d.lastAttach = time.Time{}
	for ms := 1000; ms < 4000; ms += 100 {
		d.ev.lastNote = t0.Add(time.Duration(ms) * time.Millisecond)
		attached = nil
		d.attachQuiet([]string{"$0"}, d.ev.lastNote.Add(time.Millisecond))
		if len(attached) > 0 {
			if waited := ms - 1000; waited < 2000 {
				t.Errorf("attached after %d ms of noise, want 2 s", waited)
			}
			return
		}
	}
	t.Error("never attached under constant notifications")
}
