package cli

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/cpu"

	"github.com/coder/coder/cli/clibase"
)

type statCmd struct {
	watch time.Duration
}

func (*RootCmd) stat() *clibase.Cmd {
	c := &clibase.Cmd{
		Use:    "stat <type> [flags...]",
		Short:  "Display system resource usage statistics",
		Long:   "stat can be used as the script for agent metadata blocks.",
		Hidden: true,
	}
	var statCmd statCmd
	c.Options.Add(
		clibase.Option{
			Flag:          "watch",
			FlagShorthand: "w",
			Description:   "Continuously display the statistic on the given interval.",
			Value:         clibase.DurationOf(&statCmd.watch),
		},
	)
	c.AddSubcommands(
		statCmd.cpu(),
	)
	return c
}

func (sc *statCmd) watchLoop(fn clibase.HandlerFunc) clibase.HandlerFunc {
	return func(inv *clibase.Invocation) error {
		if sc.watch == 0 {
			return fn(inv)
		}

		ticker := time.NewTicker(sc.watch)
		defer ticker.Stop()

		for range ticker.C {
			if err := fn(inv); err != nil {
				_, _ = fmt.Fprintf(inv.Stderr, "error: %v", err)
			}
		}
		panic("unreachable")
	}
}

//nolint:revive
func (sc *statCmd) cpu() *clibase.Cmd {
	var interval time.Duration
	c := &clibase.Cmd{
		Use:   "cpu",
		Short: "Display the system's cpu usage",
		Long:  "Display the system's load average.",
		Handler: sc.watchLoop(func(inv *clibase.Invocation) error {
			r, err := cpu.Percent(0, false)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(inv.Stdout, "%02.0f\n", r[0])
			return nil
		}),
		Options: []clibase.Option{
			{
				Flag:          "interval",
				FlagShorthand: "i",
				Description:   "The sample collection interval.",
				Default:       "1s",
				Value:         clibase.DurationOf(&interval),
			},
		},
	}
	return c
}
