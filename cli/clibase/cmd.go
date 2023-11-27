package clibase

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"testing"
	"unicode"

	"cdr.dev/slog"

	"github.com/spf13/pflag"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/coderd/util/slice"
)

// Cmd describes an executable command.
type Cmd struct {
	// Parent is the direct parent of the command.
	Parent *Cmd
	// Children is a list of direct descendants.
	Children []*Cmd
	// Use is provided in form "command [flags] [args...]".
	Use string

	// Aliases is a list of alternative names for the command.
	Aliases []string

	// Short is a one-line description of the command.
	Short string

	// Hidden determines whether the command should be hidden from help.
	Hidden bool

	// RawArgs determines whether the command should receive unparsed arguments.
	// No flags are parsed when set, and the command is responsible for parsing
	// its own flags.
	RawArgs bool

	// Long is a detailed description of the command,
	// presented on its help page. It may contain examples.
	Long        string
	Options     OptionSet
	Annotations Annotations

	// Middleware is called before the Handler.
	// Use Chain() to combine multiple middlewares.
	Middleware  MiddlewareFunc
	Handler     HandlerFunc
	HelpHandler HandlerFunc
}

// AddSubcommands adds the given subcommands, setting their
// Parent field automatically.
func (c *Cmd) AddSubcommands(cmds ...*Cmd) {
	for _, cmd := range cmds {
		cmd.Parent = c
		c.Children = append(c.Children, cmd)
	}
}

// Walk calls fn for the command and all its children.
func (c *Cmd) Walk(fn func(*Cmd)) {
	fn(c)
	for _, child := range c.Children {
		child.Parent = c
		child.Walk(fn)
	}
}

// PrepareAll performs initialization and linting on the command and all its children.
func (c *Cmd) PrepareAll() error {
	if c.Use == "" {
		return xerrors.New("command must have a Use field so that it has a name")
	}
	var merr error

	for i := range c.Options {
		opt := &c.Options[i]
		if opt.Name == "" {
			switch {
			case opt.Flag != "":
				opt.Name = opt.Flag
			case opt.Env != "":
				opt.Name = opt.Env
			case opt.YAML != "":
				opt.Name = opt.YAML
			default:
				merr = errors.Join(merr, xerrors.Errorf("option must have a Name, Flag, Env or YAML field"))
			}
		}
		if opt.Description != "" {
			// Enforce that description uses sentence form.
			if unicode.IsLower(rune(opt.Description[0])) {
				merr = errors.Join(merr, xerrors.Errorf("option %q description should start with a capital letter", opt.Name))
			}
			if !strings.HasSuffix(opt.Description, ".") {
				merr = errors.Join(merr, xerrors.Errorf("option %q description should end with a period", opt.Name))
			}
		}
	}

	slices.SortFunc(c.Options, func(a, b Option) int {
		return slice.Ascending(a.Name, b.Name)
	})
	slices.SortFunc(c.Children, func(a, b *Cmd) int {
		return slice.Ascending(a.Name(), b.Name())
	})
	for _, child := range c.Children {
		child.Parent = c
		err := child.PrepareAll()
		if err != nil {
			merr = errors.Join(merr, xerrors.Errorf("command %v: %w", child.Name(), err))
		}
	}
	return merr
}

// Name returns the first word in the Use string.
func (c *Cmd) Name() string {
	return strings.Split(c.Use, " ")[0]
}

// FullName returns the full invocation name of the command,
// as seen on the command line.
func (c *Cmd) FullName() string {
	var names []string
	if c.Parent != nil {
		names = append(names, c.Parent.FullName())
	}
	names = append(names, c.Name())
	return strings.Join(names, " ")
}

// FullName returns usage of the command, preceded
// by the usage of its parents.
func (c *Cmd) FullUsage() string {
	var uses []string
	if c.Parent != nil {
		uses = append(uses, c.Parent.FullName())
	}
	uses = append(uses, c.Use)
	return strings.Join(uses, " ")
}

// FullOptions returns the options of the command and its parents.
func (c *Cmd) FullOptions() OptionSet {
	var opts OptionSet
	if c.Parent != nil {
		opts = append(opts, c.Parent.FullOptions()...)
	}
	opts = append(opts, c.Options...)
	return opts
}

