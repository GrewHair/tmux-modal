package daemon

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/GrewHair/tmux-modal/internal/spec"
)

// HookEvent is one transition delivered to the transition hook (§8).
type HookEvent struct {
	Event      string // mode | focus | blur | stop
	Key        string // debounce/cancellation key: "focus", "c:<client>" or "p:<pane>"
	Pane       string
	App        string
	AppFrom    string
	ModeTo     string
	ModeFrom   string
	Bucket     string
	Confidence string
	Nested     string
	Session    string
	SessionID  string
	Window     string
	Active     bool
	Client     string   // the tmux client (terminal) typed into, for Active events
	Commands   []string // global hook, then the spec's own
	Merged     int      // transitions this call stands for (coalesced)
}

// hookNote tells the daemon's loop about one hook call, for the hook badge:
// Phase "fired" when it starts, then "ok", "fail" (Code: exit status) or
// "timeout" when it ends. A call superseded by a newer one reports no end.
type hookNote struct {
	ID     uint64
	Pane   string
	Phase  string
	Mode   string
	Code   int
	Merged int
	Active bool
	At     time.Time
}

// Typing is the value of MODAL_TYPING: 0 only when the pane is confidently
// in a commanding mode. A plain shell (none) is line editing, i.e. typing,
// and an unrecognised app (unknown) must not look like commanding, since a
// subscriber acting on a false "commanding" does the harmful thing.
func Typing(bucket string) string {
	if bucket == spec.BucketCommanding {
		return "0"
	}
	return "1"
}

func (e *HookEvent) env(now time.Time) []string {
	active := "0"
	if e.Active {
		active = "1"
	}
	return append(os.Environ(),
		"MODAL_EVENT="+e.Event,
		"MODAL_TYPING="+Typing(e.Bucket),
		"MODAL_BUCKET="+e.Bucket,
		"MODAL_MODE_TO="+e.ModeTo,
		"MODAL_MODE_FROM="+e.ModeFrom,
		"MODAL_CONFIDENCE="+e.Confidence,
		"MODAL_NESTED="+e.Nested,
		"MODAL_PANE="+e.Pane,
		"MODAL_APP="+e.App,
		"MODAL_APP_FROM="+e.AppFrom,
		"MODAL_SESSION="+e.Session,
		"MODAL_SESSION_ID="+e.SessionID,
		"MODAL_WINDOW="+e.Window,
		"MODAL_PANE_ACTIVE="+active,
		"MODAL_CLIENT="+e.Client,
		"MODAL_TIMESTAMP_MS="+strconv.FormatInt(now.UnixMilli(), 10),
	)
}

// hookRunner runs hooks detached from the daemon's loop. Per key it runs
// at most one call per debounce window: a transition after a quiet spell
// runs at once (the latency an outer keyboard layer feels), later ones in
// the window coalesce into one call with the final state at its end. It
// keeps at most one invocation alive: a newer transition cancels an older
// one still running, because a stale transition is worse than a missed one.
type hookRunner struct {
	log *Logger
	// observe, when set, is told about every call (never "stop" events).
	// It is called with h.mu held and must not block.
	observe func(hookNote)

	mu       sync.Mutex
	timeout  time.Duration
	debounce time.Duration
	pending  map[string]*HookEvent
	timers   map[string]*time.Timer
	last     map[string]time.Time // last call started, per key
	running  map[string]runningHook
	gen      uint64
	wg       sync.WaitGroup
}

type runningHook struct {
	id     uint64
	cancel context.CancelFunc
}

func newHookRunner(log *Logger) *hookRunner {
	return &hookRunner{log: log, pending: map[string]*HookEvent{},
		timers: map[string]*time.Timer{}, last: map[string]time.Time{}, running: map[string]runningHook{}}
}

func (h *hookRunner) configure(timeout, debounce time.Duration) {
	h.mu.Lock()
	h.timeout, h.debounce = timeout, debounce
	h.mu.Unlock()
}

