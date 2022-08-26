package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// FeaturesService is the interface for interacting with enterprise features.
type FeaturesService interface {
	EntitlementsAPI(w http.ResponseWriter, r *http.Request)

	// TODO
	// Get returns the implementations for feature interfaces. Parameter `s `must be a pointer to a
	// struct type containing feature interfaces as fields.  The FeatureService sets all fields to
	// the correct implementations depending on whether the features are turned on.
	// Get(s any) error
}

type featuresService struct{}

func (featuresService) EntitlementsAPI(rw http.ResponseWriter, _ *http.Request) {
	features := make(map[string]codersdk.Feature)
	for _, f := range codersdk.FeatureNames {
		features[f] = codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     false,
		}
	}
	httpapi.Write(rw, http.StatusOK, codersdk.Entitlements{
		Features:   features,
		Warnings:   []string{},
		HasLicense: false,
	})
}
