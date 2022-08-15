package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func entitlements(rw http.ResponseWriter, _ *http.Request) {
	features := make(map[string]codersdk.Feature)
	for _, f := range codersdk.AllFeatures {
		features[f] = codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     false,
		}
	}
	httpapi.Write(rw, http.StatusOK, codersdk.Entitlements{
		Features:   features,
		Warnings:   nil,
		HasLicense: false,
	})
}
