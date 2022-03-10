package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	ProjectVersionParameterSchema []ProjectVersionParameterSchema `json:"schema"`
	Resources                     []WorkspaceResource             `json:"resources"`
}

func (c *Client) Templates(ctx context.Context) ([]Template, error) {
	res, err := c.request(ctx, http.MethodGet, "/api/v2/templates", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var templates []Template
	return templates, json.NewDecoder(res.Body).Decode(&templates)
}

func (c *Client) TemplateArchive(ctx context.Context, id string) ([]byte, string, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s", id), nil)
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
