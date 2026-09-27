// Package tmux talks to a tmux server, either through a persistent
// control-mode client (the fast path: no process spawn per command) or by
// running the tmux binary once per command (the fallback, and what one-shot
// CLI commands use).
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes tmux commands and returns their output lines.
type Runner interface {
	// Run executes one command (argv form, no shell).
	Run(args ...string) ([]string, error)
}

// Server identifies a tmux server by socket.
type Server struct {
	Bin    string // tmux binary, default "tmux"
	Socket string // -S path; "" means the default server (or $TMUX)
	Label  string // -L name, used when Socket is empty
}

// FromEnv returns the server this process runs under, from $TMUX.
func FromEnv() Server {
	s := Server{Bin: "tmux"}
	if t := os.Getenv("TMUX"); t != "" {
		s.Socket = strings.SplitN(t, ",", 2)[0]
	}
	return s
}

func (s Server) base() []string {
	var a []string
	switch {
	case s.Socket != "":
		a = append(a, "-S", s.Socket)
	case s.Label != "":
		a = append(a, "-L", s.Label)
	}
	return a
}

func (s Server) bin() string {
	if s.Bin == "" {
		return "tmux"
	}
	return s.Bin
}

// Exec runs one tmux process per command.
type Exec struct{ Server Server }

func (e Exec) Run(args ...string) ([]string, error) {
	cmd := exec.Command(e.Server.bin(), append(e.Server.base(), args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("tmux %s: %s", strings.Join(args, " "), msg)
	}
	return splitOutput(out.String()), nil
}

func splitOutput(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Quote renders one argument for tmux's command parser (used by control
// mode and by generated key bindings). Single quotes suppress every
// expansion; an embedded single quote is spliced in from double quotes.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\#;{}$~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// Command renders argv as one tmux command line.
func Command(args ...string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = Quote(a)
	}
	return strings.Join(q, " ")
}
