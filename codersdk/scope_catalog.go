package codersdk

// ScopeCatalog describes the public API key scope catalog exposed by coderd.
type ScopeCatalog struct {
	Specials   []APIKeyScope           `json:"specials"`
	LowLevel   []ScopeCatalogLowLevel  `json:"low_level"`
	Composites []ScopeCatalogComposite `json:"composites"`
}

// ScopeCatalogLowLevel contains metadata about a low-level scope that maps
// directly to a single resource/action tuple.
type ScopeCatalogLowLevel struct {
	Name     APIKeyScope  `json:"name"`
	Resource RBACResource `json:"resource"`
	Action   string       `json:"action"`
}

// ScopeCatalogComposite contains metadata about composite coder:* scopes
// and the low-level scopes they expand to.
type ScopeCatalogComposite struct {
	Name      APIKeyScope   `json:"name"`
	ExpandsTo []APIKeyScope `json:"expands_to"`
}
