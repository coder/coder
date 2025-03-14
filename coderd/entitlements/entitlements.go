package entitlements

import (
	"errors"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/coder/coder/v2/codersdk"
)

type Set struct {
	entitlementsMu sync.RWMutex
	entitlements   codersdk.Entitlements

	// right2Update works like a semaphore. Reading from the chan gives the right to update the set,
	// and you send on the chan when you are done. We only allow one simultaneous update, so this
	// serve to serialize them.  You MUST NOT attempt to read from this channel while holding the
	// entitlementsMu lock. It is permissible to acquire the entitlementsMu lock while holding the
	// right2Update token.
	right2Update chan struct{}
}
func New() *Set {
	s := &Set{
		// Some defaults for an unlicensed instance.
		// These will be updated when coderd is initialized.

		entitlements: codersdk.Entitlements{
			Features:         map[codersdk.FeatureName]codersdk.Feature{},
			Warnings:         []string{},
			Errors:           []string{},
			HasLicense:       false,
			Trial:            false,
			RequireTelemetry: false,
			RefreshedAt:      time.Time{},
		},
		right2Update: make(chan struct{}, 1),
	}
	// Ensure all features are present in the entitlements. Our frontend
	// expects this.
	for _, featureName := range codersdk.FeatureNames {
		s.entitlements.AddFeature(featureName, codersdk.Feature{
			Entitlement: codersdk.EntitlementNotEntitled,
			Enabled:     false,
		})
	}
	s.right2Update <- struct{}{} // one token, serialized updates
	return s
}
// ErrLicenseRequiresTelemetry is an error returned by a fetch passed to Update to indicate that the
// fetched license cannot be used because it requires telemetry.
var ErrLicenseRequiresTelemetry = errors.New(codersdk.LicenseTelemetryRequiredErrorText)
func (l *Set) Update(ctx context.Context, fetch func(context.Context) (codersdk.Entitlements, error)) error {
	select {

	case <-ctx.Done():
		return ctx.Err()
	case <-l.right2Update:
		defer func() {

			l.right2Update <- struct{}{}
		}()
	}
	ents, err := fetch(ctx)
	if errors.Is(err, ErrLicenseRequiresTelemetry) {
		// We can't fail because then the user couldn't remove the offending
		// license w/o a restart.
		//
		// We don't simply append to entitlement.Errors since we don't want any
		// enterprise features enabled.
		l.Modify(func(entitlements *codersdk.Entitlements) {
			entitlements.Errors = []string{err.Error()}
		})
		return nil
	}
	if err != nil {
		return err
	}
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()
	l.entitlements = ents
	return nil
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

	l.entitlementsMu.RLock()
	defer l.entitlementsMu.RUnlock()
	f, ok := l.entitlements.Features[feature]

	if !ok {
		return false
	}
	return f.Enabled

}
// AsJSON is used to return this to the api without exposing the entitlements for
// mutation.
func (l *Set) AsJSON() json.RawMessage {

	l.entitlementsMu.RLock()
	defer l.entitlementsMu.RUnlock()
	b, _ := json.Marshal(l.entitlements)
	return b

}
func (l *Set) Modify(do func(entitlements *codersdk.Entitlements)) {
	l.entitlementsMu.Lock()
	defer l.entitlementsMu.Unlock()
	do(&l.entitlements)
}
func (l *Set) FeatureChanged(featureName codersdk.FeatureName, newFeature codersdk.Feature) (initial, changed, enabled bool) {

	l.entitlementsMu.RLock()
	defer l.entitlementsMu.RUnlock()
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
func (l *Set) Errors() []string {
	l.entitlementsMu.RLock()

	defer l.entitlementsMu.RUnlock()
	return slices.Clone(l.entitlements.Errors)
}
