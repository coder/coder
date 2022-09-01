package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type AddLicenseRequest struct {
	License string `json:"license" validate:"required"`
}

type License struct {
	ID         int32     `json:"id"`
	UploadedAt time.Time `json:"uploaded_at"`
	// Claims are the JWT claims asserted by the license.  Here we use
	// a generic string map to ensure that all data from the server is
	// parsed verbatim, not just the fields this version of Coder
	// understands.
	Claims map[string]interface{} `json:"claims"`
}

func (c *Client) AddLicense(ctx context.Context, r AddLicenseRequest) (License, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/licenses", r)
	if err != nil {
		return License{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return License{}, readBodyAsError(res)
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
		return nil, readBodyAsError(res)
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
		return readBodyAsError(res)
	}
	return nil
}
