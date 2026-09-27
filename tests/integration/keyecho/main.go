// Command keyecho is a deterministic full-screen test application.
//
// It shows the name of the last key it received. '/' opens a PROMPT> text
// prompt on the last row (cursor visible there); Enter or Escape closes
// it. 'q' in normal mode quits. The first line is the identity banner,
// "$KEYECHO_NAME TEST APPLICATION v1" (default KEYECHO). It redraws on
// SIGWINCH like a real TUI. With KEYECHO_LOG set, every key is appended
// to that file.
//
// It replaced a bash script: bash cannot both wait for a key and redraw on
// SIGWINCH without polling with `read -t`, and a byte arriving just as
// that timeout fires is lost, which made the integration tests flaky.
// The integration tests build it statically, so the same binary also runs
// in the SSH container.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	name   = "KEYECHO"
	prompt bool
	last   = "none"
	buf    string
)

func main() {
	if n := os.Getenv("KEYECHO_NAME"); n != "" {
		name = n
	}
	stty("-echo", "-icanon", "min", "1", "time", "0")
	fmt.Print("\x1b[?1049h")
	defer func() {
		fmt.Print("\x1b[?25h\x1b[?1049l")
		stty("sane")
	}()

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	in := make(chan byte, 64)
	go func() {
		b := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(b)
			for _, c := range b[:n] {
				in <- c
			}
			if err != nil {
				close(in)
				return
			}
		}
	}()

	draw()
	for {
		select {
		case <-winch:
			draw()
		case c, ok := <-in:
			if !ok {
				return
			}
			k := keyName(c, in)
			logKey(k)
			last = k
			if prompt {
				switch k {
				case "Enter", "Escape":
					prompt, buf = false, ""
				default:
					buf += k
				}
			} else {
				switch k {
				case "/":
					prompt = true
				case "q":
					return
				}
			}
			draw()
		}
	}
}

// keyName names one key; an Escape followed within 20ms by more bytes is
// the start of a sequence (arrow keys, Home, PageDown...).
func keyName(c byte, in chan byte) string {
	switch c {
	case '\r', '\n':
		return "Enter"
	case 0x04:
		return "C-d"
	case 0x15:
		return "C-u"
	case 0x1b:
	default:
		return string(c)
	}
	var seq []byte
	timeout := time.After(20 * time.Millisecond)
	for len(seq) < 2 {
		select {
		case b := <-in:
			seq = append(seq, b)
		case <-timeout:
			if len(seq) == 0 {
				return "Escape"
			}
			return "ESC" + string(seq)
		}
	}
	switch string(seq) {
	case "[A":
		return "Up"
	case "[B":
		return "Down"
	case "[C":
		return "Right"
	case "[D":
		return "Left"
	case "[H":
		return "Home"
	case "[F":
		return "End"
	case "[5", "[6", "[1", "[4":
		select { // the trailing '~'
		case <-in:
		case <-time.After(20 * time.Millisecond):
		}
		return map[string]string{"[5": "PageUp", "[6": "PageDown", "[1": "Home", "[4": "End"}[string(seq)]
	}
	return "ESC" + string(seq)
}

func draw() {
	rows := termRows()
	var b strings.Builder
	fmt.Fprintf(&b, "\x1b[?25l\x1b[H\x1b[2J%s TEST APPLICATION v1\r\n", name)
	fmt.Fprintf(&b, "last=[%s]\r\n", last)
	if prompt {
		fmt.Fprintf(&b, "\x1b[%d;1HPROMPT> %s\x1b[?25h", rows, buf)
	} else {
		fmt.Fprintf(&b, "\x1b[%d;1Hkeyecho normal mode status bar", rows)
	}
	os.Stdout.WriteString(b.String())
}

func termRows() int {
	var ws struct{ Row, Col, X, Y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.Row == 0 {
		return 24
	}
	return int(ws.Row)
}

func stty(args ...string) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	cmd.Run()
}

func logKey(k string) {
	path := os.Getenv("KEYECHO_LOG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s %q\n", time.Now().Format("15:04:05.000000"), k)
	f.Close()
}
