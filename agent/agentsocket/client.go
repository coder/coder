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
	UnitName     string           `table:"Unit" json:"unit_name"`
	Status       string           `table:"Status" json:"status"`
	IsReady      bool             `table:"Ready" json:"is_ready"`
	Dependencies []DependencyInfo `table:"Dependencies,recursive_inline" json:"dependencies"`
}

// DependencyInfo contains information about a unit dependency.
type DependencyInfo struct {
	DependsOn      string `table:"Depends On,default_sort" json:"depends_on"`
	RequiredStatus string `table:"Required" json:"required_status"`
	CurrentStatus  string `table:"Current" json:"current_status"`
	IsSatisfied    bool   `table:"Satisfied" json:"is_satisfied"`
}
