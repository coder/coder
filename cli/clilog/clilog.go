package clilog

import (
	"errors"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"cdr.dev/slog/sloggers/slogjson"
	"cdr.dev/slog/sloggers/slogstackdriver"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
type (
	Option  func(*Builder)
	Builder struct {

		Filter      []string
		Human       string
		JSON        string
		Stackdriver string
		Trace       bool
		Verbose     bool
	}
)
func New(opts ...Option) *Builder {
	b := &Builder{}
	for _, opt := range opts {
		opt(b)

	}
	return b
}
func WithFilter(filters ...string) Option {
	return func(b *Builder) {
		b.Filter = filters
	}
}

func WithHuman(loc string) Option {
	return func(b *Builder) {
		b.Human = loc
	}
}
func WithJSON(loc string) Option {

	return func(b *Builder) {
		b.JSON = loc
	}
}
func WithStackdriver(loc string) Option {
	return func(b *Builder) {

		b.Stackdriver = loc
	}
}
func WithTrace() Option {
	return func(b *Builder) {
		b.Trace = true

	}
}
func WithVerbose() Option {
	return func(b *Builder) {
		b.Verbose = true
	}

}
func FromDeploymentValues(vals *codersdk.DeploymentValues) Option {
	return func(b *Builder) {
		b.Filter = vals.Logging.Filter.Value()
		b.Human = vals.Logging.Human.Value()
		b.JSON = vals.Logging.JSON.Value()

		b.Stackdriver = vals.Logging.Stackdriver.Value()
		b.Trace = vals.Trace.Enable.Value()
		b.Verbose = vals.Verbose.Value()
	}
}
func (b *Builder) Build(inv *serpent.Invocation) (log slog.Logger, closeLog func(), err error) {

	var (
		sinks   = []slog.Sink{}
		closers = []func() error{}
	)
	defer func() {
		if err != nil {
			for _, closer := range closers {
				_ = closer()
			}
		}
	}()

	noopClose := func() {}
	addSinkIfProvided := func(sinkFn func(io.Writer) slog.Sink, loc string) error {
		switch loc {
		case "":
		case "/dev/stdout":
			sinks = append(sinks, sinkFn(inv.Stdout))
		case "/dev/stderr":
			sinks = append(sinks, sinkFn(inv.Stderr))
		default:
			logWriter := &LumberjackWriteCloseFixer{Writer: &lumberjack.Logger{
				Filename: loc,
				MaxSize:  5, // MB
				// Without this, rotated logs will never be deleted.

				MaxBackups: 1,
			}}

			closers = append(closers, logWriter.Close)
			sinks = append(sinks, sinkFn(logWriter))
		}
		return nil
	}
	err = addSinkIfProvided(sloghuman.Sink, b.Human)

	if err != nil {
		return slog.Logger{}, noopClose, fmt.Errorf("add human sink: %w", err)
	}

	err = addSinkIfProvided(slogjson.Sink, b.JSON)
	if err != nil {
		return slog.Logger{}, noopClose, fmt.Errorf("add json sink: %w", err)
	}
	err = addSinkIfProvided(slogstackdriver.Sink, b.Stackdriver)
	if err != nil {
		return slog.Logger{}, noopClose, fmt.Errorf("add stackdriver sink: %w", err)
	}
	if b.Trace {
		sinks = append(sinks, tracing.SlogSink{})
	}
	// User should log to null device if they don't want logs.
	if len(sinks) == 0 {

		return slog.Logger{}, noopClose, errors.New("no loggers provided, use /dev/null to disable logging")
	}
	filter := &debugFilterSink{next: sinks}
	err = filter.compile(b.Filter)
	if err != nil {
		return slog.Logger{}, noopClose, fmt.Errorf("compile filters: %w", err)
	}
	level := slog.LevelInfo
	// Debug logging is always enabled if a filter is present.
	if b.Verbose || filter.re != nil {
		level = slog.LevelDebug
	}
	return inv.Logger.AppendSinks(filter).Leveled(level), func() {

		for _, closer := range closers {
			_ = closer()
		}
	}, nil

}
var _ slog.Sink = &debugFilterSink{}
type debugFilterSink struct {
	next []slog.Sink
	re   *regexp.Regexp

}
func (f *debugFilterSink) compile(res []string) error {

	if len(res) == 0 {
		return nil
	}
	var reb strings.Builder
	for i, re := range res {

		_, _ = fmt.Fprintf(&reb, "(%s)", re)
		if i != len(res)-1 {
			_, _ = reb.WriteRune('|')
		}
	}
	re, err := regexp.Compile(reb.String())

	if err != nil {
		return fmt.Errorf("compile regex: %w", err)
	}
	f.re = re
	return nil
}
func (f *debugFilterSink) LogEntry(ctx context.Context, ent slog.SinkEntry) {

	if ent.Level == slog.LevelDebug {
		logName := strings.Join(ent.LoggerNames, ".")

		if f.re != nil && !f.re.MatchString(logName) && !f.re.MatchString(ent.Message) {
			return
		}
	}
	for _, sink := range f.next {

		sink.LogEntry(ctx, ent)
	}
}
func (f *debugFilterSink) Sync() {
	for _, sink := range f.next {

		sink.Sync()
	}
}
// LumberjackWriteCloseFixer is a wrapper around an io.WriteCloser that
// prevents writes after Close. This is necessary because lumberjack
// re-opens the file on Write.
type LumberjackWriteCloseFixer struct {
	Writer io.WriteCloser

	mu     sync.Mutex // Protects following.
	closed bool
}
func (c *LumberjackWriteCloseFixer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.Writer.Close()

}
func (c *LumberjackWriteCloseFixer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	return c.Writer.Write(p)
}
