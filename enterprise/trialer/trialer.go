package trialer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

// Many structures here mimic those from https://github.com/coder/license/blob/main/server/server.go

// Creates a handler that can issue trial licenses
func New(db database.Store, url string, keys map[string]ed25519.PublicKey) func(ctx context.Context, body codersdk.LicensorTrialRequest) error {
	return func(ctx context.Context, body codersdk.LicensorTrialRequest) error {
		deploymentID, err := db.GetDeploymentID(ctx)
		if err != nil {
			return xerrors.Errorf("get deployment id: %w", err)
		}
		body.DeploymentID = deploymentID
		data, err := json.Marshal(body)
		if err != nil {
			return xerrors.Errorf("marshal: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return xerrors.Errorf("create license request: %w", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return xerrors.Errorf("perform license request: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode > 300 {
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return xerrors.Errorf("read license response: %w", err)
			}
			// This is the format of the error response from the license server.
			var msg struct {
				Error string `json:"error"`
			}
			err = json.Unmarshal(body, &msg)
			if err != nil {
				return xerrors.Errorf("unmarshal error: %w", err)
			}
			return xerrors.New(msg.Error)
		}
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			return xerrors.Errorf("read license: %w", err)
		}
		rawClaims, err := license.ParseRaw(string(raw), keys)
		if err != nil {
			return xerrors.Errorf("parse license: %w", err)
		}
		exp, ok := rawClaims["exp"].(float64)
		if !ok {
			return xerrors.New("invalid license missing exp claim")
		}
		expTime := time.Unix(int64(exp), 0)

		claims, err := license.ParseClaims(string(raw), keys)
		if err != nil {
			return xerrors.Errorf("parse claims: %w", err)
		}
		id, err := uuid.Parse(claims.ID)
		if err != nil {
			return xerrors.Errorf("parse uuid: %w", err)
		}
		_, err = db.InsertLicense(ctx, database.InsertLicenseParams{
			UploadedAt: dbtime.Now(),
			JWT:        string(raw),
			Exp:        expTime,
			UUID:       id,
		})
		if err != nil {
			return xerrors.Errorf("insert license: %w", err)
		}
		return nil
	}
}

// LicenseRequestURL is the Coder licensor endpoint that issues trial licenses.
const LicenseRequestURL = "https://v2-licensor.coder.com/trial"

const (
	// licenseRequestSource tells the licensor where a trial request originated.
	licenseRequestSource = "Product"
	// licenseRequestTimeout bounds a single request to the licensor.
	licenseRequestTimeout = 30 * time.Second
)

type LicensorError struct {
	Message string
}

func (e *LicensorError) Error() string {
	return e.Message
}

// licenseRequest is the licensor's wire format for a trial request.
type licenseRequest struct {
	DeploymentID string `json:"deployment_id"`
	Email        string `json:"email"`
	Source       string `json:"source"`

	// Personal details.
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	PhoneNumber string `json:"phone_number"`
	JobTitle    string `json:"job_title"`
	CompanyName string `json:"company_name"`
	Country     string `json:"country"`
	Developers  string `json:"developers"`
}

// NewLicenseRequester creates a handler that requests a trial license and
// returns the raw signed JWT. Unlike New, it does not touch the database, so
// the caller installs the license and therefore keeps auditing and entitlement
// refresh on the same path as a manually uploaded license.
func NewLicenseRequester(url string) func(ctx context.Context, deploymentID string, req codersdk.CreateTrialLicenseRequest) (string, error) {
	client := &http.Client{Timeout: licenseRequestTimeout}
	return func(ctx context.Context, deploymentID string, req codersdk.CreateTrialLicenseRequest) (string, error) {
		data, err := json.Marshal(licenseRequest{
			DeploymentID: deploymentID,
			Email:        req.Email,
			Source:       licenseRequestSource,
			FirstName:    req.FirstName,
			LastName:     req.LastName,
			PhoneNumber:  req.PhoneNumber,
			JobTitle:     req.JobTitle,
			CompanyName:  req.CompanyName,
			Country:      req.Country,
			Developers:   req.Developers,
		})
		if err != nil {
			return "", xerrors.Errorf("marshal: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return "", xerrors.Errorf("create license request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		res, err := client.Do(httpReq)
		if err != nil {
			return "", xerrors.Errorf("perform license request: %w", err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return "", xerrors.Errorf("read license response: %w", err)
		}
		if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
			return "", &LicensorError{Message: licenseRequestErrorMessage(body, res.Status)}
		}
		raw := strings.TrimSpace(string(body))
		if raw == "" {
			return "", xerrors.New("licensor returned an empty license")
		}
		return raw, nil
	}
}

// licenseRequestErrorMessage reads the licensor's error message, falling back to
// the HTTP status when the body is not in the expected shape.
func licenseRequestErrorMessage(body []byte, status string) string {
	var msg struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &msg); err == nil && msg.Error != "" {
		return msg.Error
	}
	return status
}
