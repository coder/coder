package clibase

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/pflag"
	"golang.org/x/xerrors"
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

// Walk calls fn for the command and all its children.
func (c *Cmd) Walk(fn func(*Cmd)) {
	fn(c)
	for _, child := range c.Children {
		child.Walk(fn)
	}
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
		uses = append(uses, c.Parent.FullUsage())
	}
	uses = append(uses, c.Use)
	return strings.Join(uses, " ")
}

// Invoke creates a new invokation of the command, with
// stdio discarded.
//
// The returned invokation is not live until Run() is called.
func (c *Cmd) Invoke(args ...string) *Invokation {
	return &Invokation{
		Command: c,
		Args:    args,
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Stdin:   strings.NewReader(""),
	}
}

// Invokation represents an instance of a command being executed.
type Invokation struct {
	parent *Invokation

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
}

func (i *Invokation) Context() context.Context {
	if i.ctx == nil {
		return context.Background()
	}
	return i.ctx
}

func (i *Invokation) ParsedFlags() *pflag.FlagSet {
	if i.parsedFlags == nil {
		panic("flags not parsed, has Run() been called?")
	}
	return i.parsedFlags
}

type runState struct {
	allArgs      []string
	commandDepth int

	flagParseErr error
}

// run recursively executes the command and its children.
// allArgs is wired through the stack so that global flags can be accepted
// anywhere in the command invokation.
func (i *Invokation) run(state *runState) error {
	err := i.Command.Options.SetDefaults()
	if err != nil {
		return xerrors.Errorf("setting defaults: %w", err)
	}

	err = i.Command.Options.ParseEnv(i.Environ)
	if err != nil {
		return xerrors.Errorf("parsing env: %w", err)
	}

	// Now the fun part, argument parsing!

	children := make(map[string]*Cmd)
	for _, child := range i.Command.Children {
		for _, name := range append(child.Aliases, child.Name()) {
			if _, ok := children[name]; ok {
				return xerrors.Errorf("duplicate command name: %s", name)
			}
			children[name] = child
		}
	}

	if i.parsedFlags == nil {
		i.parsedFlags = pflag.NewFlagSet(i.Command.Name(), pflag.ContinueOnError)
	}

	i.parsedFlags.AddFlagSet(i.Command.Options.FlagSet())

	var parsedArgs []string

	if !i.Command.RawArgs {
		// Flag parsing will fail on intermediate commands in the command tree,
		// so we check the error after looking for a child command.
		state.flagParseErr = i.parsedFlags.Parse(state.allArgs)
		parsedArgs = i.parsedFlags.Args()
	}

	// Run child command if found (next child only)
	// We must do subcommand detection after flag parsing so we don't mistake flag
	// values for subcommand names.
	if len(parsedArgs) > 0 {
		// _, _ = fmt.Printf("args: %v, %v\n", i.Args, state.flagParseErr)
		nextArg := parsedArgs[0]
		if child, ok := children[nextArg]; ok {
			// _, _ = fmt.Printf("push args: %v\n", i.Args)
			child.Parent = i.Command
			i.Command = child
			state.commandDepth++
			err = i.run(state)
			if err != nil {
				return xerrors.Errorf(
					"subcommand %s: %w", child.Name(), err,
				)
			}
			return nil
		}
	}

	// Flag parse errors are irrelevant for raw args commands.
	if !i.Command.RawArgs && state.flagParseErr != nil {
		return xerrors.Errorf(
			"parsing flags (%v) for %q: %w",
			state.allArgs,
			i.Command.FullName(), state.flagParseErr,
		)
	}

	if i.Command.RawArgs {
		// If we're at the root command, then the name is omitted
		// from the arguments, so we can just use the entire slice.
		if state.commandDepth == 0 {
			i.Args = state.allArgs
		} else {
			argPos, err := findArg(i.Command.Name(), state.allArgs, i.parsedFlags)
			if err != nil {
				panic(err)
			}
			i.Args = state.allArgs[argPos+1:]
		}
	} else {
		// In non-raw-arg mode, we want to skip over flags.
		i.Args = parsedArgs[state.commandDepth:]
	}

	mw := i.Command.Middleware
	if mw == nil {
		mw = Chain()
	}

	if i.Command.Handler == nil {
		if i.Command.HelpHandler == nil {
			return xerrors.Errorf("no handler or help for command %s", i.Command.FullName())
		}
		return i.Command.HelpHandler(i)
	}

	err = mw(i.Command.Handler)(i)
	if err != nil {
		return xerrors.Errorf("running command %s: %w", i.Command.FullName(), err)
	}
	return nil
}

// findArg returns the index of the first occurrence of arg in args, skipping
// over all flags.
func findArg(want string, args []string, fs *pflag.FlagSet) (int, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
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

		if arg == want {
			return i, nil
		}
	}

	return -1, xerrors.Errorf("arg %s not found", want)
}

// Run executes the command.
// If two command share a flag name, the first command wins.
//
//nolint:revive
func (i *Invokation) Run() error {
	return i.run(&runState{
		allArgs: i.Args,
	})
}

// WithContext returns a copy of the Invokation with the given context.
func (i *Invokation) WithContext(ctx context.Context) *Invokation {
	i2 := *i
	i2.parent = i
	i2.ctx = ctx
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
		return func(i *Invokation) error {
			got := len(i.Args)
			switch {
			case start == end && got != start:
				switch start {
				case 0:
					return xerrors.Errorf("wanted no args but got %v %v", got, i.Args)
				default:
					return fmt.Errorf(
						"wanted %v args but got %v %v",
						start,
						got,
						i.Args,
					)
				}
			case start > 0 && end == -1:
				switch {
				case got < start:
					return fmt.Errorf(
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
				return fmt.Errorf(
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

// HandlerFunc handles an Invokation of a command.
type HandlerFunc func(i *Invokation) error