// Emit runs an event now if the key's last call is a debounce window ago,
// else schedules it for the end of the window, replacing any event still
// pending for the same key.
func (h *hookRunner) Emit(e *HookEvent) {
	if len(e.Commands) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	e.Merged = 1
	if prev, ok := h.pending[e.Key]; ok {
		// Coalesce: the subscriber never saw prev, so the transition it
		// needs is from what it last saw to the newest state.
		e.ModeFrom, e.AppFrom = prev.ModeFrom, prev.AppFrom
		e.Merged = prev.Merged + 1
	}
	h.pending[e.Key] = e
	if _, ok := h.timers[e.Key]; ok {
		return
	}
	key := e.Key
	if wait := h.debounce - time.Since(h.last[key]); wait > 0 {
		h.timers[key] = time.AfterFunc(wait, func() { h.fire(key) })
		return
	}
	h.start(key)
}

func (h *hookRunner) fire(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.start(key)
}

// start runs the pending event for key; h.mu is held.
func (h *hookRunner) start(key string) {
	e := h.pending[key]
	delete(h.pending, key)
	delete(h.timers, key)
	if e == nil {
		return
	}
	h.last[key] = time.Now()
	if old, ok := h.running[key]; ok {
		old.cancel() // drop the older invocation, run the newer
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout*time.Duration(len(e.Commands)))
	h.gen++
	id := h.gen
	h.running[key] = runningHook{id: id, cancel: cancel}
	timeout := h.timeout
	note := func(phase string, code int) {
		if h.observe != nil && e.Event != "stop" {
			h.observe(hookNote{ID: id, Pane: e.Pane, Phase: phase, Mode: e.ModeTo, Code: code, Merged: e.Merged, Active: e.Active, At: time.Now()})
		}
	}
	note("fired", 0)
	h.wg.Add(1)

	go func() {
		defer h.wg.Done()
		defer func() {
			h.mu.Lock()
			if r, ok := h.running[key]; ok && r.id == id {
				delete(h.running, key)
			}
			h.mu.Unlock()
			cancel()
		}()
		env := e.env(time.Now())
		phase, code := "ok", 0
		for _, cmd := range e.Commands {
			if ctx.Err() != nil {
				return
			}
			p, c := h.runOne(ctx, cmd, env, timeout)
			if p == "cancelled" {
				return // superseded: the newer call reports
			}
			if phase == "ok" && p != "ok" {
				phase, code = p, c
			}
		}
		h.mu.Lock()
		note(phase, code)
		h.mu.Unlock()
	}()
}

// runOne runs one hook command: "ok", "fail" with its exit status,
// "timeout", or "cancelled" (superseded by a newer call).
func (h *hookRunner) runOne(ctx context.Context, command string, env []string, timeout time.Duration) (string, int) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	if err := cmd.Start(); err != nil {
		h.log.Warnf("hook: %v", err)
		return "fail", -1
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			h.log.Warnf("hook failed (%v) after %v: %s: %.300s", err, time.Since(start).Round(time.Millisecond), command, out.String())
			code := -1
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
			return "fail", code
		}
		h.log.Debugf("hook ok in %v", time.Since(start).Round(time.Millisecond))
		return "ok", 0
	case <-cctx.Done():
		// Kill the whole process group: the hook may have spawned
		// children that would otherwise outlive it.
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		if ctx.Err() == context.Canceled {
			h.log.Debugf("hook superseded by a newer transition: %s", command)
			return "cancelled", 0
		}
		h.log.Warnf("hook timed out after %v: %s", timeout, command)
		return "timeout", 0
	}
}

// Flush fires every pending event now and waits for all hooks (used at
// shutdown, so the final "stop" reaches subscribers).
func (h *hookRunner) Flush(wait time.Duration) {
	h.mu.Lock()
	var keys []string
	for k, t := range h.timers {
		t.Stop()
		keys = append(keys, k)
	}
	h.mu.Unlock()
	for _, k := range keys {
		h.fire(k)
	}
	done := make(chan struct{})
	go func() { h.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(wait):
	}
}
