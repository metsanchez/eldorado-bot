package logger

import (
	"fmt"
	"io"
	"log"
	"time"
)

// Logger is a minimal structured logger wrapper.
type Logger struct {
	out *log.Logger
}

func New(w io.Writer) *Logger {
	return &Logger{
		out: log.New(w, "", 0),
	}
}

func (l *Logger) log(level string, format string, args ...any) {
	ts := time.Now().Format(time.RFC3339)
	prefix := fmt.Sprintf("%s [%s] ", ts, level)
	l.out.Printf(prefix+format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.log("INFO", format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.log("ERROR", format, args...)
}

