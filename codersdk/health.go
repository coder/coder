package codersdk

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"golang.org/x/xerrors"
)

func (c *Client) CheckLiveness(ctx context.Context) error {
	res, err := c.Request(ctx, http.MethodGet, "/healthz", nil)
	if err != nil {
		return xerrors.Errorf("liveness check failed to create request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return xerrors.Errorf("liveness check returned non-200 response: HTTP %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return xerrors.Errorf("failed to read liveness check response body: %w", err)
	}

	if !bytes.Equal(body, []byte("OK")) {
		return xerrors.Errorf("liveness check returned non-OK body: %s", body)
	}

	return nil
}
