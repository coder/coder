package codersdk
import (
	"errors"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"github.com/google/uuid"
)
const (
	LicenseExpiryClaim                = "license_expires"
	LicenseTelemetryRequiredErrorText = "License requires telemetry but telemetry is disabled"
)
type AddLicenseRequest struct {
	License string `json:"license" validate:"required"`
}
type License struct {
	ID         int32     `json:"id"`
	UUID       uuid.UUID `json:"uuid" format:"uuid"`
	UploadedAt time.Time `json:"uploaded_at" format:"date-time"`
	// Claims are the JWT claims asserted by the license.  Here we use
	// a generic string map to ensure that all data from the server is
	// parsed verbatim, not just the fields this version of Coder
	// understands.
	Claims map[string]interface{} `json:"claims" table:"claims"`
}
// ExpiresAt returns the expiration time of the license.
// If the claim is missing or has an unexpected type, an error is returned.
func (l *License) ExpiresAt() (time.Time, error) {
	expClaim, ok := l.Claims[LicenseExpiryClaim]
	if !ok {
		return time.Time{}, errors.New("license_expires claim is missing")
	}
	// This claim should be a unix timestamp.
	// Everything is already an interface{}, so we need to do some type
	// assertions to figure out what we're dealing with.
	if unix, ok := expClaim.(json.Number); ok {
		i64, err := unix.Int64()
		if err != nil {
			return time.Time{}, fmt.Errorf("license_expires claim is not a valid unix timestamp: %w", err)
		}
		return time.Unix(i64, 0), nil
	}
	return time.Time{}, fmt.Errorf("license_expires claim has unexpected type %T", expClaim)
}
func (l *License) Trial() bool {
	if trail, ok := l.Claims["trail"].(bool); ok {
		return trail
	}
	return false
}
func (l *License) AllFeaturesClaim() bool {
	if all, ok := l.Claims["all_features"].(bool); ok {
		return all
	}
	return false
}
// FeaturesClaims provides the feature claims in license.
// This only returns the explicit claims. If checking for actual usage,
// also check `AllFeaturesClaim`.
func (l *License) FeaturesClaims() (map[FeatureName]int64, error) {
	strMap, ok := l.Claims["features"].(map[string]interface{})
	if !ok {
		return nil, errors.New("features key is unexpected type")
	}
	fMap := make(map[FeatureName]int64)
	for k, v := range strMap {
		jn, ok := v.(json.Number)
		if !ok {
			return nil, fmt.Errorf("feature %q has unexpected type", k)
		}
		n, err := jn.Int64()
		if err != nil {
			return nil, err
		}
		fMap[FeatureName(k)] = n
	}
	return fMap, nil
}
func (c *Client) AddLicense(ctx context.Context, r AddLicenseRequest) (License, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/licenses", r)
	if err != nil {
		return License{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return License{}, ReadBodyAsError(res)
	}
	var l License
	d := json.NewDecoder(res.Body)
	d.UseNumber()
	return l, d.Decode(&l)
}
func (c *Client) Licenses(ctx context.Context) ([]License, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/licenses", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var licenses []License
	d := json.NewDecoder(res.Body)
	d.UseNumber()
	return licenses, d.Decode(&licenses)
}
func (c *Client) DeleteLicense(ctx context.Context, id int32) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/licenses/%d", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}
