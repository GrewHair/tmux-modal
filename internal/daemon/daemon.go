// Package daemon is the per-server detector: scheduler, gate,
// fingerprinter, state store and dispatcher (§4).
package daemon

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GrewHair/tmux-modal/internal/spec"
	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// Daemon is one detector for one tmux server.
type Daemon struct {
	srv     tmux.Server
	exec    tmux.Exec
	log     *Logger
	bundled spec.Source
	set     *spec.Set
	specKey string
	cfg     Config

	ev    *events
	conns map[string]*tmux.Control // session id -> control client

	panes     map[string]*paneState
	sessions  map[string]*sessionState
	clients   []clientInfo
	installed map[string]string // app -> hash of the root table it copied

	throttle       float64
	cpu            cpuSample
	pidOpt         string
	forceReconcile bool
	lastCPU        float64

	hooks        *hookRunner
	focusEmitted map[string]focusState // session id -> last focus report
	statusDirty  bool
	indicatorKey string
}

// events collects notifications from the control clients' reader
// goroutines. Output is coalesced to "pane X had output at T": the daemon
// needs the edge, never the bytes.
type events struct {
	mu       sync.Mutex
	output   map[string]time.Time
	notes    []tmux.Notification
	overflow bool
	wake     chan struct{}
}

func (e *events) onNote(n tmux.Notification) {
	e.mu.Lock()
	switch n.Name {
	case "output", "extended-output":
		if len(n.Args) > 0 {
			e.output[n.Args[0]] = time.Now()
		}
	case "begin", "end", "error":
	default:
		if len(e.notes) < 256 {
			e.notes = append(e.notes, n)
		} else {
			e.overflow = true
		}
	}
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *events) take() (map[string]time.Time, []tmux.Notification, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out, notes, of := e.output, e.notes, e.overflow
	e.output, e.notes, e.overflow = map[string]time.Time{}, nil, false
	return out, notes, of
}

type clientInfo struct {
	Name      string
	SessionID string
	Control   bool
	KeyTable  string
}

// Main runs the daemon (the `daemon` subcommand).
func Main(args []string, bundled spec.Source) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	socket := fs.String("socket", "", "tmux server socket (default: from $TMUX)")
	label := fs.String("L", "", "tmux server socket name (-L)")
	logPath := fs.String("log", "", "log file (default: state dir; - for stderr)")
	fs.Parse(args)

	srv := tmux.FromEnv()
	if *socket != "" {
		srv.Socket = *socket
	}
	if *label != "" {
		srv.Socket, srv.Label = "", *label
	}
	if srv.Socket == "" && srv.Label == "" {
		return errors.New("not inside tmux: pass --socket or -L, or run from modal.tmux")
	}
	id := serverKey(srv)
	unlock, err := lock(id)
	if err != nil {
		return err
	}
	defer unlock()

	if *logPath == "" {
		*logPath = filepath.Join(stateDir(), id+".log")
	}
	d := &Daemon{
		srv:       srv,
		exec:      tmux.Exec{Server: srv},
		log:       newLogger(openLog(*logPath), "warn"),
		bundled:   bundled,
		ev:        &events{output: map[string]time.Time{}, wake: make(chan struct{}, 1)},
		conns:     map[string]*tmux.Control{},
		panes:     map[string]*paneState{},
		sessions:  map[string]*sessionState{},
		installed: map[string]string{},
		throttle:  1,

		focusEmitted: map[string]focusState{},
	}
	d.hooks = newHookRunner(d.log)
	return d.loop()
}

// serverKey identifies a tmux server for lock and log names.
func serverKey(s tmux.Server) string {
	key := s.Socket
	if key == "" {
		key = "L:" + s.Label
	}
	sum := sha256.Sum256([]byte(key))
	base := filepath.Base(key)
	return fmt.Sprintf("%s-%x", strings.TrimPrefix(base, "L:"), sum[:4])
}

func stateDir() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	dir = filepath.Join(dir, "tmux-modal")
	os.MkdirAll(dir, 0o700)
	return dir
}