// Invoke creates a new invocation of the command, with
// stdio discarded.
//
// The returned invocation is not live until Run() is called.
func (c *Cmd) Invoke(args ...string) *Invocation {
	return &Invocation{
		Command: c,
		Args:    args,
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Stdin:   strings.NewReader(""),
		Logger:  slog.Make(),
	}
}

// Invocation represents an instance of a command being executed.
type Invocation struct {
	ctx         context.Context
	Command     *Cmd
	parsedFlags *pflag.FlagSet
	Args        []string
	// Environ is a list of environment variables. Use EnvsWithPrefix to parse
	// os.Environ.
	Environ Environ
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader
	Logger  slog.Logger

	// testing
	signalNotifyContext func(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc)
}

// WithOS returns the invocation as a main package, filling in the invocation's unset
// fields with OS defaults.
func (inv *Invocation) WithOS() *Invocation {
	return inv.with(func(i *Invocation) {
		i.Stdout = os.Stdout
		i.Stderr = os.Stderr
		i.Stdin = os.Stdin
		i.Args = os.Args[1:]
		i.Environ = ParseEnviron(os.Environ(), "")
	})
}

// WithTestSignalNotifyContext allows overriding the default implementation of SignalNotifyContext.
// This should only be used in testing.
func (inv *Invocation) WithTestSignalNotifyContext(
	_ testing.TB, // ensure we only call this from tests
	f func(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc),
) *Invocation {
	return inv.with(func(i *Invocation) {
		i.signalNotifyContext = f
	})
}

// SignalNotifyContext is equivalent to signal.NotifyContext, but supports being overridden in
// tests.
func (inv *Invocation) SignalNotifyContext(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc) {
	if inv.signalNotifyContext == nil {
		return signal.NotifyContext(parent, signals...)
	}
	return inv.signalNotifyContext(parent, signals...)
}

func (inv *Invocation) WithTestParsedFlags(
	_ testing.TB, // ensure we only call this from tests
	parsedFlags *pflag.FlagSet,
) *Invocation {
	return inv.with(func(i *Invocation) {
		i.parsedFlags = parsedFlags
	})
}

func (inv *Invocation) Context() context.Context {
	if inv.ctx == nil {
		return context.Background()
	}
	return inv.ctx
}

func (inv *Invocation) ParsedFlags() *pflag.FlagSet {
	if inv.parsedFlags == nil {
		panic("flags not parsed, has Run() been called?")
	}
	return inv.parsedFlags
}

type runState struct {
	allArgs      []string
	commandDepth int

	flagParseErr error
}

func copyFlagSetWithout(fs *pflag.FlagSet, without string) *pflag.FlagSet {
	fs2 := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs2.Usage = func() {}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name == without {
			return
		}
		fs2.AddFlag(f)
	})
	return fs2
}

