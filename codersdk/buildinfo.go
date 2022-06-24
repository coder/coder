package codersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// BuildInfoResponse contains build information for this instance of Coder.
type BuildInfoResponse struct {
	// ExternalURL is a URL referencing the current Coder version.  For production
	// builds, this will link directly to a release.  For development builds, this
	// will link to a commit.
	ExternalURL string `json:"external_url"`
	// Version returns the semantic version of the build.
	Version string `json:"version"`
}

// TrimmedVersion trims build information from the version.
// E.g. 'v0.7.4-devel+11573034' -> 'v0.7.4'.
func (b BuildInfoResponse) TrimmedVersion() string {
	// Linter doesn't like strings.Index...
	idx := bytes.Index([]byte(b.Version), []byte("-devel"))
	if idx < 0 {
		return string(b.Version)
	}

	return b.Version[:idx]
}

// BuildInfo returns build information for this instance of Coder.
func (c *Client) BuildInfo(ctx context.Context) (BuildInfoResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/buildinfo", nil)
	if err != nil {
		return BuildInfoResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return BuildInfoResponse{}, readBodyAsError(res)
	}

	var buildInfo BuildInfoResponse
	return buildInfo, json.NewDecoder(res.Body).Decode(&buildInfo)
}
