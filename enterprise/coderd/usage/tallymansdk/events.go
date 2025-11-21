package tallymansdk

import (
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

// PublishUsageEvents publishes usage events to the Tallyman API.
func (c *Client) PublishUsageEvents(ctx context.Context, req usagetypes.TallymanV1IngestRequest) (usagetypes.TallymanV1IngestResponse, error) {
	resp, err := c.Request(ctx, http.MethodPost, "/api/v1/events/ingest", req)
	if err != nil {
		return usagetypes.TallymanV1IngestResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return usagetypes.TallymanV1IngestResponse{}, readErrorResponse(resp)
	}

	var respBody usagetypes.TallymanV1IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return usagetypes.TallymanV1IngestResponse{}, xerrors.Errorf("decode response body: %w", err)
	}

	return respBody, nil
}
