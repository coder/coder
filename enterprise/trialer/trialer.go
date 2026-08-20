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

type Trialer struct {
	db     database.Store
	url    string
	keys   map[string]ed25519.PublicKey
	client *http.Client
}

// New creates a handler that can issue trial licenses
func New(db database.Store, url string, keys map[string]ed25519.PublicKey) *Trialer {
	return &Trialer{
		db:     db,
		url:    url,
		keys:   keys,
		client: &http.Client{Timeout: licenseRequestTimeout},
	}
}

// Request returns the raw signed license without storing it.
func (t *Trialer) Request(ctx context.Context, body codersdk.LicensorTrialRequest) (string, error) {
	body.Source = licenseRequestSource
	data, err := json.Marshal(body)
	if err != nil {
		return "", xerrors.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(data))
	if err != nil {
		return "", xerrors.Errorf("create license request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := t.client.Do(req)
	if err != nil {
		return "", xerrors.Errorf("perform license request: %w", err)
	}
	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return "", xerrors.Errorf("read license response: %w", err)
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return "", &LicensorError{Message: licenseRequestErrorMessage(resBody, res.Status)}
	}
	raw := strings.TrimSpace(string(resBody))
	if raw == "" {
		return "", xerrors.New("licensor returned an empty license")
	}
	return raw, nil
}

// Generate requests a trial license and stores it, skipping the auditing and
// entitlement refresh that installing an uploaded license performs.
func (t *Trialer) Generate(ctx context.Context, body codersdk.LicensorTrialRequest) error {
	deploymentID, err := t.db.GetDeploymentID(ctx)
	if err != nil {
		return xerrors.Errorf("get deployment id: %w", err)
	}
	body.DeploymentID = deploymentID
	raw, err := t.Request(ctx, body)
	if err != nil {
		return err
	}
	rawClaims, err := license.ParseRaw(raw, t.keys)
	if err != nil {
		return xerrors.Errorf("parse license: %w", err)
	}
	exp, ok := rawClaims["exp"].(float64)
	if !ok {
		return xerrors.New("invalid license missing exp claim")
	}
	expTime := time.Unix(int64(exp), 0)

	claims, err := license.ParseClaims(raw, t.keys)
	if err != nil {
		return xerrors.Errorf("parse claims: %w", err)
	}
	id, err := uuid.Parse(claims.ID)
	if err != nil {
		return xerrors.Errorf("parse uuid: %w", err)
	}
	_, err = t.db.InsertLicense(ctx, database.InsertLicenseParams{
		UploadedAt: dbtime.Now(),
		JWT:        raw,
		Exp:        expTime,
		UUID:       id,
	})
	if err != nil {
		return xerrors.Errorf("insert license: %w", err)
	}
	return nil
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
