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

func (c *Client) UploadFile(ctx context.Context, contentType string, content []byte) (coderd.UploadFileResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/files", content, func(r *http.Request) {
		r.Header.Set("Content-Type", contentType)
	})
	if err != nil {
		return coderd.UploadFileResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return coderd.UploadFileResponse{}, readBodyAsError(res)
	}
	var resp coderd.UploadFileResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
