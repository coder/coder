package codersdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) InitScript(ctx context.Context, os, arch string) (string, error) {
	url := fmt.Sprintf("/api/v2/init-script/%s/%s", os, arch)
	res, err := c.Request(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", ReadBodyAsError(res)
	}

	script, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(script), nil
}
