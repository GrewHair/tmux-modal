package integration

import (
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
