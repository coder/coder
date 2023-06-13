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
	defaultCols := []string{"host_cpu", "host_memory", "home_disk"}
	if ok, err := clistat.IsContainerized(fs); err == nil && ok {
		// If running in a container, we assume that users want to see these first. Prepend.
		defaultCols = append([]string{"container_cpu", "container_memory"}, defaultCols...)
	}
	var (
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat([]statsRow{}, defaultCols),
			cliui.JSONFormat(),
		)
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
			errCh := make(chan error, 2)
			go func() {
				cs, err := st.HostCPU()
				if err != nil {
					errCh <- err
					return
				}
				sr.HostCPU = cs
				errCh <- nil
			}()
			go func() {
				if ok, _ := clistat.IsContainerized(fs); !ok {
					errCh <- nil
				}
				cs, err := st.ContainerCPU()
				if err != nil {
					errCh <- err
					return
				}
				sr.ContainerCPU = cs
				errCh <- nil
			}()

			if err1 := <-errCh; err1 != nil {
				return err
			}
			if err2 := <-errCh; err2 != nil {
				return err
			}
			close(errCh)

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