func (d *Daemon) loop() error {
	cfg, err := ReadConfig(d.exec)
	if err != nil {
		return fmt.Errorf("cannot reach tmux: %w", err)
	}
	d.applyConfig(cfg)
	d.pidOpt = strconv.Itoa(os.Getpid())
	d.exec.Run("set-option", "-g", "@modal_daemon_pid", d.pidOpt)
	d.recoverKeyTables()
	d.log.Infof("started pid=%d profile=%s burst=%v poll=%v idle=%v scope=%s specs=%d",
		os.Getpid(), d.cfg.Profile, d.cfg.Burst, d.cfg.Poll, d.cfg.Idle, d.cfg.Scope, len(d.set.Order))

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	timer := time.NewTimer(0)
	lastReconcile := time.Time{}

	for {
		now := time.Now()
		if d.forceReconcile || now.Sub(lastReconcile) >= d.cfg.Idle {
			d.forceReconcile = false
			if !d.reconcile() {
				d.log.Infof("tmux server gone; exiting")
				d.closeConns()
				return nil
			}
			lastReconcile = now
		}
		d.cycle()
		resetTimer(timer, d.nextWake(lastReconcile))
		select {
		case s := <-sigs:
			if s == syscall.SIGHUP {
				d.log.Infof("SIGHUP: reloading specs")
				d.specKey = ""
				lastReconcile = time.Time{}
				continue
			}
			d.shutdown()
			return nil
		case <-d.ev.wake:
			d.drainEvents()
		case <-timer.C:
		}
	}
}

func resetTimer(t *time.Timer, dur time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	if dur < 0 {
		dur = 0
	}
	t.Reset(dur)
}

func (d *Daemon) applyConfig(c Config) {
	d.cfg = c
	d.log.SetLevel(c.LogLevel)
	d.hooks.configure(c.HookTimeout, c.HookDebounce)
	if key := fmt.Sprint(c.Indicator); key != d.indicatorKey {
		first := d.indicatorKey == ""
		d.indicatorKey = key
		if !first {
			d.republishIndicators()
		}
	}
	key := strings.Join(c.SpecPaths, ":")
	if key != d.specKey {
		srcs := []spec.Source{d.bundled}
		for _, p := range c.SpecPaths {
			srcs = append(srcs, spec.DirSource(p))
		}
		d.set = spec.Load(srcs)
		d.specKey = key
		d.installed = map[string]string{}
		for _, w := range d.set.Warnings {
			d.log.Warnf("spec: %s", w)
		}
		d.log.Infof("loaded %d specs", len(d.set.Specs))
	}
}

// runner returns the cheapest live command channel: any control client,
// or one tmux process per command when none is attached.
func (d *Daemon) runner() tmux.Runner {
	for _, c := range d.conns {
		select {
		case <-c.Done():
		default:
			return c
		}
	}
	return d.exec
}

// reconcile refreshes config and clients, and keeps exactly one control
// client attached to every session that has a human client. It returns
// false when the server is gone.
func (d *Daemon) reconcile() bool {
	r := d.runner()
	cfg, err := ReadConfig(r)
	if err != nil {
		if _, err2 := ReadConfig(d.exec); err2 != nil {
			return false
		}
		r = d.exec
		cfg, _ = ReadConfig(r)
	}
	d.applyConfig(cfg)
	d.refreshClients(r)
	d.checkCPU()

	humans := map[string]bool{}
	for _, c := range d.clients {
		if !c.Control {
			humans[c.SessionID] = true
		}
	}
	for sid, c := range d.conns {
		select {
		case <-c.Done():
			delete(d.conns, sid)
			continue
		default:
		}
		if !humans[sid] || !d.cfg.Enabled {
			c.Close()
			delete(d.conns, sid)
		}
	}
	if d.cfg.Enabled {
		for sid := range humans {
			if _, ok := d.conns[sid]; ok {
				continue
			}
			c, err := tmux.Attach(d.srv, sid, d.ev.onNote)
			if err != nil {
				d.log.Warnf("attach control client to %s: %v", sid, err)
				continue
			}
			d.conns[sid] = c
			d.log.Debugf("control client attached to %s", sid)
		}
	}
	return true
}

