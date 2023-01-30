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
	Claims map[string]interface{} `json:"claims"`
}

// Features provides the feature claims in license.
func (l *License) Features() (map[FeatureName]int64, error) {
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
