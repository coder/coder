package entitlements

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/coder/v2/codersdk"
)

type Set struct {
	entitlementsMu sync.RWMutex
	entitlements   codersdk.Entitlements
}

func New() *Set {
	return &Set{
		// Some defaults for an unlicensed instance.
		// These will be updated when coderd is initialized.
		entitlements: codersdk.Entitlements{
			Features:         map[codersdk.FeatureName]codersdk.Feature{},
			Warnings:         nil,
			Errors:           nil,
			HasLicense:       false,
			Trial:            false,
			RequireTelemetry: false,
			RefreshedAt:      time.Time{},
		},
	}
}

// AllowRefresh returns whether the entitlements are allowed to be refreshed.
// If it returns false, that means it was recently refreshed and the caller should
// wait the returned duration before trying again.
func (l *Set) AllowRefresh(now time.Time) (bool, time.Duration) {
	l.entitlementsMu.RLock()
	defer l.entitlementsMu.RUnlock()

	diff := now.Sub(l.entitlements.RefreshedAt)
	if diff < time.Minute {
		return false, time.Minute - diff
	}

	return true, 0
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

func (l *Set) Replace(entitlements codersdk.Entitlements) {
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()

	l.entitlements = entitlements
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
