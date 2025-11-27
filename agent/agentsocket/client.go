package agentsocket

import (
	"github.com/coder/coder/v2/agent/unit"
)

// Option represents a configuration option for NewClient.
type Option func(*options)

type options struct {
	path string
}

// WithPath sets the socket path. If not provided or empty, the client will
// auto-discover the default socket path.
func WithPath(path string) Option {
	return func(opts *options) {
		if path == "" {
			return
		}
		opts.path = path
	}
}

// SyncStatusResponse contains the status information for a unit.
type SyncStatusResponse struct {
	UnitName     unit.ID          `table:"unit,default_sort" json:"unit_name"`
	Status       unit.Status      `table:"status" json:"status"`
	IsReady      bool             `table:"ready" json:"is_ready"`
	Dependencies []DependencyInfo `table:"dependencies" json:"dependencies"`
}

// DependencyInfo contains information about a unit dependency.
type DependencyInfo struct {
	DependsOn      unit.ID     `table:"depends on,default_sort" json:"depends_on"`
	RequiredStatus unit.Status `table:"required status" json:"required_status"`
	CurrentStatus  unit.Status `table:"current status" json:"current_status"`
	IsSatisfied    bool        `table:"satisfied" json:"is_satisfied"`
}
