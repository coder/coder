package codersdk

type Region struct {
	ID string `json:"id" format:"uuid" table:"id"`
}
type WorkspaceProxy struct {
	// Extends Region with extra information
	Region   `table:"region,recursive_inline"`
	DerpOnly bool `json:"derp_only" table:"derp_only"`
}

type RegionTypes interface {
	Region | WorkspaceProxy
}

type RegionsResponse[R RegionTypes] struct {
	Regions []R `json:"regions"`
}
