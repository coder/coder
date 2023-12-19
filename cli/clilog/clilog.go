package clilog

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogjson"
	"cdr.dev/slog/sloggers/slogstackdriver"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
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

func (b *Builder) Build(inv *clibase.Invocation) (log slog.Logger, closeLog func(), err error) {
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
			fi, err := os.OpenFile(loc, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
			if err != nil {
				return xerrors.Errorf("open log file %q: %w", loc, err)
			}
			closers = append(closers, fi.Close)
			sinks = append(sinks, sinkFn(fi))
		}
		return nil
	}

	err = addSinkIfProvided(sloghuman.Sink, b.Human)
	if err != nil {
		return slog.Logger{}, noopClose, xerrors.Errorf("add human sink: %w", err)
	}
	err = addSinkIfProvided(slogjson.Sink, b.JSON)
	if err != nil {
		return slog.Logger{}, noopClose, xerrors.Errorf("add json sink: %w", err)
	}
	err = addSinkIfProvided(slogstackdriver.Sink, b.Stackdriver)
	if err != nil {
		return slog.Logger{}, noopClose, xerrors.Errorf("add stackdriver sink: %w", err)
	}

	if b.Trace {
		sinks = append(sinks, tracing.SlogSink{})
	}

	// User should log to null device if they don't want logs.
	if len(sinks) == 0 {
		return slog.Logger{}, noopClose, xerrors.New("no loggers provided, use /dev/null to disable logging")
	}

	filter := &debugFilterSink{next: sinks}

	err = filter.compile(b.Filter)
	if err != nil {
		return slog.Logger{}, noopClose, xerrors.Errorf("compile filters: %w", err)
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
		return xerrors.Errorf("compile regex: %w", err)
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
