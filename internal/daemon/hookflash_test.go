package daemon

import (
	"io"
	"sync"
	"testing"
	"time"
)

// The runner tells the loop when a call starts and how it ended.
func TestHookNotes(t *testing.T) {
	h := newHookRunner(newLogger(io.Discard, "error"))
	h.configure(300*time.Millisecond, 0)
	var mu sync.Mutex
	got := map[string][]hookNote{}
	h.observe = func(n hookNote) {
		mu.Lock()
		got[n.Pane] = append(got[n.Pane], n)
		mu.Unlock()
	}
	for pane, cmd := range map[string]string{"%1": "true", "%2": "exit 3", "%3": "sleep 2"} {
		h.Emit(&HookEvent{Event: "mode", Key: "p:" + pane, Pane: pane, ModeTo: "insert", Commands: []string{cmd}})
	}
	h.Emit(&HookEvent{Event: "stop", Key: "s:x", Pane: "%9", Commands: []string{"true"}})
	h.Flush(3 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	want := map[string][2]string{"%1": {"fired", "ok"}, "%2": {"fired", "fail"}, "%3": {"fired", "timeout"}}
	for pane, w := range want {
		ns := got[pane]
		if len(ns) != 2 || ns[0].Phase != w[0] || ns[1].Phase != w[1] || ns[0].ID != ns[1].ID || ns[0].Mode != "insert" {
			t.Errorf("%s: notes %+v, want %v", pane, ns, w)
		}
	}
	if ns := got["%2"]; len(ns) == 2 && ns[1].Code != 3 {
		t.Errorf("exit status %d, want 3", ns[1].Code)
	}
	if len(got["%9"]) != 0 {
		t.Errorf("a stop event flashed: %+v", got["%9"])
	}
}

// The flash: a call replaces what showed, its result replaces "fired"
// only while the badge still shows, and everything is gone after the
// flash time counted from the call.
func TestHookFlash(t *testing.T) {
	t0 := time.Now()
	var f hookFlash
	f.note(hookNote{ID: 1, Phase: "fired", Mode: "insert", Merged: 3, At: t0}, 1500*time.Millisecond)
	f.note(hookNote{ID: 1, Phase: "ok", At: t0.Add(40 * time.Millisecond)}, 1500*time.Millisecond)
	if f.phase != "ok" || f.merged != 3 {
		t.Fatalf("after the result: %+v", f)
	}
	tpl := parseConfig(map[string]string{}).Badges
	bs := renderBadges(tpl, modeState{Mode: "none", Bucket: "none"}, detail{Hook: f}, "_")
	if got := plain(joinBadges(bs)); got != "HOOK ✓ insert ×3" {
		t.Errorf("badge %q", got)
	}
	f.expire(t0.Add(1499 * time.Millisecond))
	if f.phase == "" {
		t.Error("gone too early")
	}
	f.expire(t0.Add(1500 * time.Millisecond))
	if f.phase != "" {
		t.Errorf("still showing after the flash: %+v", f)
	}

	// A late result (after the flash) does not bring it back; a result of
	// an older call does not overwrite a newer one.
	f.note(hookNote{ID: 1, Phase: "fail", Code: 2, At: t0.Add(2 * time.Second)}, 1500*time.Millisecond)
	if f.phase != "" {
		t.Errorf("a late result came back: %+v", f)
	}
	f.note(hookNote{ID: 2, Phase: "fired", At: t0}, time.Second)
	f.note(hookNote{ID: 1, Phase: "fail", At: t0}, time.Second)
	if f.phase != "fired" {
		t.Errorf("an older call's result overwrote a newer call: %+v", f)
	}
	f.note(hookNote{ID: 2, Phase: "timeout", At: t0}, time.Second)
	bs = renderBadges(tpl, modeState{Mode: "none", Bucket: "none"}, detail{Hook: f}, "_")
	if got := plain(joinBadges(bs)); got != "HOOK ⏱" {
		t.Errorf("timeout badge %q", got)
	}
	f.note(hookNote{ID: 3, Phase: "fired", At: t0}, 0)
	if f.phase != "" {
		t.Errorf("@modal_hook_flash 0 still shows: %+v", f)
	}
}
