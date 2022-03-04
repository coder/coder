package codersdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/coder/coderd"
)

const (
	ContentTypeTar = "application/x-tar"
)

// Upload uploads an arbitrary file with the content type provided.
// This is used to upload a source-code archive.
func (c *Client) Upload(ctx context.Context, contentType string, content []byte) (coderd.UploadResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/upload", content, func(r *http.Request) {
		r.Header.Set("Content-Type", contentType)
	})
	if err != nil {
		return coderd.UploadResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return coderd.UploadResponse{}, readBodyAsError(res)
	}
	var resp coderd.UploadResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
