package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get template version presets
// @ID get-template-version-presets
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} codersdk.Preset
// @Router /templateversions/{templateversion}/presets [get]
func (api *API) templateVersionPresets(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	templateVersion := httpmw.TemplateVersionParam(r)

	presets, err := api.Database.GetPresetsByTemplateVersionID(ctx, templateVersion.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version presets.",
			Detail:  err.Error(),
		})
		return
	}

	presetParams, err := api.Database.GetPresetParametersByTemplateVersionID(ctx, database.GetPresetParametersByTemplateVersionIDParams{
		TemplateVersionID: templateVersion.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version presets.",
			Detail:  err.Error(),
		})
		return
	}

	var res []codersdk.Preset
	for _, preset := range presets {
		sdkPreset := codersdk.Preset{
			ID:   preset.ID,
			Name: preset.Name,
		}
		for _, presetParam := range presetParams {
			if presetParam.TemplateVersionPresetID != preset.ID {
				continue
			}

			sdkPreset.Parameters = append(sdkPreset.Parameters, codersdk.PresetParameter{
				Name:  presetParam.Name,
				Value: presetParam.Value,
			})
		}
		res = append(res, sdkPreset)
	}

	httpapi.Write(ctx, rw, http.StatusOK, res)
}
