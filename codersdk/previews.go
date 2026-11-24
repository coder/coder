package codersdk

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// PreviewSignInRequest names one preview of a workspace the caller owns, for
// the preview address and sign-in endpoints.
type PreviewSignInRequest struct {
	WorkspaceID uuid.UUID `json:"workspace_id" format:"uuid"`
	AgentName   string    `json:"agent_name"`
	// PreviewSlug is the preview's app slug, such as "preview--0123abcd" or
	// "preview--8080".
	PreviewSlug string `json:"preview_slug"`
	// URLPath is the path and query the preview opens at. Empty opens the
	// root.
	URLPath string `json:"url_path,omitempty"`
	// RegionID picks the primary or a workspace proxy. Empty picks the
	// primary.
	RegionID uuid.UUID `json:"region_id,omitzero" format:"uuid"`
}

// PreviewSignInResponse is how the dashboard frames a preview.
type PreviewSignInResponse struct {
	// URL is the preview URL. From sign-in, it also carries a sign-in token
	// for the preview's host, valid for one minute, plus up to one minute of
	// clock leeway between coderd and the server that checks it.
	URL string `json:"url"`
	// Origin is the preview's origin. Clients should allow their frame to
	// load only this origin.
	Origin string `json:"origin"`
	// Sandbox is the iframe sandbox to frame the preview with.
	Sandbox string `json:"sandbox"`
}

// PreviewAddress returns where a preview loads, without signing in.
func (c *ExperimentalClient) PreviewAddress(ctx context.Context, req PreviewSignInRequest) (PreviewSignInResponse, error) {
	return c.previewFrame(ctx, "/api/experimental/previews/address", req)
}

// PreviewSignIn returns a URL that signs the browser in to a preview.
func (c *ExperimentalClient) PreviewSignIn(ctx context.Context, req PreviewSignInRequest) (PreviewSignInResponse, error) {
	return c.previewFrame(ctx, "/api/experimental/previews/sign-in", req)
}

func (c *ExperimentalClient) previewFrame(ctx context.Context, path string, req PreviewSignInRequest) (PreviewSignInResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, path, req)
	if err != nil {
		return PreviewSignInResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return PreviewSignInResponse{}, ReadBodyAsError(res)
	}
	var resp PreviewSignInResponse
	return resp, ReadBodyAsJSON(res, &resp)
}
