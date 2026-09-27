package tmux

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Notification is an asynchronous control-mode message such as
// "%output %3 ..." or "%window-pane-changed @1 %4".
type Notification struct {
	Name string   // without the leading %, e.g. "output"
	Args []string // space-separated fields; for output, [pane, data]
}

// Control is a persistent control-mode client attached to one session.
//
// It is attached with ignore-size so it never influences window sizes.
// Commands are pipelined; replies arrive in order, each wrapped in a
// %begin/%end (or %error) block whose flags mark it as ours.
type Control struct {
	Session string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	notify func(Notification)

	mu      sync.Mutex
	pending []chan reply
	closed  bool
	done    chan struct{}
	err     error
}

type reply struct {
	lines []string
	err   error
}

var ErrClosed = errors.New("control client closed")

// Attach starts a control-mode client on session (a name or $id).
// notify is called from the reader goroutine for every notification; it
// must not block and must not issue commands on this client.
func Attach(s Server, session string, notify func(Notification)) (*Control, error) {
	args := append(s.base(), "-C", "attach-session", "-f", "ignore-size", "-t", session)
	cmd := exec.Command(s.bin(), args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Control{Session: session, cmd: cmd, stdin: stdin, notify: notify, done: make(chan struct{})}
	go c.read(stdout)
	return c, nil
}

// Pid is the control client's process id (its #{client_pid}).
func (c *Control) Pid() int { return c.cmd.Process.Pid }

// Done is closed when the client exits.
func (c *Control) Done() <-chan struct{} { return c.done }

// Err is why the client exited, once Done is closed.
func (c *Control) Err() error { return c.err }

// Run implements Runner.
func (c *Control) Run(args ...string) ([]string, error) {
	return c.RunLine(Command(args...))
}

// RunLine sends one raw command line (may contain several commands
// separated by " ; ") and waits for its reply.
func (c *Control) RunLine(line string) ([]string, error) {
	ch, err := c.Send(line)
	if err != nil {
		return nil, err
	}
	r := <-ch
	return r.lines, r.err
}

// Send queues a command line without waiting; the reply arrives on the
// returned channel. Several Sends pipeline over the one connection.
func (c *Control) Send(line string) (<-chan reply, error) {
	if strings.ContainsAny(line, "\n\r") {
		return nil, fmt.Errorf("control command contains a newline")
	}
	ch := make(chan reply, 1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrClosed
	}
	c.pending = append(c.pending, ch)
	if _, err := io.WriteString(c.stdin, line+"\n"); err != nil {
		c.pending = c.pending[:len(c.pending)-1]
		return nil, err
	}
	return ch, nil
}

// Wait returns the reply of a Send.
func Wait(ch <-chan reply) ([]string, error) {
	r := <-ch
	return r.lines, r.err
}

// Close detaches the client.
func (c *Control) Close() {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.stdin.Close()
	}
	c.mu.Unlock()
	<-c.done
}

func (c *Control) read(r io.Reader) {
	br := bufio.NewReaderSize(r, 1<<16)
	var (
		inBlock bool
		ours    bool
		guard   string // "<time> <number>" of the open block
		lines   []string
	)
	for {
		raw, err := br.ReadString('\n')
		if err != nil {
			c.finish(err)
			return
		}
		line := strings.TrimSuffix(raw, "\n")
		if inBlock {
			// Only a terminator carrying this block's exact time and
			// number ends it, so pane text that happens to look like
			// "%end" inside captured output cannot desynchronise us.
			if kind, rest, ok := cutTerminator(line); ok && strings.HasPrefix(rest, guard+" ") {
				inBlock = false
				if ours {
					c.deliver(lines, kind == "%error")
				}
				lines = nil
				continue
			}
			lines = append(lines, line)
			continue
		}
		if strings.HasPrefix(line, "%begin ") {
			f := strings.Fields(line)
			if len(f) == 4 {
				inBlock, guard, lines = true, f[1]+" "+f[2], nil
				ours = f[3] != "0"
				continue
			}
		}
		if strings.HasPrefix(line, "%") {
			c.notification(line)
		}
	}
}

func cutTerminator(line string) (kind, rest string, ok bool) {
	for _, k := range []string{"%end ", "%error "} {
		if strings.HasPrefix(line, k) {
			return strings.TrimSpace(k), line[len(k):], true
		}
	}
	return "", "", false
}

func (c *Control) deliver(lines []string, isErr bool) {
	c.mu.Lock()
	if len(c.pending) == 0 {
		c.mu.Unlock()
		return
	}
	ch := c.pending[0]
	c.pending = c.pending[1:]
	c.mu.Unlock()
	if isErr {
		ch <- reply{err: fmt.Errorf("tmux: %s", strings.Join(lines, "; "))}
		return
	}
	ch <- reply{lines: lines}
}

func (c *Control) notification(line string) {
	name, rest, _ := strings.Cut(line[1:], " ")
	n := Notification{Name: name}
	if name == "output" {
		pane, data, _ := strings.Cut(rest, " ")
		n.Args = []string{pane, data}
	} else if rest != "" {
		n.Args = strings.Fields(rest)
	}
	if c.notify != nil {
		c.notify(n)
	}
}

func (c *Control) finish(err error) {
	c.mu.Lock()
	c.closed = true
	pend := c.pending
	c.pending = nil
	c.mu.Unlock()
	for _, ch := range pend {
		ch <- reply{err: ErrClosed}
	}
	if c.cmd != nil {
		_ = c.cmd.Wait()
	}
	if err == io.EOF {
		err = nil
	}
	c.err = err
	close(c.done)
}
