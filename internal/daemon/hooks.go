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
	Event      string // mode | focus | stop
	Key        string // debounce/cancellation key: "s:<session>" or "p:<pane>"
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
	Commands   []string // global hook, then the spec's own
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
		"MODAL_TIMESTAMP_MS="+strconv.FormatInt(now.UnixMilli(), 10),
	)
}

// hookRunner runs hooks detached from the daemon's loop. Per key it
// debounces (emitting only the final state of a burst) and keeps at most
// one invocation alive: a newer transition cancels an older one still
// running, because a stale transition is worse than a missed one.
type hookRunner struct {
	log *Logger

	mu       sync.Mutex
	timeout  time.Duration
	debounce time.Duration
	pending  map[string]*HookEvent
	timers   map[string]*time.Timer
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
		timers: map[string]*time.Timer{}, running: map[string]runningHook{}}
}

func (h *hookRunner) configure(timeout, debounce time.Duration) {
	h.mu.Lock()
	h.timeout, h.debounce = timeout, debounce
	h.mu.Unlock()
}

// Emit schedules an event after the debounce window, replacing any event
// still pending for the same key.
func (h *hookRunner) Emit(e *HookEvent) {
	if len(e.Commands) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if prev, ok := h.pending[e.Key]; ok {
		// Coalesce: the subscriber never saw prev, so the transition it
		// needs is from what it last saw to the newest state.
		e.ModeFrom, e.AppFrom = prev.ModeFrom, prev.AppFrom
	}
	h.pending[e.Key] = e
	if _, ok := h.timers[e.Key]; ok {
		return
	}
	key := e.Key
	h.timers[key] = time.AfterFunc(h.debounce, func() { h.fire(key) })
}

func (h *hookRunner) fire(key string) {
	h.mu.Lock()
	e := h.pending[key]
	delete(h.pending, key)
	delete(h.timers, key)
	if e == nil {
		h.mu.Unlock()
		return
	}
	if old, ok := h.running[key]; ok {
		old.cancel() // drop the older invocation, run the newer
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout*time.Duration(len(e.Commands)))
	h.gen++
	id := h.gen
	h.running[key] = runningHook{id: id, cancel: cancel}
	timeout := h.timeout
	h.wg.Add(1)
	h.mu.Unlock()

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
		for _, cmd := range e.Commands {
			if ctx.Err() != nil {
				return
			}
			h.runOne(ctx, cmd, env, timeout)
		}
	}()
}

func (h *hookRunner) runOne(ctx context.Context, command string, env []string, timeout time.Duration) {
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
		return
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			h.log.Warnf("hook failed (%v) after %v: %s: %.300s", err, time.Since(start).Round(time.Millisecond), command, out.String())
		} else {
			h.log.Debugf("hook ok in %v", time.Since(start).Round(time.Millisecond))
		}
	case <-cctx.Done():
		// Kill the whole process group: the hook may have spawned
		// children that would otherwise outlive it.
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		if ctx.Err() == context.Canceled {
			h.log.Debugf("hook superseded by a newer transition: %s", command)
		} else {
			h.log.Warnf("hook timed out after %v: %s", timeout, command)
		}
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
