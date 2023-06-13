package clistat

import (
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-sysinfo"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"
	"tailscale.com/types/ptr"

	sysinfotypes "github.com/elastic/go-sysinfo/types"
)

// Result is a generic result type for a statistic.
// Total is the total amount of the resource available.
// It is nil if the resource is not a finite quantity.
// Unit is the unit of the resource.
// Used is the amount of the resource used.
type Result struct {
	Total *float64 `json:"total"`
	Unit  string   `json:"unit"`
	Used  float64  `json:"used"`
}

// String returns a human-readable representation of the result.
func (r *Result) String() string {
	if r == nil {
		return "-"
	}
	var sb strings.Builder
	_, _ = sb.WriteString(strconv.FormatFloat(r.Used, 'f', 1, 64))
	if r.Total != (*float64)(nil) {
		_, _ = sb.WriteString("/")
		_, _ = sb.WriteString(strconv.FormatFloat(*r.Total, 'f', 1, 64))
	}
	if r.Unit != "" {
		_, _ = sb.WriteString(" ")
		_, _ = sb.WriteString(r.Unit)
	}
	return sb.String()
}

// Statter is a system statistics collector.
// It is a thin wrapper around the elastic/go-sysinfo library.
type Statter struct {
	hi             sysinfotypes.Host
	fs             afero.Fs
	sampleInterval time.Duration
	nproc          int
	wait           func(time.Duration)
}

type Option func(*Statter)

// WithSampleInterval sets the sample interval for the statter.
func WithSampleInterval(d time.Duration) Option {
	return func(s *Statter) {
		s.sampleInterval = d
	}
}

// WithFS sets the fs for the statter.
func WithFS(fs afero.Fs) Option {
	return func(s *Statter) {
		s.fs = fs
	}
}

func New(opts ...Option) (*Statter, error) {
	hi, err := sysinfo.Host()
	if err != nil {
		return nil, xerrors.Errorf("get host info: %w", err)
	}
	s := &Statter{
		hi:             hi,
		fs:             afero.NewReadOnlyFs(afero.NewOsFs()),
		sampleInterval: 100 * time.Millisecond,
		nproc:          runtime.NumCPU(),
		wait: func(d time.Duration) {
			<-time.After(d)
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// HostCPU returns the CPU usage of the host. This is calculated by
// taking two samples of CPU usage and calculating the difference.
// Total will always be equal to the number of cores.
// Used will be an estimate of the number of cores used during the sample interval.
// This is calculated by taking the difference between the total and idle HostCPU time
// and scaling it by the number of cores.
// Units are in "cores".
func (s *Statter) HostCPU() (*Result, error) {
	r := &Result{
		Unit:  "cores",
		Total: ptr.To(float64(s.nproc)),
	}
	c1, err := s.hi.CPUTime()
	if err != nil {
		return nil, xerrors.Errorf("get first cpu sample: %w", err)
	}
	s.wait(s.sampleInterval)
	c2, err := s.hi.CPUTime()
	if err != nil {
		return nil, xerrors.Errorf("get second cpu sample: %w", err)
	}
	total := c2.Total() - c1.Total()
	idle := c2.Idle - c1.Idle
	used := total - idle
	scaleFactor := float64(s.nproc) / total.Seconds()
	r.Used = used.Seconds() * scaleFactor
	return r, nil
}

// HostMemory returns the memory usage of the host, in gigabytes.
func (s *Statter) HostMemory() (*Result, error) {
	r := &Result{
		Unit: "GB",
	}
	hm, err := s.hi.Memory()
	if err != nil {
		return nil, xerrors.Errorf("get memory info: %w", err)
	}
	r.Total = ptr.To(float64(hm.Total) / 1024 / 1024 / 1024)
	r.Used = float64(hm.Used) / 1024 / 1024 / 1024
	return r, nil
}