// run recursively executes the command and its children.
// allArgs is wired through the stack so that global flags can be accepted
// anywhere in the command invocation.
func (inv *Invocation) run(state *runState) error {
	err := inv.Command.Options.ParseEnv(inv.Environ)
	if err != nil {
		return xerrors.Errorf("parsing env: %w", err)
	}

	// Now the fun part, argument parsing!

	children := make(map[string]*Cmd)
	for _, child := range inv.Command.Children {
		child.Parent = inv.Command
		for _, name := range append(child.Aliases, child.Name()) {
			if _, ok := children[name]; ok {
				return xerrors.Errorf("duplicate command name: %s", name)
			}
			children[name] = child
		}
	}

	if inv.parsedFlags == nil {
		inv.parsedFlags = pflag.NewFlagSet(inv.Command.Name(), pflag.ContinueOnError)
		// We handle Usage ourselves.
		inv.parsedFlags.Usage = func() {}
	}

	// If we find a duplicate flag, we want the deeper command's flag to override
	// the shallow one. Unfortunately, pflag has no way to remove a flag, so we
	// have to create a copy of the flagset without a value.
	inv.Command.Options.FlagSet().VisitAll(func(f *pflag.Flag) {
		if inv.parsedFlags.Lookup(f.Name) != nil {
			inv.parsedFlags = copyFlagSetWithout(inv.parsedFlags, f.Name)
		}
		inv.parsedFlags.AddFlag(f)
	})

	var parsedArgs []string

	if !inv.Command.RawArgs {
		// Flag parsing will fail on intermediate commands in the command tree,
		// so we check the error after looking for a child command.
		state.flagParseErr = inv.parsedFlags.Parse(state.allArgs)
		parsedArgs = inv.parsedFlags.Args()
	}

	// Set value sources for flags.
	for i, opt := range inv.Command.Options {
		if fl := inv.parsedFlags.Lookup(opt.Flag); fl != nil && fl.Changed {
			inv.Command.Options[i].ValueSource = ValueSourceFlag
		}
	}

	// Read YAML configs, if any.
	for _, opt := range inv.Command.Options {
		path, ok := opt.Value.(*YAMLConfigPath)
		if !ok || path.String() == "" {
			continue
		}

		byt, err := os.ReadFile(path.String())
		if err != nil {
			return xerrors.Errorf("reading yaml: %w", err)
		}

		var n yaml.Node
		err = yaml.Unmarshal(byt, &n)
		if err != nil {
			return xerrors.Errorf("decoding yaml: %w", err)
		}

		err = inv.Command.Options.UnmarshalYAML(&n)
		if err != nil {
			return xerrors.Errorf("applying yaml: %w", err)
		}
	}

	err = inv.Command.Options.SetDefaults()
	if err != nil {
		return xerrors.Errorf("setting defaults: %w", err)
	}

	// Run child command if found (next child only)
	// We must do subcommand detection after flag parsing so we don't mistake flag
	// values for subcommand names.
	if len(parsedArgs) > state.commandDepth {
		nextArg := parsedArgs[state.commandDepth]
		if child, ok := children[nextArg]; ok {
			child.Parent = inv.Command
			inv.Command = child
			state.commandDepth++
			return inv.run(state)
		}
	}

	// Flag parse errors are irrelevant for raw args commands.
	if !inv.Command.RawArgs && state.flagParseErr != nil && !errors.Is(state.flagParseErr, pflag.ErrHelp) {
		return xerrors.Errorf(
			"parsing flags (%v) for %q: %w",
			state.allArgs,
			inv.Command.FullName(), state.flagParseErr,
		)
	}

	// All options should be set. Check all required options have sources,
	// meaning they were set by the user in some way (env, flag, etc).
	var missing []string
	for _, opt := range inv.Command.Options {
		if opt.Required && opt.ValueSource == ValueSourceNone {
			missing = append(missing, opt.Flag)
		}
	}
	if len(missing) > 0 {
		return xerrors.Errorf("Missing values for the required flags: %s", strings.Join(missing, ", "))
	}

	if inv.Command.RawArgs {
		// If we're at the root command, then the name is omitted
		// from the arguments, so we can just use the entire slice.
		if state.commandDepth == 0 {
			inv.Args = state.allArgs
		} else {
			argPos, err := findArg(inv.Command.Name(), state.allArgs, inv.parsedFlags)
			if err != nil {
				panic(err)
			}
			inv.Args = state.allArgs[argPos+1:]
		}
	} else {
		// In non-raw-arg mode, we want to skip over flags.
		inv.Args = parsedArgs[state.commandDepth:]
	}

	mw := inv.Command.Middleware
	if mw == nil {
		mw = Chain()
	}

	ctx := inv.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	inv = inv.WithContext(ctx)

	if inv.Command.Handler == nil || errors.Is(state.flagParseErr, pflag.ErrHelp) {
		if inv.Command.HelpHandler == nil {
			return xerrors.Errorf("no handler or help for command %s", inv.Command.FullName())
		}
		return inv.Command.HelpHandler(inv)
	}

	err = mw(inv.Command.Handler)(inv)
	if err != nil {
		return &RunCommandError{
			Cmd: inv.Command,
			Err: err,
		}
	}
	return nil
}

type RunCommandError struct {
	Cmd *Cmd
	Err error
}

func (e *RunCommandError) Unwrap() error {
	return e.Err
}

func (e *RunCommandError) Error() string {
	return fmt.Sprintf("running command %q: %+v", e.Cmd.FullName(), e.Err)
}

