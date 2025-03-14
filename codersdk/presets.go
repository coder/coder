package codersdk
import (
	"errors"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"github.com/google/uuid"
)
type Preset struct {
	ID         uuid.UUID
	Name       string
	Parameters []PresetParameter
}
type PresetParameter struct {
	Name  string
	Value string
}
// TemplateVersionPresets returns the presets associated with a template version.
func (c *Client) TemplateVersionPresets(ctx context.Context, templateVersionID uuid.UUID) ([]Preset, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templateversions/%s/presets", templateVersionID), nil)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var presets []Preset
	return presets, json.NewDecoder(res.Body).Decode(&presets)
}
