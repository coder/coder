package entitlements

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/coder/coder/v2/codersdk"
)

type Set struct {
	entitlementsMu sync.RWMutex
	entitlements   codersdk.Entitlements
}

func New(entitlements codersdk.Entitlements) *Set {
	return &Set{
		entitlements: entitlements,
	}
}

func (l *Set) Replace(entitlements codersdk.Entitlements) {
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()

	l.entitlements = entitlements
}

func (l *Set) Feature(name codersdk.FeatureName) (codersdk.Feature, bool) {
	l.entitlementsMu.RLock()
	defer l.entitlementsMu.RUnlock()

	f, ok := l.entitlements.Features[name]
	return f, ok
}

func (l *Set) Enabled(feature codersdk.FeatureName) bool {
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()

	f, ok := l.entitlements.Features[feature]
	if !ok {
		return false
	}
	return f.Enabled
}

// AsJSON is used to return this to the api without exposing the entitlements for
// mutation.
func (l *Set) AsJSON() json.RawMessage {
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()

	b, _ := json.Marshal(l.entitlements)
	return b
}

func (l *Set) Update(do func(entitlements *codersdk.Entitlements)) {
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()

	do(&l.entitlements)
}

func (l *Set) FeatureChanged(featureName codersdk.FeatureName, newFeature codersdk.Feature) (initial, changed, enabled bool) {
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()

	oldFeature := l.entitlements.Features[featureName]
	if oldFeature.Enabled != newFeature.Enabled {
		return false, true, newFeature.Enabled
	}
	return false, false, newFeature.Enabled
}

func (l *Set) WriteEntitlementWarningHeaders(header http.Header) {
	l.entitlementsMu.RLock()
	defer l.entitlementsMu.RUnlock()

	for _, warning := range l.entitlements.Warnings {
		header.Add(codersdk.EntitlementsWarningHeader, warning)
	}
}
