package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
)

const (
	ContentTypeTar = "application/x-tar"
	ContentTypeZip = "application/zip"

	FormatZip = "zip"
)

// UploadResponse contains the hash to reference the uploaded file.
type UploadResponse struct {
	ID uuid.UUID `json:"hash" format:"uuid"`
}

// Upload uploads an arbitrary file with the content type provided.
// This is used to upload a source-code archive.
func (c *Client) Upload(ctx context.Context, contentType string, rd io.Reader) (UploadResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/files", rd, func(r *http.Request) {
		r.Header.Set("Content-Type", contentType)
	})
	if err != nil {
		return UploadResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return UploadResponse{}, ReadBodyAsError(res)
	}
	var resp UploadResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// Download fetches a file by uploaded hash.
func (c *Client) Download(ctx context.Context, id uuid.UUID) ([]byte, string, error) {
	return c.DownloadWithFormat(ctx, id, "")
}

// Download fetches a file by uploaded hash, but it forces format conversion.
func (c *Client) DownloadWithFormat(ctx context.Context, id uuid.UUID, format string) ([]byte, string, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/files/%s?format=%s", id.String(), format), nil)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", ReadBodyAsError(res)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return data, res.Header.Get("Content-Type"), nil
}
