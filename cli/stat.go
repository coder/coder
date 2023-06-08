package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clistat"
	"github.com/coder/coder/cli/cliui"
)

func (*RootCmd) stat() *clibase.Cmd {
	defaultCols := []string{"host_cpu", "host_memory", "home_disk", "uptime"}
	if ok := clistat.IsContainerized(); ok != nil && *ok {
		// If running in a container, we assume that users want to see these first. Prepend.
		defaultCols = append([]string{"container_cpu", "container_memory"}, defaultCols...)
	}
	var (
		sampleInterval time.Duration
		formatter      = cliui.NewOutputFormatter(
			cliui.TableFormat([]statsRow{}, defaultCols),
			cliui.JSONFormat(),
		)
	)

	cmd := &clibase.Cmd{
		Use:   "stat",
		Short: "Show workspace resource usage.",
		Options: clibase.OptionSet{
			{
				Description: "Configure the sample interval.",
				Flag:        "sample-interval",
				Value:       clibase.DurationOf(&sampleInterval),
				Default:     "100ms",
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			var s *clistat.Statter
			if st, err := clistat.New(clistat.WithSampleInterval(sampleInterval)); err != nil {
				return err
			} else {
				s = st
			}

			var sr statsRow
			if cs, err := s.HostCPU(); err != nil {
				return err
			} else {
				sr.HostCPU = cs
			}

			if ms, err := s.HostMemory(); err != nil {
				return err
			} else {
				sr.HostMemory = ms
			}

			if home, err := os.UserHomeDir(); err != nil {
				return err
			} else if ds, err := s.Disk(home); err != nil {
				return err
			} else {
				sr.Disk = ds
			}

			if us, err := s.Uptime(); err != nil {
				return err
			} else {
				sr.Uptime = us
			}

			out, err := formatter.Format(inv.Context(), []statsRow{sr})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

type statsRow struct {
	HostCPU         *clistat.Result `json:"host_cpu" table:"host_cpu,default_sort"`
	HostMemory      *clistat.Result `json:"host_memory" table:"host_memory"`
	Disk            *clistat.Result `json:"home_disk" table:"home_disk"`
	ContainerCPU    *clistat.Result `json:"container_cpu" table:"container_cpu"`
	ContainerMemory *clistat.Result `json:"container_memory" table:"container_memory"`
	Uptime          *clistat.Result `json:"uptime" table:"uptime"`
}
