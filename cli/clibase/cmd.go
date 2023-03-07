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
	return i.ctx
}

func (i *Invokation) ParsedFlags() *pflag.FlagSet {
	if i.parsedFlags == nil {
		panic("flags not parsed, has Run() been called?")
	}
	return i.parsedFlags
}

// run recursively executes the command and its children.
// allArgs is wired through the stack so that global flags can be accepted
// anywhere in the command invokation.
func (i *Invokation) run(allArgs []string, flagSet *pflag.FlagSet) error {
	err := i.Command.Options.SetDefaults()
	if err != nil {
		return xerrors.Errorf("setting defaults: %w", err)
	}

	childrenMap := make(map[string]*Cmd)
	for _, child := range i.Command.Children {
		for _, name := range append(child.Aliases, child.Name()) {
			if _, ok := childrenMap[name]; ok {
				return xerrors.Errorf("duplicate command name: %s", name)
			}
			childrenMap[name] = child
		}
	}

	if flagSet == nil {
		flagSet = pflag.NewFlagSet(i.Command.Name(), pflag.ContinueOnError)
	}

	additionalFlags := i.Command.Options.FlagSet()
	flagSet.AddFlagSet(additionalFlags)

	// Run child command if found.
	for argI, arg := range i.Args {
		if child, ok := childrenMap[arg]; ok {
			i.Args = i.Args[argI+1:]
			child.Parent = i.Command
			i.Command = child
			err := i.run(allArgs, flagSet)
			if err != nil {
				return xerrors.Errorf(
					"subcommand %s: %w", child.Name(), err,
				)
			}
			return nil
		}
	}

	err = i.Command.Options.ParseEnv(i.Environ)
	if err != nil {
		return xerrors.Errorf("parsing env: %w", err)
	}

	err = flagSet.Parse(allArgs)
	if err != nil {
		return xerrors.Errorf("parsing flags: %w", err)
	}
	i.parsedFlags = flagSet

	mw := i.Command.Middleware
	if mw == nil {
		mw = Chain()
	}

	if i.Command.Handler == nil {
		if i.Command.HelpHandler != nil {
			return i.Command.HelpHandler(i)
		}
		return xerrors.Errorf("no handler or help for command %s", i.Command.FullName())
	}

	i.Args = stripFlags(i.Args)
	return mw(i.Command.Handler)(i)
}

func stripFlags(args []string) []string {
	var stripped []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		stripped = append(stripped, arg)
	}
	return stripped
}

// Run executes the command.
// If two command share a flag name, the deepest command wins.
//
//nolint:revive
func (i *Invokation) Run() error {
	return i.run(i.Args, nil)
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
	if want < 0 {
		panic("want must be >= 0")
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(i *Invokation) error {
			if len(i.Args) != want {
				if want == 0 {
					return xerrors.Errorf("wanted no args but got %v", len(i.Args))
				}
				return fmt.Errorf(
					"wanted %v args but got %v",
					want,
					len(i.Args),
				)
			}
			return next(i)
		}
	}
}

// HandlerFunc handles an Invokation of a command.
type HandlerFunc func(i *Invokation) error
