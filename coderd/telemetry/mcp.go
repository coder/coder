package telemetry

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/coder/quartz"
)

// Transports over which MCP tool calls are served.
const (
	// MCPTransportRemote is the coderd HTTP MCP endpoint.
	MCPTransportRemote = "remote"
	// MCPTransportStdio is the local `coder exp mcp server` command.
	MCPTransportStdio = "stdio"
)

// maxMCPToolUsageKeys caps the number of distinct aggregation keys held
// between snapshots. Callers normalize tool and client names to small
// fixed sets, so this is only a guard against a caller that does not.
const maxMCPToolUsageKeys = 1024

// MCPToolUsageKey identifies one aggregate of MCP tool calls. Tool and
// Client must already be normalized to bounded sets of values.
type MCPToolUsageKey struct {
	Transport string
	Tool      string
	Client    string
}

// MCPToolUsageCounts holds call counts for one MCP tool usage key. Every
// error is also counted in Calls.
type MCPToolUsageCounts struct {
	Calls            int64
	ToolErrors       int64
	ValidationErrors int64
	AuthzErrors      int64
}

// MCPToolUsageSummary reports aggregated MCP tool calls for one tool,
// transport, and normalized client over a period. It contains no tool
// arguments, results, or user identifiers.
//
// Every coderd replica aggregates its own calls in memory, so summaries
// from several replicas with overlapping periods must be summed, not
// deduplicated. ID is unique per summary.
type MCPToolUsageSummary struct {
	ID               uuid.UUID `json:"id"`
	PeriodStart      time.Time `json:"period_start"`
	PeriodEnd        time.Time `json:"period_end"`
	Transport        string    `json:"transport"`
	Tool             string    `json:"tool"`
	Client           string    `json:"client"`
	Calls            int64     `json:"calls"`
	ToolErrors       int64     `json:"tool_errors"`
	ValidationErrors int64     `json:"validation_errors"`
	AuthzErrors      int64     `json:"authz_errors"`
}

// MCPToolUsage aggregates MCP tool call counts in memory between
// telemetry snapshots. It is safe for concurrent use.
type MCPToolUsage struct {
	clock quartz.Clock

	mu     sync.Mutex
	since  time.Time
	counts map[MCPToolUsageKey]MCPToolUsageCounts
}

// NewMCPToolUsage returns an empty aggregator. A nil clock uses the real
// clock.
func NewMCPToolUsage(clock quartz.Clock) *MCPToolUsage {
	if clock == nil {
		clock = quartz.NewReal()
	}
	return &MCPToolUsage{
		clock:  clock,
		since:  clock.Now().UTC(),
		counts: make(map[MCPToolUsageKey]MCPToolUsageCounts),
	}
}

// Add adds counts to the aggregate for key. Counts without calls, with
// negative values, or with more errors than calls are ignored.
func (u *MCPToolUsage) Add(key MCPToolUsageKey, counts MCPToolUsageCounts) {
	if counts.Calls <= 0 || counts.ToolErrors < 0 || counts.ValidationErrors < 0 || counts.AuthzErrors < 0 ||
		counts.ToolErrors+counts.ValidationErrors+counts.AuthzErrors > counts.Calls {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	current, ok := u.counts[key]
	if !ok && len(u.counts) >= maxMCPToolUsageKeys {
		return
	}
	current.Calls += counts.Calls
	current.ToolErrors += counts.ToolErrors
	current.ValidationErrors += counts.ValidationErrors
	current.AuthzErrors += counts.AuthzErrors
	u.counts[key] = current
}

// Collect returns the aggregates accumulated since the previous Collect,
// sorted by key, and resets the aggregator.
func (u *MCPToolUsage) Collect() []MCPToolUsageSummary {
	u.mu.Lock()
	counts := u.counts
	start := u.since
	end := u.clock.Now().UTC()
	u.counts = make(map[MCPToolUsageKey]MCPToolUsageCounts)
	u.since = end
	u.mu.Unlock()

	summaries := make([]MCPToolUsageSummary, 0, len(counts))
	for key, c := range counts {
		summaries = append(summaries, MCPToolUsageSummary{
			ID:               uuid.New(),
			PeriodStart:      start,
			PeriodEnd:        end,
			Transport:        key.Transport,
			Tool:             key.Tool,
			Client:           key.Client,
			Calls:            c.Calls,
			ToolErrors:       c.ToolErrors,
			ValidationErrors: c.ValidationErrors,
			AuthzErrors:      c.AuthzErrors,
		})
	}
	slices.SortFunc(summaries, func(a, b MCPToolUsageSummary) int {
		return cmp.Or(
			cmp.Compare(a.Transport, b.Transport),
			cmp.Compare(a.Tool, b.Tool),
			cmp.Compare(a.Client, b.Client),
		)
	})
	return summaries
}
