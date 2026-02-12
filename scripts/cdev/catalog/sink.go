package catalog

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
)

// LoggerSink is a controllable slog.Sink with pretty formatting.
type LoggerSink struct {
	mu          sync.Mutex
	w           io.Writer
	emoji       string
	serviceName string
	done        atomic.Bool
}

// NewLoggerSink returns a controllable sink with pretty formatting.
// If svc is non-nil, lines are prefixed with the service's emoji
// and name. Pass nil for non-service contexts.
func NewLoggerSink(w io.Writer, svc ServiceBase) *LoggerSink {
	s := &LoggerSink{w: w, emoji: "🚀", serviceName: "cdev"}
	if svc != nil {
		s.emoji = svc.Emoji()
		s.serviceName = svc.Name()
	}
	return s
}

func (l *LoggerSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	if l.done.Load() {
		return
	}

	ts := cliui.Timestamp(e.Time)

	var streamTag string
	if e.Level >= slog.LevelWarn {
		streamTag = pretty.Sprint(cliui.DefaultStyles.Warn, "stderr")
	} else {
		streamTag = pretty.Sprint(cliui.DefaultStyles.Keyword, "stdout")
	}

	serviceLabel := fmt.Sprintf("%s %-10s", l.emoji, l.serviceName)

	var fields string
	for _, f := range e.Fields {
		fields += fmt.Sprintf(" %s=%v", f.Name, f.Value)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.w, "%s %s [%s] %s%s\n", serviceLabel, ts, streamTag, e.Message, fields)
}

func (*LoggerSink) Sync() {}

func (l *LoggerSink) Close() {
	l.done.Store(true)
}
