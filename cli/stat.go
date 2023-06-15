package cli

import (
	"fmt"
	"os"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clistat"
	"github.com/coder/coder/cli/cliui"
)

func (r *RootCmd) stat() *clibase.Cmd {
	fs := afero.NewReadOnlyFs(afero.NewOsFs())
	defaultCols := []string{
		"host_cpu",
		"host_memory",
		"home_disk",
		"container_cpu",
		"container_memory",
	}
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]statsRow{}, defaultCols),
		cliui.JSONFormat(),
	)
	st, err := clistat.New(clistat.WithFS(fs))
	if err != nil {
		panic(xerrors.Errorf("initialize workspace stats collector: %w", err))
	}

	cmd := &clibase.Cmd{
		Use:   "stat",
		Short: "Show resource usage for the current workspace.",
		Children: []*clibase.Cmd{
			r.statCPU(st, fs),
			r.statMem(st, fs),
			r.statDisk(st),
		},
		Handler: func(inv *clibase.Invocation) error {
			var sr statsRow

			// Get CPU measurements first.
			hostErr := make(chan error, 1)
			containerErr := make(chan error, 1)
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

func (*RootCmd) statCPU(s *clistat.Statter, fs afero.Fs) *clibase.Cmd {
	var hostArg bool
	var prefixArg string
	formatter := cliui.NewOutputFormatter(cliui.TextFormat(), cliui.JSONFormat())
	cmd := &clibase.Cmd{
		Use:   "cpu",
		Short: "Show CPU usage, in cores.",
		Options: clibase.OptionSet{
			{
				Flag:        "host",
				Value:       clibase.BoolOf(&hostArg),
				Description: "Force host CPU measurement.",
			},
			{
				Flag:        "prefix",
				Value:       clibase.StringOf(&prefixArg),
				Description: "Unit prefix.",
				Default:     "",
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			var cs *clistat.Result
			var err error
			if ok, _ := clistat.IsContainerized(fs); ok && !hostArg {
				cs, err = s.ContainerCPU()
			} else {
				cs, err = s.HostCPU()
			}
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), cs)
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

func (*RootCmd) statMem(s *clistat.Statter, fs afero.Fs) *clibase.Cmd {
	var hostArg bool
	formatter := cliui.NewOutputFormatter(cliui.TextFormat(), cliui.JSONFormat())
	cmd := &clibase.Cmd{
		Use:   "mem",
		Short: "Show memory usage, in gigabytes.",
		Options: clibase.OptionSet{
			{
				Flag:        "host",
				Value:       clibase.BoolOf(&hostArg),
				Description: "Force host memory measurement.",
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			var ms *clistat.Result
			var err error
			if ok, _ := clistat.IsContainerized(fs); ok && !hostArg {
				ms, err = s.ContainerMemory()
			} else {
				ms, err = s.HostMemory()
			}
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), ms)
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

func (*RootCmd) statDisk(s *clistat.Statter) *clibase.Cmd {
	var pathArg string
	formatter := cliui.NewOutputFormatter(cliui.TextFormat(), cliui.JSONFormat())
	cmd := &clibase.Cmd{
		Use:   "disk",
		Short: "Show disk usage, in gigabytes.",
		Options: clibase.OptionSet{
			{
				Flag:        "path",
				Value:       clibase.StringOf(&pathArg),
				Description: "Path for which to check disk usage.",
				Default:     "/",
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			ds, err := s.Disk(pathArg)
			if err != nil {
				return err
			}

			out, err := formatter.Format(inv.Context(), ds)
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
