package cli

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-sysinfo"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

func (*RootCmd) stat() *clibase.Cmd {
	var (
		sampleInterval time.Duration
		formatter      = cliui.NewOutputFormatter(
			cliui.TextFormat(),
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
			stats, err := newStats(sampleInterval)
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), stats)
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

type stats struct {
	HostCPU         stat `json:"cpu_host"`
	HostMemory      stat `json:"mem_host"`
	Disk            stat `json:"disk"`
	InContainer     bool `json:"in_container,omitempty"`
	ContainerCPU    stat `json:"cpu_container,omitempty"`
	ContainerMemory stat `json:"mem_container,omitempty"`
}

func (s *stats) String() string {
	var sb strings.Builder
	sb.WriteString(s.HostCPU.String())
	sb.WriteString("\n")
	sb.WriteString(s.HostMemory.String())
	sb.WriteString("\n")
	sb.WriteString(s.Disk.String())
	sb.WriteString("\n")
	if s.InContainer {
		sb.WriteString(s.ContainerCPU.String())
		sb.WriteString("\n")
		sb.WriteString(s.ContainerMemory.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

func newStats(dur time.Duration) (stats, error) {
	var s stats
	nproc := float64(runtime.NumCPU())
	// start := time.Now()
	// ticksPerDur := dur / tickInterval
	h1, err := sysinfo.Host()
	if err != nil {
		return s, err
	}
	<-time.After(dur)
	h2, err := sysinfo.Host()
	if err != nil {
		return s, err
	}
	// elapsed := time.Since(start)
	// numTicks := elapsed / tickInterval
	cts1, err := h1.CPUTime()
	if err != nil {
		return s, err
	}
	cts2, err := h2.CPUTime()
	if err != nil {
		return s, err
	}
	// Assuming the total measured should add up to $(nproc) "cores",
	// we determine a scaling factor such that scaleFactor * total = nproc.
	// We then calculate used as the total time spent idle, and multiply
	// that by scaleFactor to give a rough approximation of how busy the
	// CPU(s) were.
	s.HostCPU.Total = nproc
	total := (cts2.Total() - cts1.Total())
	idle := (cts2.Idle - cts1.Idle)
	used := total - idle
	scaleFactor := nproc / total.Seconds()
	s.HostCPU.Used = used.Seconds() * scaleFactor
	s.HostCPU.Unit = "cores"

	return s, nil
}

type stat struct {
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
	Unit  string  `json:"unit"`
}

func (s *stat) String() string {
	var sb strings.Builder
	_, _ = sb.WriteString(strconv.FormatFloat(s.Used, 'f', 1, 64))
	_, _ = sb.WriteString("/")
	_, _ = sb.WriteString(strconv.FormatFloat(s.Total, 'f', 1, 64))
	_, _ = sb.WriteString(" ")
	if s.Unit != "" {
		_, _ = sb.WriteString(s.Unit)
		_, _ = sb.WriteString(" ")
	}
	_, _ = sb.WriteString("(")
	var pct float64
	if s.Total == 0 {
		pct = math.NaN()
	} else {
		pct = s.Used / s.Total * 100
	}
	_, _ = sb.WriteString(strconv.FormatFloat(pct, 'f', 1, 64))
	_, _ = sb.WriteString("%)")
	return sb.String()
}
