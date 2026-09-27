package daemon

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger is a levelled, line-oriented logger.
type Logger struct {
	mu    sync.Mutex
	w     io.Writer
	level int
}

var levels = map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}

func newLogger(w io.Writer, level string) *Logger {
	return &Logger{w: w, level: levels[level]}
}

func (l *Logger) SetLevel(level string) {
	l.mu.Lock()
	l.level = levels[level]
	l.mu.Unlock()
}

func (l *Logger) log(lv int, tag, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lv < l.level {
		return
	}
	fmt.Fprintf(l.w, "%s %-5s %s\n", time.Now().Format("15:04:05.000"), tag, fmt.Sprintf(format, args...))
}

func (l *Logger) Debugf(f string, a ...any) { l.log(0, "debug", f, a...) }
func (l *Logger) Infof(f string, a ...any)  { l.log(1, "info", f, a...) }
func (l *Logger) Warnf(f string, a ...any)  { l.log(2, "warn", f, a...) }
func (l *Logger) Errorf(f string, a ...any) { l.log(3, "error", f, a...) }

func openLog(path string) io.Writer {
	if path == "" || path == "-" {
		return os.Stderr
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return os.Stderr
	}
	return f
}
