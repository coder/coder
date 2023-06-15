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

// Prefix is an SI prefix for a unit.
type Prefix string

// Float64 returns the prefix as a float64.
func (m *Prefix) Float64() (float64, error) {
	switch *m {
	case PrefixDeciShort, PrefixDeci:
		return 0.1, nil
	case PrefixCentiShort, PrefixCenti:
		return 0.01, nil
	case PrefixMilliShort, PrefixMilli:
		return 0.001, nil
	case PrefixMicroShort, PrefixMicro:
		return 0.000_001, nil
	case PrefixNanoShort, PrefixNano:
		return 0.000_000_001, nil
	case PrefixKiloShort, PrefixKilo:
		return 1_000.0, nil
	case PrefixMegaShort, PrefixMega:
		return 1_000_000.0, nil
	case PrefixGigaShort, PrefixGiga:
		return 1_000_000_000.0, nil
	case PrefixTeraShort, PrefixTera:
		return 1_000_000_000_000.0, nil
	case PrefixKibiShort, PrefixKibi:
		return 1024.0, nil
	case PrefixMebiShort, PrefixMebi:
		return 1_048_576.0, nil
	case PrefixGibiShort, PrefixGibi:
		return 1_073_741_824.0, nil
	case PrefixTebiShort, PrefixTebi:
		return 1_099_511_627_776.0, nil
	default:
		return 0, xerrors.Errorf("unknown prefix: %s", *m)
	}
}

const (
	PrefixDeci  Prefix = "deci"
	PrefixCenti Prefix = "centi"
	PrefixMilli Prefix = "milli"
	PrefixMicro Prefix = "micro"
	PrefixNano  Prefix = "nano"

	PrefixDeciShort  Prefix = "d"
	PrefixCentiShort Prefix = "c"
	PrefixMilliShort Prefix = "m"
	PrefixMicroShort Prefix = "u"
	PrefixNanoShort  Prefix = "n"

	PrefixKilo Prefix = "kilo"
	PrefixMega Prefix = "mega"
	PrefixGiga Prefix = "giga"
	PrefixTera Prefix = "tera"

	PrefixKiloShort Prefix = "K"
	PrefixMegaShort Prefix = "M"
	PrefixGigaShort Prefix = "G"
	PrefixTeraShort Prefix = "T"

	PrefixKibi = "kibi"
	PrefixMebi = "mebi"
	PrefixGibi = "gibi"
	PrefixTebi = "tebi"

	PrefixKibiShort Prefix = "Ki"
	PrefixMebiShort Prefix = "Mi"
	PrefixGibiShort Prefix = "Gi"
	PrefixTebiShort Prefix = "Ti"
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
	// Prefix controls the string representation of the result.
	Prefix Prefix `json:"-"`
}

// String returns a human-readable representation of the result.
func (r *Result) String() string {
	if r == nil {
		return "-"
	}
	var sb strings.Builder
	scale, err := r.Prefix.Float64()
	prefix := string(r.Prefix)
	if err != nil {
		prefix = ""
		scale = 1.0
	}
	_, _ = sb.WriteString(strconv.FormatFloat(r.Used/scale, 'f', 1, 64))
	if r.Total != (*float64)(nil) {
		_, _ = sb.WriteString("/")
		_, _ = sb.WriteString(strconv.FormatFloat(*r.Total/scale, 'f', 1, 64))
	}
	if r.Unit != "" {
		_, _ = sb.WriteString(" ")
		_, _ = sb.WriteString(prefix)
		_, _ = sb.WriteString(r.Unit)
	}
	if r.Total != (*float64)(nil) && *r.Total != 0.0 {
		_, _ = sb.WriteString(" (")
		_, _ = sb.WriteString(strconv.FormatFloat(100.0*r.Used/(*r.Total), 'f', 0, 64))
		_, _ = sb.WriteString("%)")
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
func (s *Statter) HostCPU(m Prefix) (*Result, error) {
	r := &Result{
		Unit:   "cores",
		Total:  ptr.To(float64(s.nproc)),
		Prefix: m,
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
	if total == 0 {
		return r, nil // no change
	}
	idle := c2.Idle - c1.Idle
	used := total - idle
	scaleFactor := float64(s.nproc) / total.Seconds()
	r.Used = used.Seconds() * scaleFactor
	return r, nil
}

// HostMemory returns the memory usage of the host, in gigabytes.
func (s *Statter) HostMemory(m Prefix) (*Result, error) {
	r := &Result{
		Unit:   "B",
		Prefix: m,
	}
	hm, err := s.hi.Memory()
	if err != nil {
		return nil, xerrors.Errorf("get memory info: %w", err)
	}
	r.Total = ptr.To(float64(hm.Total))
	r.Used = float64(hm.Used)
	return r, nil
}
