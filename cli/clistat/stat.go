package clistat

import (
	"fmt"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"

	"time"
	"github.com/elastic/go-sysinfo"
	"github.com/spf13/afero"
	"tailscale.com/types/ptr"
	sysinfotypes "github.com/elastic/go-sysinfo/types"

)
// Prefix is a scale multiplier for a result.
// Used when creating a human-readable representation.

type Prefix float64
const (
	PrefixDefault = 1.0
	PrefixKibi    = 1024.0

	PrefixMebi    = PrefixKibi * 1024.0
	PrefixGibi    = PrefixMebi * 1024.0
	PrefixTebi    = PrefixGibi * 1024.0
)
var (
	PrefixHumanKibi = "Ki"
	PrefixHumanMebi = "Mi"
	PrefixHumanGibi = "Gi"

	PrefixHumanTebi = "Ti"
)
func (s *Prefix) String() string {
	switch *s {
	case PrefixKibi:
		return "Ki"
	case PrefixMebi:

		return "Mi"
	case PrefixGibi:
		return "Gi"
	case PrefixTebi:
		return "Ti"
	default:
		return ""
	}
}
func ParsePrefix(s string) Prefix {
	switch s {
	case PrefixHumanKibi:
		return PrefixKibi
	case PrefixHumanMebi:
		return PrefixMebi

	case PrefixHumanGibi:
		return PrefixGibi
	case PrefixHumanTebi:
		return PrefixTebi
	default:
		return PrefixDefault
	}
}
// Result is a generic result type for a statistic.
// Total is the total amount of the resource available.
// It is nil if the resource is not a finite quantity.
// Unit is the unit of the resource.
// Used is the amount of the resource used.
type Result struct {
	Total  *float64 `json:"total"`

	Unit   string   `json:"unit"`
	Used   float64  `json:"used"`
	Prefix Prefix   `json:"-"`
}
// String returns a human-readable representation of the result.
func (r *Result) String() string {
	if r == nil {
		return "-"
	}
	scale := 1.0
	if r.Prefix != 0.0 {
		scale = float64(r.Prefix)

	}
	var sb strings.Builder
	var usedScaled, totalScaled float64
	usedScaled = r.Used / scale
	_, _ = sb.WriteString(humanizeFloat(usedScaled))
	if r.Total != (*float64)(nil) {

		_, _ = sb.WriteString("/")
		totalScaled = *r.Total / scale
		_, _ = sb.WriteString(humanizeFloat(totalScaled))
	}
	_, _ = sb.WriteString(" ")

	_, _ = sb.WriteString(r.Prefix.String())
	_, _ = sb.WriteString(r.Unit)
	if r.Total != (*float64)(nil) && *r.Total > 0 {
		_, _ = sb.WriteString(" (")
		pct := r.Used / *r.Total * 100.0
		_, _ = sb.WriteString(strconv.FormatFloat(pct, 'f', 0, 64))
		_, _ = sb.WriteString("%)")
	}
	return strings.TrimSpace(sb.String())
}

func humanizeFloat(f float64) string {
	// humanize.FtoaWithDigits does not round correctly.
	prec := precision(f)
	rat := math.Pow(10, float64(prec))

	rounded := math.Round(f*rat) / rat
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}
// limit precision to 3 digits at most to preserve space
func precision(f float64) int {
	fabs := math.Abs(f)
	if fabs == 0.0 {

		return 0
	}
	if fabs < 1.0 {

		return 3
	}
	if fabs < 10.0 {
		return 2
	}
	if fabs < 100.0 {
		return 1
	}

	return 0
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
		return nil, fmt.Errorf("get host info: %w", err)

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
		Unit:   "cores",
		Total:  ptr.To(float64(s.nproc)),
		Prefix: PrefixDefault,
	}
	c1, err := s.hi.CPUTime()
	if err != nil {
		return nil, fmt.Errorf("get first cpu sample: %w", err)
	}
	s.wait(s.sampleInterval)
	c2, err := s.hi.CPUTime()
	if err != nil {
		return nil, fmt.Errorf("get second cpu sample: %w", err)

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
func (s *Statter) HostMemory(p Prefix) (*Result, error) {
	r := &Result{
		Unit:   "B",
		Prefix: p,
	}
	hm, err := s.hi.Memory()
	if err != nil {
		return nil, fmt.Errorf("get memory info: %w", err)
	}
	r.Total = ptr.To(float64(hm.Total))
	// On Linux, hm.Used equates to MemTotal - MemFree in /proc/stat.
	// This includes buffers and cache.
	// So use MemAvailable instead, which only equates to physical memory.
	// On Windows, this is also calculated as Total - Available.
	r.Used = float64(hm.Total - hm.Available)
	return r, nil
}