// findArg returns the index of the first occurrence of arg in args, skipping
// over all flags.
func findArg(want string, args []string, fs *pflag.FlagSet) (int, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			if arg == want {
				return i, nil
			}
			continue
		}

		// This is a flag!
		if strings.Contains(arg, "=") {
			// The flag contains the value in the same arg, just skip.
			continue
		}

		// We need to check if NoOptValue is set, then we should not wait
		// for the next arg to be the value.
		f := fs.Lookup(strings.TrimLeft(arg, "-"))
		if f == nil {
			return -1, xerrors.Errorf("unknown flag: %s", arg)
		}
		if f.NoOptDefVal != "" {
			continue
		}

		if i == len(args)-1 {
			return -1, xerrors.Errorf("flag %s requires a value", arg)
		}

		// Skip the value.
		i++
	}

	return -1, xerrors.Errorf("arg %s not found", want)
}

// Run executes the command.
// If two command share a flag name, the first command wins.
//
//nolint:revive
func (inv *Invocation) Run() (err error) {
	defer func() {
		// Pflag is panicky, so additional context is helpful in tests.
		if flag.Lookup("test.v") == nil {
			return
		}
		if r := recover(); r != nil {
			err = xerrors.Errorf("panic recovered for %s: %v", inv.Command.FullName(), r)
			panic(err)
		}
	}()
	// We close Stdin to prevent deadlocks, e.g. when the command
	// has ended but an io.Copy is still reading from Stdin.
	defer func() {
		if inv.Stdin == nil {
			return
		}
		rc, ok := inv.Stdin.(io.ReadCloser)
		if !ok {
			return
		}
		e := rc.Close()
		err = errors.Join(err, e)
	}()
	err = inv.run(&runState{
		allArgs: inv.Args,
	})
	return err
}

// WithContext returns a copy of the Invocation with the given context.
func (inv *Invocation) WithContext(ctx context.Context) *Invocation {
	return inv.with(func(i *Invocation) {
		i.ctx = ctx
	})
}

// with returns a copy of the Invocation with the given function applied.
func (inv *Invocation) with(fn func(*Invocation)) *Invocation {
	i2 := *inv
	fn(&i2)
	return &i2
}

// MiddlewareFunc returns the next handler in the chain,
// or nil if there are no more.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

func chain(ms ...MiddlewareFunc) MiddlewareFunc {
	return MiddlewareFunc(func(next HandlerFunc) HandlerFunc {
		if len(ms) > 0 {
			return chain(ms[1:]...)(ms[0](next))
		}
		return next
	})
}

// Chain returns a Handler that first calls middleware in order.
//
//nolint:revive
func Chain(ms ...MiddlewareFunc) MiddlewareFunc {
	// We need to reverse the array to provide top-to-bottom execution
	// order when defining a command.
	reversed := make([]MiddlewareFunc, len(ms))
	for i := range ms {
		reversed[len(ms)-1-i] = ms[i]
	}
	return chain(reversed...)
}

func RequireNArgs(want int) MiddlewareFunc {
	return RequireRangeArgs(want, want)
}

// RequireRangeArgs returns a Middleware that requires the number of arguments
// to be between start and end (inclusive). If end is -1, then the number of
// arguments must be at least start.
func RequireRangeArgs(start, end int) MiddlewareFunc {
	if start < 0 {
		panic("start must be >= 0")
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(i *Invocation) error {
			got := len(i.Args)
			switch {
			case start == end && got != start:
				switch start {
				case 0:
					if len(i.Command.Children) > 0 {
						return xerrors.Errorf("unrecognized subcommand %q", i.Args[0])
					}
					return xerrors.Errorf("wanted no args but got %v %v", got, i.Args)
				default:
					return xerrors.Errorf(
						"wanted %v args but got %v %v",
						start,
						got,
						i.Args,
					)
				}
			case start > 0 && end == -1:
				switch {
				case got < start:
					return xerrors.Errorf(
						"wanted at least %v args but got %v",
						start,
						got,
					)
				default:
					return next(i)
				}
			case start > end:
				panic("start must be <= end")
			case got < start || got > end:
				return xerrors.Errorf(
					"wanted between %v and %v args but got %v",
					start, end,
					got,
				)
			default:
				return next(i)
			}
		}
	}
}

// HandlerFunc handles an Invocation of a command.
type HandlerFunc func(i *Invocation) error
