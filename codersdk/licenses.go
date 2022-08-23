package codersdk

import (
	"context"
	"encoding/json"
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
