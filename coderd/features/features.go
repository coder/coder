package features

import "net/http"

// Service is the interface for interacting with enterprise features.
type Service interface {
	EntitlementsAPI(w http.ResponseWriter, r *http.Request)

	// Get returns the implementations for feature interfaces. Parameter `s` must be a pointer to a
	// struct type containing feature interfaces as fields.  The FeatureService sets all fields to
	// the correct implementations depending on whether the features are turned on.
	Get(s any) error
}
