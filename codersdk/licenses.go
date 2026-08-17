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
)

type AddLicenseRequest struct {
	License string `json:"license" validate:"required"`
}

// CreateTrialLicenseRequest defines the input payload for requesting a trial license.
type CreateTrialLicenseRequest struct {
	Email       string `json:"email"        validate:"required,email,max=254"      example:"jane.doe@example.com"  format:"email" maxLength:"254"`
	FirstName   string `json:"first_name"   validate:"required,min=1,max=60"       example:"Jane"                   minLength:"1" maxLength:"60"`
	LastName    string `json:"last_name"    validate:"required,min=1,max=60"       example:"Doe"                    minLength:"1" maxLength:"60"`
	PhoneNumber string `json:"phone_number" validate:"required"                    example:"+14155552671"           pattern:"^\\+?[\\d\\s\\-\\.\\(\\)]{7,20}$"`
	JobTitle    string `json:"job_title"    validate:"required,min=2,max=100"      example:"Engineering Manager"    minLength:"2" maxLength:"100"`
	CompanyName string `json:"company_name" validate:"required,min=2,max=100"      example:"Acme Corp"              minLength:"2" maxLength:"100"`
	Country     string `json:"country"      validate:"required"                    example:"United States"`
	Developers  string `json:"developers"   validate:"required"`
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

// CreateTrialLicense requests a trial license from the Coder licensor and install it.
func (c *Client) CreateTrialLicense(ctx context.Context, r CreateTrialLicenseRequest) (License, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/licenses/trial", r)
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
