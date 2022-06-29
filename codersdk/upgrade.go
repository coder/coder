package codersdk

import (
	"context"
	"io"
	"net/http"

	"golang.org/x/xerrors"
)

func (c *Client) Upgrade(ctx context.Context) (io.ReadCloser, error) {
	res, err := c.Request(ctx, http.MethodGet, "/upgrade", nil)
	if err != nil {
		_ = res.Body.Close()
		return nil, xerrors.Errorf("do request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, readBodyAsError(res)
	}

	return res.Body, nil
}
