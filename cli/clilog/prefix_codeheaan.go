package clilog

import (
  "context"
	"fmt"

	"cdr.dev/slog"
)

type CodeHEaaNMessageSink struct {
	Inner      slog.Sink
	DefaultMsg string
}


func (d *CodeHEaaNMessageSink) LogEntry(ctx context.Context, e slog.SinkEntry) {
	// Create a new message with the default message.
	e.Message = fmt.Sprintf("%s %s", d.DefaultMsg, e.Message)
	d.Inner.LogEntry(ctx, e)
}

func (d *CodeHEaaNMessageSink) Sync() {
	if syncer, ok := d.Inner.(interface{ Sync() }); ok {
		syncer.Sync()
	}
}
