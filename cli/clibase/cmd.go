package clibase

import "strings"

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
	// Long is a detailed description of the command,
	// presented on its help page. It may contain examples.
	Long        string
	Options     OptionSet
	Annotations Annotations
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
