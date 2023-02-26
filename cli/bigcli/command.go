package bigcli

import "strings"

// Command describes an executable command.
type Command struct {
	// Parents is a list of parent commands, with
	// the root command at index 0.
	Parents []*Command
	// Use is provided in form "command [flags] [args...]".
	Use string
	// Short is a one-line description of the command.
	Short string
	// Long is a detailed description of the command,
	// presented on its help page. It may contain examples.
	Long    string
	Options OptionSet
}

// Name returns the first word in the Use string.
func (c *Command) Name() string {
	return strings.Split(c.Use, " ")[0]
}

// FullName returns the full invokation name of the command,
// as seen on the command line.
func (c *Command) FullName() string {
	var names []string
	for _, p := range c.Parents {
		names = append(names, p.Name())
	}
	names = append(names, c.Name())
	return strings.Join(names, " ")
}

// FullName returns usage of the command, preceded
// by the usage of its parents.
func (c *Command) FullUsage() string {
	var uses []string
	for _, p := range c.Parents {
		uses = append(uses, p.Use)
	}
	uses = append(uses, c.Use)
	return strings.Join(uses, " ")
}

// Parent returns the direct parent of the command,
// or nil if there are no parents.
func (c *Command) Parent() *Command {
	if len(c.Parents) == 0 {
		return nil
	}
	return c.Parents[len(c.Parents)-1]
}
