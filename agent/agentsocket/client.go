package agentsocket

// Option represents a configuration option for NewClient.
type Option func(*options)

type options struct {
	path string
}

// WithPath sets the socket path. If not provided or empty, the client will
// auto-discover the default socket path.
func WithPath(path string) Option {
	return func(opts *options) {
		opts.path = path
	}
}

// SyncStatusResponse contains the status information for a unit.
type SyncStatusResponse struct {
	UnitName     string           `table:"unit" json:"unit_name"`
	Status       string           `table:"status" json:"status"`
	IsReady      bool             `table:"ready" json:"is_ready"`
	Dependencies []DependencyInfo `table:"dependencies,recursive_inline" json:"dependencies"`
}

// DependencyInfo contains information about a unit dependency.
type DependencyInfo struct {
	DependsOn      string `table:"depends on,default_sort" json:"depends_on"`
	RequiredStatus string `table:"required status" json:"required_status"`
	CurrentStatus  string `table:"current status" json:"current_status"`
	IsSatisfied    bool   `table:"satisfied" json:"is_satisfied"`
}
