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

// @Summary List template builder modules
// @ID list-template-builder-modules
// @Security CoderSessionToken
// @Produce json
// @Tags TemplateBuilder
// @Param base query string false "Base template example ID for OS-compatibility filtering"
// @Success 200 {object} codersdk.TemplateBuilderModulesResponse
// @Router /api/v2/templatebuilder/modules [get]
func (api *API) templateBuilderModules(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionRead, rbac.ResourceTemplate.AnyOrganization()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	manifests, err := templatebuilder.LoadModules()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error loading module catalog.",
			Detail:  err.Error(),
		})
		return
	}

	// Resolve OS filter from the base query param.
	var filterOS templatebuilder.BaseOS
	if base := r.URL.Query().Get("base"); base != "" {
		filterOS = templatebuilder.BaseTemplateOS(base)
		if filterOS == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Unknown base template.",
				Detail:  "The \"base\" query parameter must be a valid base template ID.",
			})
			return
		}
	}

	modules := make([]codersdk.TemplateBuilderModule, 0, len(manifests))
	for _, m := range manifests {
		if filterOS != "" && !m.CompatibleWithOS(string(filterOS)) {
			continue
		}
		modules = append(modules, m.ToSDK())
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.TemplateBuilderModulesResponse{
		Modules: modules,
	})
}

// @Summary Compose template from base and modules
// @ID compose-template-builder
// @Security CoderSessionToken
// @Accept json
// @Produce application/x-tar
// @Tags TemplateBuilder
// @Param request body codersdk.TemplateBuilderComposeRequest true "Compose request"
// @Success 200
// @Router /api/v2/templatebuilder/compose [post]
func (api *API) templateBuilderCompose(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !api.Authorize(r, policy.ActionCreate, rbac.ResourceTemplate.AnyOrganization()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	var req codersdk.TemplateBuilderComposeRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if req.BaseTemplateID == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Missing base_template_id.",
		})
		return
	}

	composeReq := templatebuilder.ComposeRequest{
		BaseTemplateID: req.BaseTemplateID,
		RegistryURL:    api.DeploymentValues.TemplateBuilder.RegistryURL.String(),
	}
	for _, m := range req.Modules {
		composeReq.Modules = append(composeReq.Modules, templatebuilder.ComposeModule{
			ID:        m.ID,
			Variables: m.Variables,
		})
	}

	result, err := templatebuilder.Compose(composeReq)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Failed to compose template.",
			Detail:  err.Error(),
		})
		return
	}

	tarData, err := templatebuilder.BundleTar(result)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error bundling template.",
			Detail:  err.Error(),
		})
		return
	}

	rw.Header().Set("Content-Type", "application/x-tar")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(tarData)
}
