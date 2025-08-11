package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// This endpoint is experimental and not guaranteed to be stable, so we're not
// generating public-facing documentation for it.
func (api *API) aiTasksPrompts(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	buildIDsParam := r.URL.Query().Get("build_ids")
	if buildIDsParam == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "build_ids query parameter is required",
		})
		return
	}

	// Parse build IDs
	buildIDStrings := strings.Split(buildIDsParam, ",")
	buildIDs := make([]uuid.UUID, 0, len(buildIDStrings))
	for _, idStr := range buildIDStrings {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid build ID format: %s", idStr),
				Detail:  err.Error(),
			})
			return
		}
		buildIDs = append(buildIDs, id)
	}

	parameters, err := api.Database.GetWorkspaceBuildParametersByBuildIDs(ctx, buildIDs)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build parameters.",
			Detail:  err.Error(),
		})
		return
	}

	promptsByBuildID := make(map[string]string, len(parameters))
	for _, param := range parameters {
		if param.Name != codersdk.AITaskPromptParameterName {
			continue
		}
		buildID := param.WorkspaceBuildID.String()
		promptsByBuildID[buildID] = param.Value
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.AITasksPromptsResponse{
		Prompts: promptsByBuildID,
	})
}

// This endpoint is experimental and not guaranteed to be stable, so we're not
// generating public-facing documentation for it.
func (api *API) aiTasksCreate(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx     = r.Context()
		apiKey  = httpmw.APIKey(r)
		auditor = api.Auditor.Load()
	)

	var req codersdk.CreateAITasksRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	user, err := api.Database.GetUserByID(ctx, apiKey.UserID)
	if httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching user.",
			Detail:  err.Error(),
		})
		return
	}

	hasAIPrompt, err := api.Database.GetTemplateVersionHasAIPrompt(ctx, req.TemplateVersionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || rbac.IsUnauthorizedError(err) {
			httpapi.ResourceNotFound(rw)
			return
		}

		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching if template version has ai prompt.",
			Detail:  err.Error(),
		})
		return
	}
	if !hasAIPrompt {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: `Template does not have required parameter "AI Prompt"`,
		})
		return
	}

	createReq := codersdk.CreateWorkspaceRequest{
		Name:                    req.Name,
		TemplateVersionID:       req.TemplateVersionID,
		TemplateVersionPresetID: req.TemplateVersionPresetID,
		RichParameterValues: []codersdk.WorkspaceBuildParameter{
			{Name: "AI Prompt", Value: req.Prompt},
		},
	}

	owner := workspaceOwner{
		ID:        user.ID,
		Username:  user.Username,
		AvatarURL: user.AvatarURL,
	}

	aReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:   *auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: owner.Username,
		},
	})

	defer commitAudit()
	createWorkspace(ctx, aReq, apiKey.UserID, api, owner, createReq, rw, r)
}
