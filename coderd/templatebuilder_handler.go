package coderd

import (
	"net/http"
	"sort"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/templatebuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/examples"
)

// @Summary List template builder base templates
// @ID list-template-builder-base-templates
// @Security CoderSessionToken
// @Produce json
// @Tags TemplateBuilder
// @Success 200 {object} codersdk.TemplateBuilderBasesResponse
// @Router /api/v2/templatebuilder/bases [get]
func (api *API) templateBuilderBases(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceTemplate.AnyOrganization()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	exampleList, err := examples.List()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error listing examples.",
			Detail:  err.Error(),
		})
		return
	}

	examplesByID := make(map[string]codersdk.TemplateExample, len(exampleList))
	for _, ex := range exampleList {
		examplesByID[ex.ID] = ex
	}

	bases := make([]codersdk.TemplateBuilderBase, 0, len(templatebuilder.BaseTemplateIDs()))
	for _, id := range templatebuilder.BaseTemplateIDs() {
		ex, ok := examplesByID[id]
		if !ok {
			api.Logger.Warn(ctx, "base template has no matching example",
				slog.F("base_template_id", id))
			continue
		}
		bases = append(bases, codersdk.TemplateBuilderBase{
			ID:          ex.ID,
			Name:        ex.Name,
			Description: ex.Description,
			Icon:        ex.Icon,
			OS:          string(templatebuilder.BaseTemplateOS(id)),
		})
	}

	sort.Slice(bases, func(i, j int) bool {
		return bases[i].Name < bases[j].Name
	})

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.TemplateBuilderBasesResponse{
		Bases: bases,
	})
}
