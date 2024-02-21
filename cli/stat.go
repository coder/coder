package cli

import (
	"fmt"
	"os"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clistat"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func initStatterMW(tgt **clistat.Statter, fs afero.Fs) serpent.MiddlewareFunc {
	return func(next serpent.HandlerFunc) serpent.HandlerFunc {
		return func(i *serpent.Invocation) error {
			var err error
			stat, err := clistat.New(clistat.WithFS(fs))
			if err != nil {
				return xerrors.Errorf("initialize workspace stats collector: %w", err)
			}
			*tgt = stat
			return next(i)
		}
	}
}

func (r *RootCmd) stat() *serpent.Cmd {
	var (
		st        *clistat.Statter
		fs        = afero.NewReadOnlyFs(afero.NewOsFs())
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat([]statsRow{}, []string{
				"host_cpu",
				"host_memory",
				"home_disk",
				"container_cpu",
				"container_memory",
			}),
			cliui.JSONFormat(),
		)
	)
	cmd := &serpent.Cmd{
		Use:        "stat",
		Short:      "Show resource usage for the current workspace.",
		Middleware: initStatterMW(&st, fs),
		Children: []*serpent.Cmd{
			r.statCPU(fs),
			r.statMem(fs),
			r.statDisk(fs),
		},
		Handler: func(inv *serpent.Invocation) error {
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
			ms, err := st.HostMemory(clistat.PrefixGibi)
			if err != nil {
				return err
			}
			sr.HostMemory = ms

			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			ds, err := st.Disk(clistat.PrefixGibi, home)
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

				ms, err := st.ContainerMemory(clistat.PrefixGibi)
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

func (*RootCmd) statCPU(fs afero.Fs) *serpent.Cmd {
	var (
		hostArg   bool
		st        *clistat.Statter
		formatter = cliui.NewOutputFormatter(cliui.TextFormat(), cliui.JSONFormat())
	)
	cmd := &serpent.Cmd{
		Use:        "cpu",
		Short:      "Show CPU usage, in cores.",
		Middleware: initStatterMW(&st, fs),
		Options: serpent.OptionSet{
			{
				Flag:        "host",
				Value:       serpent.BoolOf(&hostArg),
				Description: "Force host CPU measurement.",
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			var cs *clistat.Result
			var err error
			if ok, _ := clistat.IsContainerized(fs); ok && !hostArg {
				cs, err = st.ContainerCPU()
			} else {
				cs, err = st.HostCPU()
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

func (*RootCmd) statMem(fs afero.Fs) *serpent.Cmd {
	var (
		hostArg   bool
		prefixArg string
		st        *clistat.Statter
		formatter = cliui.NewOutputFormatter(cliui.TextFormat(), cliui.JSONFormat())
	)
	cmd := &serpent.Cmd{
		Use:        "mem",
		Short:      "Show memory usage, in gigabytes.",
		Middleware: initStatterMW(&st, fs),
		Options: serpent.OptionSet{
			{
				Flag:        "host",
				Value:       serpent.BoolOf(&hostArg),
				Description: "Force host memory measurement.",
			},
			{
				Description: "SI Prefix for memory measurement.",
				Default:     clistat.PrefixHumanGibi,
				Flag:        "prefix",
				Value: serpent.EnumOf(&prefixArg,
					clistat.PrefixHumanKibi,
					clistat.PrefixHumanMebi,
					clistat.PrefixHumanGibi,
					clistat.PrefixHumanTebi,
				),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			pfx := clistat.ParsePrefix(prefixArg)
			var ms *clistat.Result
			var err error
			if ok, _ := clistat.IsContainerized(fs); ok && !hostArg {
				ms, err = st.ContainerMemory(pfx)
			} else {
				ms, err = st.HostMemory(pfx)
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

func (*RootCmd) statDisk(fs afero.Fs) *serpent.Cmd {
	var (
		pathArg   string
		prefixArg string
		st        *clistat.Statter
		formatter = cliui.NewOutputFormatter(cliui.TextFormat(), cliui.JSONFormat())
	)
	cmd := &serpent.Cmd{
		Use:        "disk",
		Short:      "Show disk usage, in gigabytes.",
		Middleware: initStatterMW(&st, fs),
		Options: serpent.OptionSet{
			{
				Flag:        "path",
				Value:       serpent.StringOf(&pathArg),
				Description: "Path for which to check disk usage.",
				Default:     "/",
			},
			{
				Flag:        "prefix",
				Default:     clistat.PrefixHumanGibi,
				Description: "SI Prefix for disk measurement.",
				Value: serpent.EnumOf(&prefixArg,
					clistat.PrefixHumanKibi,
					clistat.PrefixHumanMebi,
					clistat.PrefixHumanGibi,
					clistat.PrefixHumanTebi,
				),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			pfx := clistat.ParsePrefix(prefixArg)
			// Users may also call `coder stat disk <path>`.
			if len(inv.Args) > 0 {
				pathArg = inv.Args[0]
			}
			ds, err := st.Disk(pfx, pathArg)
			if err != nil {
				if os.IsNotExist(err) {
					//nolint:gocritic // fmt.Errorf produces a more concise error.
					return fmt.Errorf("not found: %q", pathArg)
				}
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
