package cli

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-sysinfo"
	sysinfotypes "github.com/elastic/go-sysinfo/types"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

func (*RootCmd) stat() *clibase.Cmd {
	defaultCols := []string{"host_cpu", "host_memory", "disk"}
	if isContainerized() {
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
			hi, err := sysinfo.Host()
			if err != nil {
				return err
			}
			sr := statsRow{}
			if cs, err := statCPU(hi, sampleInterval); err != nil {
				return err
			} else {
				sr.HostCPU = cs
			}
			if ms, err := statMem(hi); err != nil {
				return err
			} else {
				sr.HostMemory = ms
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

func statCPU(hi sysinfotypes.Host, interval time.Duration) (*stat, error) {
	nproc := float64(runtime.NumCPU())
	s := &stat{
		Unit: "cores",
	}
	c1, err := hi.CPUTime()
	if err != nil {
		return nil, err
	}
	<-time.After(interval)
	c2, err := hi.CPUTime()
	if err != nil {
		return nil, err
	}
	s.Total = nproc
	total := c2.Total() - c1.Total()
	idle := c2.Idle - c1.Idle
	used := total - idle
	scaleFactor := nproc / total.Seconds()
	s.Used = used.Seconds() * scaleFactor
	return s, nil
}

func statMem(hi sysinfotypes.Host) (*stat, error) {
	s := &stat{
		Unit: "GB",
	}
	hm, err := hi.Memory()
	if err != nil {
		return nil, err
	}
	s.Total = float64(hm.Total) / 1024 / 1024 / 1024
	s.Used = float64(hm.Used) / 1024 / 1024 / 1024
	return s, nil
}

type statsRow struct {
	HostCPU         *stat `json:"host_cpu" table:"host_cpu,default_sort"`
	HostMemory      *stat `json:"host_memory" table:"host_memory"`
	Disk            *stat `json:"disk" table:"disk"`
	ContainerCPU    *stat `json:"container_cpu" table:"container_cpu"`
	ContainerMemory *stat `json:"container_memory" table:"container_memory"`
}

type stat struct {
	Total float64 `json:"total"`
	Unit  string  `json:"unit"`
	Used  float64 `json:"used"`
}

func (s *stat) String() string {
	if s == nil {
		return "-"
	}
	var sb strings.Builder
	_, _ = sb.WriteString(strconv.FormatFloat(s.Used, 'f', 1, 64))
	_, _ = sb.WriteString("/")
	_, _ = sb.WriteString(strconv.FormatFloat(s.Total, 'f', 1, 64))
	_, _ = sb.WriteString(" ")
	if s.Unit != "" {
		_, _ = sb.WriteString(s.Unit)
	}
	return sb.String()
}

func isContainerized() bool {
	hi, err := sysinfo.Host()
	if err != nil {
		// If we can't get the host info, we have other issues.
		panic(err)
	}
	return hi.Info().Containerized != nil && *hi.Info().Containerized
}
