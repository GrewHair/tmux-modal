package daemon

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/GrewHair/tmux-modal/internal/spec"
)

// A spec's poll_interval replaces @modal_poll_interval for panes running
// that app, and the CPU throttle still scales it.
func TestPollForSpecOverride(t *testing.T) {
	fsys := fstest.MapFS{"slow.toml": &fstest.MapFile{Data: []byte(`
poll_interval = 900
modes = ["normal"]
[buckets]
commanding = ["normal"]
[match]
command = ["slow"]
`)}}
	set := spec.Load([]spec.Source{{FS: fsys, Dir: ".", Label: "test"}})
	d := &Daemon{set: set, cfg: Config{Poll: 150 * time.Millisecond}, throttle: 1}
	if got := d.pollFor(&paneState{app: "slow"}); got != 900*time.Millisecond {
		t.Errorf("spec pane: %v, want 900ms", got)
	}
	if got := d.pollFor(&paneState{}); got != 150*time.Millisecond {
		t.Errorf("other pane: %v, want the global 150ms", got)
	}
	d.throttle = 2
	if got := d.pollFor(&paneState{app: "slow"}); got != 1800*time.Millisecond {
		t.Errorf("throttled: %v, want 1.8s", got)
	}
}
