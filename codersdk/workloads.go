package codersdk

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// WorkloadGeneration identifies a prepared, immutable project environment.
type WorkloadGeneration struct {
	ID           string            `json:"id"`
	Repositories map[string]string `json:"repositories"`
	Workdir      string            `json:"workdir"`
}

// WorkloadHostInfo describes locally validated prepared capacity.
type WorkloadHostInfo struct {
	Generation           WorkloadGeneration `json:"generation"`
	Runtime              string             `json:"runtime"`
	CPU                  int64              `json:"cpu"`
	MemoryBytes          int64              `json:"memory_bytes"`
	AvailableCPU         int64              `json:"available_cpu"`
	AvailableMemoryBytes int64              `json:"available_memory_bytes"`
	Draining             bool               `json:"draining"`
}

// CreateWorkloadRequest is the normalized run request. ID is an idempotency key.
type CreateWorkloadRequest struct {
	ID          uuid.UUID         `json:"id" format:"uuid"`
	TemplateID  uuid.UUID         `json:"template_id" format:"uuid"`
	CPU         int64             `json:"cpu"`
	MemoryBytes int64             `json:"memory_bytes"`
	Command     string            `json:"command"`
	Env         map[string]string `json:"env,omitempty"`
	Stdin       bool              `json:"stdin,omitempty"`
	TTY         bool              `json:"tty,omitempty"`
	Width       uint16            `json:"width,omitempty"`
	Height      uint16            `json:"height,omitempty"`
}

// Validate rejects unsupported workload requests before admission.
func (r CreateWorkloadRequest) Validate() error {
	if r.ID == uuid.Nil || r.TemplateID == uuid.Nil {
		return xerrors.New("id and template_id must be nonzero UUIDs")
	}
	if r.CPU <= 0 || r.CPU > math.MaxInt64/1_000_000_000 {
		return xerrors.New("cpu must be a positive whole number within the supported range")
	}
	if r.MemoryBytes < 16<<20 || r.MemoryBytes%(1<<20) != 0 {
		return xerrors.New("ram must be at least 16MiB and a whole multiple of MiB")
	}
	if strings.TrimSpace(r.Command) == "" || len(r.Command) > 64<<10 || strings.ContainsRune(r.Command, 0) {
		return xerrors.New("cmd must be nonempty, contain no NUL, and be at most 64KiB")
	}
	if len(r.Env) > 256 {
		return xerrors.New("env must contain at most 256 variables")
	}
	envBytes := 0
	for key, value := range r.Env {
		if !workloadEnvKey.MatchString(key) || strings.ContainsRune(value, 0) {
			return xerrors.New("env keys must be portable variable names and values must contain no NUL")
		}
		envBytes += len(key) + len(value)
		if envBytes > 64<<10 {
			return xerrors.New("env keys and values must total at most 64KiB")
		}
	}
	if r.TTY && (!r.Stdin || r.Width == 0 || r.Height == 0) {
		return xerrors.New("tty requires stdin and nonzero terminal dimensions")
	}
	if !r.TTY && (r.Width != 0 || r.Height != 0) {
		return xerrors.New("terminal dimensions require tty")
	}
	return nil
}

// WorkloadInput carries terminal input without retaining or replaying keystrokes.
// Type is data, resize, or eof. Data messages contain at most 16 KiB.
// Resize messages require nonzero Width and Height.
// EOF closes stdin but does not cancel the workload.
// @typescript-ignore WorkloadInput
type WorkloadInput struct {
	Type   string `json:"type"`
	Data   []byte `json:"data,omitempty" swaggertype:"string" format:"base64"`
	Width  uint16 `json:"width,omitempty"`
	Height uint16 `json:"height,omitempty"`
}

