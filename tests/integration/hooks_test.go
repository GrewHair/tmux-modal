package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hookLog(t *testing.T) (string, string) {
	p := filepath.Join(t.TempDir(), "hook.log")
	// Set via argv, so tmux stores '$' that it later prints as '\$'.
	return p, `echo "$MODAL_EVENT $MODAL_PANE $MODAL_APP $MODAL_MODE_FROM>$MODAL_MODE_TO typing=$MODAL_TYPING active=$MODAL_PANE_ACTIVE" >> ` + p
}

func (h *harness) expectHookLine(path, want string) {
	h.t.Helper()
	h.waitFor("hook line "+want, 5*time.Second, func() bool {
		b, _ := os.ReadFile(path)
		return strings.Contains(string(b), want)
	})
}

// The focused pane's stream: initial focus, mode changes, focus moving to a
// shell (typing=1: line editing), and back.
func TestTransitionHook(t *testing.T) {
	log, hook := hookLog(t)
	h := newHarness(t, opts{cmd: []string{keyecho}, options: map[string]string{"@modal_transition_hook": hook}})
	h.tmuxIn("split-window", "-d", "-t", "main", "bash --norc --noprofile")
	h.startDaemon()
	h.expectHookLine(log, "focus %0 keyecho >normal typing=0 active=1")

	h.typeKeys("/")
	h.expectHookLine(log, "mode %0 keyecho normal>insert typing=1 active=1")
	h.typeKeys("Escape")
	h.expectHookLine(log, "mode %0 keyecho insert>normal typing=0 active=1")

	h.tmuxIn("select-pane", "-t", "%1")
	h.expectHookLine(log, "focus %1  normal>none typing=1 active=1")
	h.tmuxIn("select-pane", "-t", "%0")
	h.expectHookLine(log, "focus %0 keyecho none>normal typing=0 active=1")

	h.stopDaemon()
	h.expectHookLine(log, "stop %0  normal>none typing=1 active=1")
}

// The per-spec hook runs after the global one.
func TestSpecHookAfterGlobal(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "order.log")
	spec, _ := os.ReadFile(filepath.Join(specDir, "keyhook.toml"))
	// Top-level key, so it must precede the first table header.
	os.WriteFile(filepath.Join(dir, "keyhook.toml"),
		append([]byte("hook = 'echo spec-$MODAL_MODE_TO >> "+log+"'\n"), spec...), 0o644)
	h := newHarness(t, opts{cmd: []string{keyecho}, env: []string{"KEYECHO_NAME=KEYHOOK"},
		options: map[string]string{"@modal_transition_hook": "echo global-$MODAL_MODE_TO >> " + log}})
	h.tmuxIn("set-option", "-g", "@modal_spec_paths", dir)
	h.startDaemon()
	h.expectHookLine(log, "global-normal\nspec-normal")
}

// The indicator is rendered in real pane borders: a mode tag for known
// apps, N/A for an unrecognised full-screen app, nothing for a shell.
func TestIndicatorInBorders(t *testing.T) {
	h := newHarness(t, opts{cmd: []string{keyecho}, options: map[string]string{
		"pane-border-status": "top",
		"pane-border-format": "<#{pane_index}:#{@modal_indicator}>",
		// plain text templates keep the rendered border easy to match
		"@modal_indicator_commanding": "[{MODE}]",
		"@modal_indicator_typing":     "[{MODE}:{app}]",
		"@modal_indicator_unknown":    "[N/A]",
		"@modal_scope":                "visible",
	}})
	h.tmuxIn("split-window", "-d", "-h", "-t", "main", "bash --norc --noprofile")
	// keyecho under another name: full screen, but no spec matches it.
	h.tmuxIn("split-window", "-d", "-h", "-t", "main", "-e", "KEYECHO_NAME=NOSPEC", keyecho)
	h.startDaemon()
	border := func(want string) {
		h.t.Helper()
		h.waitFor("border "+want, 5*time.Second, func() bool {
			out, _ := run(h.outer, "capture-pane", "-p", "-t", "%0")
			return strings.Contains(out, want)
		})
	}
	border("<0:[NORMAL]>")
	border("<1:[N/A]>")
	border("<2:>")
	h.typeKeys("/")
	border("<0:[INSERT:keyecho]>")
}

// Two terminals (outer panes attached to sessions main and two) and
// their focus reports (ESC [ I / ESC [ O written into the inner client):
// the hook follows the focused terminal, replays its pane when it
// regains focus, reports the other terminal's changes as not active, and
// reports blur only when asked to (D39).
func TestHookFollowsTerminalFocus(t *testing.T) {
	log, hook := hookLog(t)
	h := newHarness(t, opts{cmd: []string{keyecho}, options: map[string]string{
		"@modal_transition_hook": hook, "focus-events": "on"}})
	h.tmuxIn("new-session", "-d", "-s", "two", keyecho)
	h.tmuxOut("new-window", "-d", fmt.Sprintf("tmux -L %s attach -t two", h.inner))
	h.waitFor("second client attached", 5*time.Second, func() bool {
		return strings.Count(h.tmuxIn("list-clients", "-F", "#{client_control_mode}"), "0") == 2
	})
	focus := func(outerPane string, in bool) {
		b := "4f"
		if in {
			b = "49"
		}
		h.tmuxOut("send-keys", "-t", outerPane, "-H", "1b", "5b", b)
	}
	step := func(what string, act func(), want ...string) {
		t.Helper()
		os.WriteFile(log, nil, 0o600)
		act()
		for _, w := range want {
			h.expectHookLine(log, w)
		}
	}
	h.startDaemon()
	// Both clients start out focused (tmux's default): not known yet.
	h.expectHookLine(log, "focus %0 keyecho >normal typing=0 active=1")
	h.expectHookLine(log, "focus %1 keyecho >normal typing=0 active=1")
	h.waitFor("focus hooks installed", 5*time.Second, func() bool {
		return strings.Contains(h.tmuxIn("show-hooks", "-g", "client-focus-in"), "[7171]")
	})

	step("second terminal loses focus", func() { focus("%1", false) },
		"focus %0 keyecho normal>normal typing=0 active=1")
	step("switch terminals", func() { focus("%0", false); focus("%1", true) },
		"focus %1 keyecho normal>normal typing=0 active=1")
	step("mode change in the other terminal", func() { h.typeKeys("/") },
		"mode %0 keyecho normal>insert typing=1 active=0")
	step("away and back", func() {
		focus("%1", false)
		time.Sleep(300 * time.Millisecond)
		focus("%1", true)
	}, "focus %1 keyecho normal>normal typing=0 active=1")
	if b, _ := os.ReadFile(log); strings.Contains(string(b), "blur") {
		t.Errorf("blur reported with @modal_hook_blur off:\n%s", b)
	}

	h.tmuxIn("set-option", "-g", "@modal_hook_blur", "on")
	time.Sleep(2500 * time.Millisecond) // options are re-read at the idle check
	step("blur", func() { focus("%1", false) }, "blur %1  normal>none typing=1 active=1")

	h.stopDaemon()
	if out := h.tmuxIn("show-hooks", "-g", "client-focus-in"); strings.Contains(out, "[7171]") {
		t.Errorf("focus hook left behind after stop: %s", out)
	}
}
