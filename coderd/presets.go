package coderd

import (
	"net/http"

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

	var res []codersdk.Preset
	for _, preset := range presets {
		res = append(res, codersdk.Preset{
			ID:   preset.ID,
			Name: preset.Name,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, res)
}

// @Summary Get template version preset parameters
// @ID get-template-version-preset-parameters
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param templateversion path string true "Template version ID" format(uuid)
// @Success 200 {array} codersdk.PresetParameter
// @Router /templateversions/{templateversion}/presets/parameters [get]
func (api *API) templateVersionPresetParameters(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	templateVersion := httpmw.TemplateVersionParam(r)

	// TODO (sasswart): Test case: what if a user tries to read presets or preset parameters from a different org?
	// TODO (sasswart): Do a prelim auth check here.

	presetParams, err := api.Database.GetPresetParametersByTemplateVersionID(ctx, templateVersion.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching template version presets.",
			Detail:  err.Error(),
		})
		return
	}

	var res []codersdk.PresetParameter
	for _, presetParam := range presetParams {
		res = append(res, codersdk.PresetParameter{
			PresetID: presetParam.ID,
			Name:     presetParam.Name,
			Value:    presetParam.Value,
		})
	}

	httpapi.Write(ctx, rw, http.StatusOK, res)
}
