package clibase

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
)

// Cmd describes an executable command.
type Cmd struct {
	// Parent is the direct parent of the command.
	Parent *Cmd
	// Children is a list of direct descendants.
	Children []*Cmd
	// Use is provided in form "command [flags] [args...]".
	Use string
	// Short is a one-line description of the command.
	Short string
	// Hidden determines whether the command should be hidden from help.
	Hidden bool
	// Long is a detailed description of the command,
	// presented on its help page. It may contain examples.
	Long        string
	Options     *OptionSet
	Annotations Annotations
	Handler     Handler
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

	ctx     context.Context
	Command *Command
	Args    []string
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader

	err error
}

func (i *Invokation) Context() context.Context {
	return i.ctx
}

func (i *Invokation) Exit(err error) {
	if i.parent != nil {
		i.parent.Exit(err)
		return
	}

	i.err = err
	// Goexit simulates an os.Exit, but can be captured in tests and
	// middleware.
	runtime.Goexit()
}

func (i *Invokation) Run() error {
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		i.Command.Handler.ServeCommand(i)
	}()
	<-waitDone
	return i.err
}

// WithContext returns a copy of the Invokation with the given context.
func (i *Invokation) WithContext(ctx context.Context) *Invokation {
	i2 := *i
	i2.parent = i
	i2.ctx = ctx
	return &i2
}

// Middleware returns the next handler in the chain,
// or nil if there are no more.
type Middleware func(next Handler) Handler

// Chain returns a Handler that first calls middleware in order.
func Chain(h Handler, ms ...Middleware) Handler {
	return HandlerFunc(func(i *Invokation) {
		if len(ms) == 0 {
			h.ServeCommand(i)
			return
		}
		Chain(ms[0](h), ms[1:]...).ServeCommand(i)
	})
}

func RequireNArgs(want int) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(i *Invokation) {
			if len(i.Args) != want {
				i.Exit(
					fmt.Errorf(
						"wanted %v args but got %v",
						want,
						len(i.Args),
					),
				)
			}
			next.ServeCommand(i)
		})
	}
}

// HandlerFunc is to Handler what http.HandlerFunc is to http.Handler.
type HandlerFunc func(i *Invokation)

func (h HandlerFunc) ServeCommand(i *Invokation) {
	h(i)
}

var _ Handler = HandlerFunc(nil)

// Handler describes the executable portion of a command. It
// is loosely based on the http.Handler interface.
type Handler interface {
	ServeCommand(i *Invokation)
}
