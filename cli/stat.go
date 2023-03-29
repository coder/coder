package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

type statCmd struct {
	*RootCmd
	watch time.Duration
}

func (r *RootCmd) stat() *clibase.Cmd {
	c := &clibase.Cmd{
		Use:   "stat <type> [flags...]",
		Short: "Display local system resource usage statistics",
		Long:  "stat calls can be used as the script for agent metadata blocks.",
	}
	sc := statCmd{RootCmd: r}
	c.Options.Add(
		clibase.Option{
			Flag:          "watch",
			FlagShorthand: "w",
			Description:   "Continuously display the statistic on the given interval.",
			Value:         clibase.DurationOf(&sc.watch),
		},
	)
	c.AddSubcommands(
		sc.cpu(),
	)
	sc.setWatchLoops(c)
	return c
}

func (sc *statCmd) setWatchLoops(c *clibase.Cmd) {
	for _, cmd := range c.Children {
		innerHandler := cmd.Handler
		cmd.Handler = func(inv *clibase.Invocation) error {
			if sc.watch == 0 {
				return innerHandler(inv)
			}

			ticker := time.NewTicker(sc.watch)
			defer ticker.Stop()

			for range ticker.C {
				if err := innerHandler(inv); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "error: %v", err)
				}
			}
			panic("unreachable")
		}
	}
}

func cpuUsageFromCgroup(interval time.Duration) (float64, error) {
	cgroup, err := os.OpenFile("/proc/self/cgroup", os.O_RDONLY, 0)
	if err != nil {
		return 0, err
	}
	defer cgroup.Close()
	sc := bufio.NewScanner(cgroup)

	var groupDir string
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) != 3 {
			continue
		}
		if fields[1] != "cpu,cpuacct" {
			continue
		}
		groupDir = fields[2]
		break
	}

	if groupDir == "" {
		return 0, xerrors.New("no cpu cgroup found")
	}

	cpuAcct := func() (int64, error) {
		path := fmt.Sprintf("/sys/fs/cgroup/cpu,cpuacct/%s/cpuacct.usage", groupDir)

		byt, err := os.ReadFile(
			path,
		)
		if err != nil {
			return 0, err
		}

		return strconv.ParseInt(string(bytes.TrimSpace(byt)), 10, 64)
	}

	stat1, err := cpuAcct()
	if err != nil {
		return 0, err
	}

	time.Sleep(interval)

	stat2, err := cpuAcct()
	if err != nil {
		return 0, err
	}

	var (
		cpuTime  = time.Duration(stat2 - stat1)
		realTime = interval
	)

	ncpu, err := cpu.Counts(true)
	if err != nil {
		return 0, err
	}

	return (cpuTime.Seconds() / realTime.Seconds()) * 100 / float64(ncpu), nil
}

//nolint:revive
func (sc *statCmd) cpu() *clibase.Cmd {
	var interval time.Duration
	c := &clibase.Cmd{
		Use:     "cpu-usage",
		Aliases: []string{"cu"},
		Short:   "Display the system's cpu usage",
		Long:    "If inside a cgroup (e.g. docker container), the cpu usage is ",
		Handler: func(inv *clibase.Invocation) error {
			if sc.watch != 0 {
				interval = sc.watch
			}

			r, err := cpuUsageFromCgroup(interval)
			if err != nil {
				cliui.Infof(sc.verboseStderr(inv), "cgroup error: %+v", err)

				// Use standard methods if cgroup method fails.
				rs, err := cpu.Percent(interval, false)
				if err != nil {
					return err
				}
				r = rs[0]
			}

			_, _ = fmt.Fprintf(inv.Stdout, "%02.0f\n", r)

			return nil
		},
		Options: []clibase.Option{
			{
				Flag:          "interval",
				FlagShorthand: "i",
				Description:   `The sample collection interval. If --watch is set, it overrides this value.`,
				Default:       "0s",
				Value:         clibase.DurationOf(&interval),
			},
		},
	}
	return c
}
