package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	LicenseExpiryClaim                          = "license_expires"
	LicenseTelemetryRequiredErrorText           = "License requires telemetry but telemetry is disabled"
	LicenseManagedAgentLimitExceededWarningText = "You have built more workspaces with managed agents than your license allows."
	LicenseAIGovernance90PercentWarningText     = "You have used %d%% of your AI Governance add-on seats."
	LicenseAIGovernanceOverLimitWarningText     = "Your organization is using %d of %d AI Governance add-on seats (%d over the limit)."
	// The dashboard's LicenseBanner matches this text's pre-placeholder
	// prefix to render it muted and without a sales link, so the license
	// warning texts must stay pairwise distinct before their first
	// placeholder. See TestLicenseAgentRuntimeHoursWarningTexts.
	LicenseAgentRuntimeHoursSoftLimitWarningText         = "Your deployment is approaching its Coder Agent runtime hours allocation: %d of the %d hours included in the current license term are used, at or above the advisory soft limit of %d hours."
	LicenseAgentRuntimeHoursAllocationReachedWarningText = "Your deployment has used %d of the %d Coder Agent runtime hours included in the current license term."
	LicenseAgentRuntimeUsageUnavailableErrorText         = "Unable to determine Coder Agent runtime usage. Reported runtime hours are unavailable until the next successful refresh; workspaces are unaffected. Check the coderd logs for details."
	LicenseAgentRuntimeHoursClaimsIgnoredWarningText     = "A license contains unusable Coder Agent runtime hour claims, which were ignored. The rest of that license is unaffected. Check the coderd logs for the affected license and claims, and contact support to have the license re-issued."
	// LicenseUsagePublishingFailingWarningText is appended to entitlements
	// warnings after a sustained usage publishing failure.
	LicenseUsagePublishingFailingWarningText = "Coder has been unable to publish usage data to Coder's servers. Please check the deployment's connectivity and contact support if the issue persists."
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
		return time.Time{}, xerrors.New("license_expires claim is missing")
	}

	// This claim should be a unix timestamp.
	// Everything is already an interface{}, so we need to do some type
	// assertions to figure out what we're dealing with.
	if unix, ok := expClaim.(json.Number); ok {
		i64, err := unix.Int64()
		if err != nil {
			return time.Time{}, xerrors.Errorf("license_expires claim is not a valid unix timestamp: %w", err)
		}
		return time.Unix(i64, 0), nil
	}

	return time.Time{}, xerrors.Errorf("license_expires claim has unexpected type %T", expClaim)
}

func (l *License) Trial() bool {
	if trial, ok := l.Claims["trial"].(bool); ok {
		return trial
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
		return nil, xerrors.New("features key is unexpected type")
	}
	fMap := make(map[FeatureName]int64)
	for k, v := range strMap {
		jn, ok := v.(json.Number)
		if !ok {
			return nil, xerrors.Errorf("feature %q has unexpected type", k)
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
	return l, ReadBodyAsJSONUseNumber(res, &l)
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
	return licenses, ReadBodyAsJSONUseNumber(res, &licenses)
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
