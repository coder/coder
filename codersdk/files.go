package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/coder/coder/coderd"
)

const (
	ContentTypeTar = "application/x-tar"
)

// Upload uploads an arbitrary file with the content type provided.
// This is used to upload a source-code archive.
func (c *Client) Upload(ctx context.Context, contentType string, content []byte) (coderd.UploadResponse, error) {
	res, err := c.request(ctx, http.MethodPost, "/api/v2/files", content, func(r *http.Request) {
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

// Download fetches a file by uploaded hash.
func (c *Client) Download(ctx context.Context, hash string) ([]byte, string, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/files/%s", hash), nil)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", readBodyAsError(res)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return data, res.Header.Get("Content-Type"), nil
}