var clientFormat = "#{client_name}\t#{session_id}\t#{client_control_mode}\t#{client_key_table}"

func (d *Daemon) refreshClients(r tmux.Runner) {
	lines, err := r.Run("list-clients", "-F", clientFormat)
	if err != nil {
		return
	}
	d.clients = d.clients[:0]
	for _, l := range lines {
		f := strings.SplitN(l, "\t", 4)
		if len(f) != 4 {
			continue
		}
		d.clients = append(d.clients, clientInfo{Name: f[0], SessionID: f[1], Control: f[2] == "1", KeyTable: f[3]})
	}
}

func (d *Daemon) closeConns() {
	for sid, c := range d.conns {
		c.Close()
		delete(d.conns, sid)
	}
}

func (d *Daemon) shutdown() {
	d.log.Infof("shutting down")
	r := d.runner()
	// Tell subscribers detection has stopped, so nothing stays stuck in a
	// commanding state the daemon can no longer vouch for.
	for sid, f := range d.focusEmitted {
		p := tmux.PaneInfo{ID: f.pane, SessionID: sid}
		if st, ok := d.panes[f.pane]; ok {
			p = st.info
		}
		d.hooks.Emit(d.hookEvent("stop", "s:"+sid, &p, f.m, modeState{}, true))
	}
	d.hooks.Flush(d.cfg.HookTimeout + time.Second)
	d.restoreAllKeyTables(r)
	var cmds []string
	for id := range d.panes {
		for _, o := range paneOptions {
			cmds = append(cmds, tmux.Command("set-option", "-p", "-u", "-q", "-t", id, o))
		}
	}
	d.run(r, cmds)
	d.statusDirty = true
	d.refreshStatus(r)
	for app := range d.installed {
		runLines(r, UnbindCommands(app))
	}
	if lines, _ := r.Run("show-options", "-gqv", "@modal_daemon_pid"); len(lines) > 0 && lines[0] == d.pidOpt {
		r.Run("set-option", "-gu", "@modal_daemon_pid")
	}
	d.closeConns()
}

// Stop signals the running daemon (the `stop` subcommand).
func Stop(args []string) error {
	ex := tmux.Exec{Server: tmux.FromEnv()}
	lines, err := ex.Run("show-options", "-gqv", "@modal_daemon_pid")
	if err != nil {
		return err
	}
	if len(lines) == 0 || lines[0] == "" {
		return errors.New("no daemon running for this server")
	}
	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return err
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	for i := 0; i < 40; i++ {
		if p.Signal(syscall.Signal(0)) != nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("daemon did not exit within 2s")
}

// runLines sends several command lines, pipelined over a control client
// when one is available, and returns any errors.
func runLines(r tmux.Runner, lines []string) []error {
	if len(lines) == 0 {
		return nil
	}
	var errs []error
	if c, ok := r.(*tmux.Control); ok {
		// One control line per command: a parse error then costs only
		// that command, and the error names it.
		type pend struct {
			line string
			wait func() ([]string, error)
		}
		var ps []pend
		for _, l := range lines {
			ch, err := c.Send(l)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", l, err))
				continue
			}
			ps = append(ps, pend{l, func() ([]string, error) { return tmux.Wait(ch) }})
		}
		for _, p := range ps {
			if _, err := p.wait(); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", p.line, err))
			}
		}
		return errs
	}
	// exec: one process for everything, through a temporary file that
	// source-file parses with exactly the same syntax as control mode.
	f, err := os.CreateTemp("", "tmux-modal-*.conf")
	if err != nil {
		return []error{err}
	}
	defer os.Remove(f.Name())
	f.WriteString(strings.Join(lines, "\n") + "\n")
	f.Close()
	if _, err := r.Run("source-file", f.Name()); err != nil {
		errs = append(errs, err)
	}
	return errs
}

// run executes command lines and logs failures.
func (d *Daemon) run(r tmux.Runner, lines []string) {
	for _, err := range runLines(r, lines) {
		d.log.Warnf("tmux command failed: %v", err)
	}
}
