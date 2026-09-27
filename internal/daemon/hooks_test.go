package daemon

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRunner(t *testing.T, debounce, timeout time.Duration) (*hookRunner, string) {
	t.Helper()
	h := newHookRunner(newLogger(io.Discard, "error"))
	h.configure(timeout, debounce)
	return h, filepath.Join(t.TempDir(), "out")
}

func readLines(t *testing.T, path string) []string {
	b, _ := os.ReadFile(path)
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestHookEnvAndOrder(t *testing.T) {
	h, out := testRunner(t, 5*time.Millisecond, time.Second)
	h.Emit(&HookEvent{Event: "focus", Key: "s:$0", Pane: "%1", App: "htop", ModeTo: "insert", ModeFrom: "normal",
		Bucket: "typing", Confidence: "high", Session: "main", Active: true,
		Commands: []string{
			`echo "global $MODAL_EVENT $MODAL_TYPING $MODAL_MODE_FROM>$MODAL_MODE_TO $MODAL_APP $MODAL_PANE $MODAL_PANE_ACTIVE $MODAL_SESSION" >> ` + out,
			`echo "spec $MODAL_BUCKET" >> ` + out,
		}})
	h.Flush(2 * time.Second)
	got := readLines(t, out)
	want := []string{"global focus 1 normal>insert htop %1 1 main", "spec typing"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The first transition runs at once; the rest of a burst inside the
// debounce window yields one call with the final state, reported as a
// transition from what the subscriber last saw.
func TestHookDebounceCoalesces(t *testing.T) {
	h, out := testRunner(t, 60*time.Millisecond, time.Second)
	cmd := []string{`echo "$MODAL_MODE_FROM>$MODAL_MODE_TO" >> ` + out}
	h.Emit(&HookEvent{Key: "s:1", ModeFrom: "normal", ModeTo: "insert", Bucket: "typing", Commands: cmd})
	h.Emit(&HookEvent{Key: "s:1", ModeFrom: "insert", ModeTo: "normal", Bucket: "commanding", Commands: cmd})
	h.Emit(&HookEvent{Key: "s:1", ModeFrom: "normal", ModeTo: "visual", Bucket: "commanding", Commands: cmd})
	time.Sleep(300 * time.Millisecond)
	h.Flush(time.Second)
	if got := readLines(t, out); strings.Join(got, "|") != "normal>insert|insert>visual" {
		t.Errorf("got %q, want normal>insert at once, then one coalesced insert>visual", got)
	}
}

// A newer transition kills an older hook still running for the same key,
// including its children.
func TestHookNewerCancelsOlder(t *testing.T) {
	h, out := testRunner(t, time.Millisecond, 5*time.Second)
	h.Emit(&HookEvent{Key: "s:1", ModeTo: "insert", Commands: []string{`sleep 2; echo old >> ` + out}})
	time.Sleep(100 * time.Millisecond)
	h.Emit(&HookEvent{Key: "s:1", ModeTo: "normal", Commands: []string{`echo new >> ` + out}})
	time.Sleep(2500 * time.Millisecond)
	h.Flush(time.Second)
	if got := readLines(t, out); strings.Join(got, "|") != "new" {
		t.Errorf("got %q, want only the newer hook", got)
	}
}

func TestHookTimeoutKillsGroup(t *testing.T) {
	h, out := testRunner(t, time.Millisecond, 200*time.Millisecond)
	start := time.Now()
	h.Emit(&HookEvent{Key: "p:%1", Commands: []string{`(sleep 3; echo leaked >> ` + out + `) & sleep 3`}})
	time.Sleep(20 * time.Millisecond)
	h.Flush(2 * time.Second)
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("timeout did not stop the hook: %v", d)
	}
	time.Sleep(3200 * time.Millisecond)
	if got := readLines(t, out); len(got) != 0 {
		t.Errorf("child of a timed-out hook survived: %q", got)
	}
}

func TestTyping(t *testing.T) {
	for bucket, want := range map[string]string{"typing": "1", "commanding": "0", "none": "1", "unknown": "1"} {
		if got := Typing(bucket); got != want {
			t.Errorf("Typing(%s) = %s, want %s", bucket, got, want)
		}
	}
}

func TestUnescapeOption(t *testing.T) {
	cases := map[string]string{
		`echo \$MODAL_TYPING`: `echo $MODAL_TYPING`,
		`x \\$HOME`:           `x \$HOME`, // a literal \$ in the stored value
		`p \\ q`:              `p \\ q`,
		`#[fg=green]{MODE}`:   `#[fg=green]{MODE}`,
	}
	for in, want := range cases {
		if got := unescapeOption(in); got != want {
			t.Errorf("unescapeOption(%q) = %q, want %q", in, got, want)
		}
	}
}
