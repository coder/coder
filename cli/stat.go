package cli

import (
	"fmt"
	"os"

	"github.com/spf13/afero"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clistat"
	"github.com/coder/coder/cli/cliui"
)

func (*RootCmd) stat() *clibase.Cmd {
	fs := afero.NewReadOnlyFs(afero.NewOsFs())
	defaultCols := []string{"host_cpu", "host_memory", "home_disk", "container_cpu", "container_memory"}
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]statsRow{}, defaultCols),
		cliui.JSONFormat(),
	)

	cmd := &clibase.Cmd{
		Use:     "stat",
		Short:   "Show workspace resource usage.",
		Options: clibase.OptionSet{},
		Handler: func(inv *clibase.Invocation) error {
			st, err := clistat.New(clistat.WithFS(fs))
			if err != nil {
				return err
			}

			var sr statsRow

			// Get CPU measurements first.
			hostErr := make(chan error)
			containerErr := make(chan error)
			go func() {
				defer close(hostErr)
				cs, err := st.HostCPU()
				if err != nil {
					hostErr <- err
					return
				}
				sr.HostCPU = cs
			}()
			go func() {
				defer close(containerErr)
				if ok, _ := clistat.IsContainerized(fs); !ok {
					// don't error if we're not in a container
					return
				}
				cs, err := st.ContainerCPU()
				if err != nil {
					containerErr <- err
					return
				}
				sr.ContainerCPU = cs
			}()

			if err := <-hostErr; err != nil {
				return err
			}
			if err := <-containerErr; err != nil {
				return err
			}

			// Host-level stats
			ms, err := st.HostMemory()
			if err != nil {
				return err
			}
			sr.HostMemory = ms

			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			ds, err := st.Disk(home)
			if err != nil {
				return err
			}
			sr.Disk = ds

			// Container-only stats.
			if ok, err := clistat.IsContainerized(fs); err == nil && ok {
				cs, err := st.ContainerCPU()
				if err != nil {
					return err
				}
				sr.ContainerCPU = cs

				ms, err := st.ContainerMemory()
				if err != nil {
					return err
				}
				sr.ContainerMemory = ms
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
}
