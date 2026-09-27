package tmux

import (
	"io"
	"strings"
	"sync"
	"testing"
)

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriter) Close() error                { return nil }

// fakeControl wires a Control to scripted server output.
func fakeControl(t *testing.T) (*Control, *io.PipeWriter, *[]Notification, *sync.Mutex) {
	pr, pw := io.Pipe()
	var mu sync.Mutex
	var notes []Notification
	c := &Control{stdin: nopWriter{}, done: make(chan struct{}),
		notify: func(n Notification) { mu.Lock(); notes = append(notes, n); mu.Unlock() }}
	go c.read(pr)
	return c, pw, &notes, &mu
}

func TestControlRepliesAndNotifications(t *testing.T) {
	c, w, notes, mu := fakeControl(t)
	a, _ := c.Send("first")
	b, _ := c.Send("second")
	e, _ := c.Send("third")
	io.WriteString(w, strings.Join([]string{
		"%begin 100 1 0", // the attach command's own block: not ours
		"%end 100 1 0",
		"%session-changed $0 main",
		"%begin 101 2 1",
		"line one",
		// Pane text that looks like a terminator but carries another
		// command's number must not end this block.
		"%end 999 9 1",
		"line three",
		"%end 101 2 1",
		"%output %3 hello\\015\\012",
		"%begin 102 3 1",
		"%end 102 3 1",
		"%begin 103 4 1",
		"no such pane",
		"%error 103 4 1",
		"",
	}, "\n"))
	if got, err := Wait(a); err != nil || strings.Join(got, "|") != "line one|%end 999 9 1|line three" {
		t.Errorf("first reply = %q, %v", got, err)
	}
	if got, err := Wait(b); err != nil || len(got) != 0 {
		t.Errorf("second reply = %q, %v", got, err)
	}
	if _, err := Wait(e); err == nil || !strings.Contains(err.Error(), "no such pane") {
		t.Errorf("third reply error = %v", err)
	}
	w.Close()
	<-c.Done()
	mu.Lock()
	defer mu.Unlock()
	if len(*notes) != 2 || (*notes)[0].Name != "session-changed" || (*notes)[1].Name != "output" || (*notes)[1].Args[0] != "%3" {
		t.Errorf("notifications = %+v", *notes)
	}
}

func TestControlCloseFailsPending(t *testing.T) {
	c, w, _, _ := fakeControl(t)
	a, _ := c.Send("never answered")
	w.Close()
	if _, err := Wait(a); err != ErrClosed {
		t.Errorf("pending command after EOF: %v", err)
	}
	if _, err := c.Send("x"); err != ErrClosed {
		t.Errorf("send after close: %v", err)
	}
}

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"plain":      "plain",
		"":           "''",
		"has space":  "'has space'",
		"#{pane_id}": "'#{pane_id}'",
		"it's":       `'it'"'"'s'`,
		";":          "';'",
		"C-d":        "C-d",
		"%3":         "%3",
		"%3:off":     "'%3:off'",
		"50%":        "'50%'",
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}