var (
	workloadCPUQuantity = regexp.MustCompile(`^[1-9][0-9]*$`)
	workloadRAMQuantity = regexp.MustCompile(`^([1-9][0-9]*)(MiB|GiB)$`)
	workloadEnvKey      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ParseWorkloadResources accepts whole CPUs and integer MiB or GiB quantities.
// CPU denotes a CFS quota, not exclusive physical cores.
func ParseWorkloadResources(cpu, ram string) (cpuCount, memoryBytes int64, err error) {
	if !workloadCPUQuantity.MatchString(cpu) {
		return 0, 0, xerrors.New("--cpu requires a positive whole number, for example 2")
	}
	cpus, err := strconv.ParseInt(cpu, 10, 64)
	if err != nil || cpus > math.MaxInt64/1_000_000_000 {
		return 0, 0, xerrors.New("--cpu exceeds the supported range")
	}
	parts := workloadRAMQuantity.FindStringSubmatch(ram)
	if parts == nil {
		return 0, 0, xerrors.New("--ram requires an integer with MiB or GiB units, for example 512MiB or 4GiB")
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	unit := int64(1 << 20)
	if parts[2] == "GiB" {
		unit = 1 << 30
	}
	if err != nil || size > math.MaxInt64/unit {
		return 0, 0, xerrors.New("--ram exceeds the supported range")
	}
	if size*unit < 16<<20 {
		return 0, 0, xerrors.New("--ram must be at least 16MiB")
	}
	return cpus, size * unit, nil
}

// StartHostWorkloadRequest pins placement before invoking the host supervisor.
type StartHostWorkloadRequest struct {
	CreateWorkloadRequest
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`
	OwnerID        uuid.UUID `json:"owner_id" format:"uuid"`
	GenerationID   string    `json:"generation_id"`
	// Network preserves historical request metadata; it does not select networking.
	Network string `json:"network,omitempty"`
}

// WorkloadTimings records measured durations in nanoseconds. Missing fields
// indicate an unavailable measurement, not a zero duration. Connection timings
// belong to the server; provision and execution timings belong to the host.
type WorkloadTimings struct {
	HostConnectNS        *int64 `json:"host_connect_ns,omitempty"`
	HostConnectionReused *bool  `json:"host_connection_reused,omitempty"`
	ProvisionNS          *int64 `json:"provision_ns,omitempty"`
	ExecNS               *int64 `json:"exec_ns,omitempty"`
}

// Workload is a run's identity and observed execution outcome. Command text and
// environment values are deliberately omitted from responses and diagnostics.
type Workload struct {
	ID             uuid.UUID          `json:"id" format:"uuid"`
	OrganizationID uuid.UUID          `json:"organization_id" format:"uuid"`
	OwnerID        uuid.UUID          `json:"owner_id" format:"uuid"`
	TemplateID     uuid.UUID          `json:"template_id" format:"uuid"`
	AgentID        uuid.UUID          `json:"agent_id" format:"uuid"`
	Generation     WorkloadGeneration `json:"generation"`
	CPU            int64              `json:"cpu"`
	MemoryBytes    int64              `json:"memory_bytes"`
	State          string             `json:"state"`
	CreatedAt      time.Time          `json:"created_at" format:"date-time"`
	StartedAt      *time.Time         `json:"started_at,omitempty" format:"date-time"`
	FinishedAt     *time.Time         `json:"finished_at,omitempty" format:"date-time"`
	ExitCode       *int               `json:"exit_code,omitempty"`
	Failure        string             `json:"failure,omitempty"`
	CleanupPending bool               `json:"cleanup_pending"`
	Timings        WorkloadTimings    `json:"timings"`
}

// Terminal reports whether command execution has a final observed outcome.
func (w Workload) Terminal() bool {
	switch w.State {
	case "succeeded", "failed", "canceled", "rejected", "unknown":
		return true
	default:
		return false
	}
}

// WorkloadOutput is a bounded, ordered batch; offsets count events, not bytes.
type WorkloadOutput struct {
	Workload   Workload        `json:"workload"`
	Events     []WorkloadEvent `json:"events"`
	NextOffset int64           `json:"next_offset"`
	Truncated  bool            `json:"truncated"`
}

// WorkloadEvent retains stdout and stderr separately without interpreting bytes.
type WorkloadEvent struct {
	Stream string `json:"stream"`
	Data   []byte `json:"data" swaggertype:"string" format:"base64"`
}

// RegisterWorkloadHostRequest explicitly enrolls a managed service workspace.
// Token is a dedicated supervisor credential, never a workspace agent token.
type RegisterWorkloadHostRequest struct {
	AgentID uuid.UUID `json:"agent_id" format:"uuid"`
	Token   string    `json:"token"`
}

// RegisteredWorkloadHost identifies a validated host without exposing credentials.
type RegisteredWorkloadHost struct {
	AgentID    uuid.UUID        `json:"agent_id" format:"uuid"`
	TemplateID uuid.UUID        `json:"template_id" format:"uuid"`
	Info       WorkloadHostInfo `json:"info"`
}

// CreateWorkload admits a run onto explicitly registered prepared capacity.
func (c *Client) CreateWorkload(ctx context.Context, org uuid.UUID, req CreateWorkloadRequest) (Workload, error) {
	var result Workload
	err := c.workloadRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workloads", org), req, &result)
	return result, err
}

// WorkloadOutput fetches retained output without relaunching the command.
func (c *Client) WorkloadOutput(ctx context.Context, org, id uuid.UUID, offset int64) (WorkloadOutput, error) {
	var result WorkloadOutput
	err := c.workloadRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/workloads/%s?offset=%d", org, id, offset), nil, &result)
	return result, err
}

// CancelWorkload requests cancellation independently of the observation stream.
func (c *Client) CancelWorkload(ctx context.Context, org, id uuid.UUID) error {
	return c.workloadRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/organizations/%s/workloads/%s", org, id), nil, nil)
}

// RegisterWorkloadHost enrolls a host as an organization administrator.
func (c *Client) RegisterWorkloadHost(ctx context.Context, org uuid.UUID, req RegisterWorkloadHostRequest) (RegisteredWorkloadHost, error) {
	var result RegisteredWorkloadHost
	err := c.workloadRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workload-hosts", org), req, &result)
	return result, err
}

// WorkloadHostStats reads durable per-host counts as an organization administrator.
func (c *Client) WorkloadHostStats(ctx context.Context, org, agent uuid.UUID) (WorkloadHostStats, error) {
	var result WorkloadHostStats
	err := c.workloadRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/workload-hosts/%s/stats", org, agent), nil, &result)
	return result, err
}

// WorkloadHostHistory reads a page of private command history as an organization
// administrator. The limit must be between 1 and 10.
func (c *Client) WorkloadHostHistory(ctx context.Context, org, agent uuid.UUID, offset, limit int) (WorkloadHostHistory, error) {
	var result WorkloadHostHistory
	err := c.workloadRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/workload-hosts/%s/history?offset=%d&limit=%d", org, agent, offset, limit), nil, &result)
	return result, err
}

// DrainWorkloadHost stops new placements without canceling active commands.
func (c *Client) DrainWorkloadHost(ctx context.Context, org, agent uuid.UUID) error {
	return c.workloadRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/organizations/%s/workload-hosts/%s", org, agent), nil, nil)
}

func (c *Client) workloadRequest(ctx context.Context, method, path string, body, result any) error {
	ctx = context.WithValue(ctx, privateRequestBodyKey{}, true)
	res, err := c.Request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		err := ReadBodyAsError(res)
		if res.StatusCode == http.StatusTooManyRequests {
			return &workloadRateLimitError{err: err, retryAfter: workloadRetryAfter(res.Header)}
		}
		return err
	}
	if result != nil {
		return ReadBodyAsJSON(res, result)
	}
	return nil
}

type workloadRateLimitError struct {
	err        error
	retryAfter time.Duration
}

func (e *workloadRateLimitError) Error() string { return e.err.Error() }

func (e *workloadRateLimitError) Unwrap() error { return e.err }

// RetryAfter reports the delay before a caller should retry the request.
func (e *workloadRateLimitError) RetryAfter() time.Duration { return e.retryAfter }

func workloadRetryAfter(header http.Header) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds > 0 && seconds <= uint64(math.MaxInt64/int64(time.Second)) {
			return time.Duration(seconds) * time.Second
		}
	} else if retryAt, err := http.ParseTime(value); err == nil {
		now, err := http.ParseTime(header.Get("Date"))
		if err != nil {
			now = time.Now()
		}
		if delay := retryAt.Sub(now); delay > 0 {
			return delay
		}
	}
	// Missing or unusable guidance falls back to the current rate window.
	return time.Minute
}
