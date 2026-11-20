package codersdk

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// PreviewSignInRequest asks for a URL that signs the browser in to one
// preview of a workspace the caller owns.
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
	RegionID uuid.UUID `json:"region_id,omitempty" format:"uuid"`
}

// PreviewSignInResponse is how the dashboard frames a preview.
type PreviewSignInResponse struct {
	// URL is the preview URL with a one-minute sign-in token for its host.
	URL string `json:"url"`
	// Origin is the preview's origin, the only one its wrapper may frame.
	Origin string `json:"origin"`
	// Sandbox is the iframe sandbox to frame the preview with.
	Sandbox string `json:"sandbox"`
}

// PreviewSignIn returns a URL that signs the browser in to a preview.
func (c *ExperimentalClient) PreviewSignIn(ctx context.Context, req PreviewSignInRequest) (PreviewSignInResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/previews/sign-in", req)
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
