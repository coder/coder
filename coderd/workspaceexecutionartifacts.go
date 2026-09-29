package coderd

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/workspaceartifacts"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) registerWorkspaceExecutionArtifactRoutes(r chi.Router) {
	r.Get("/workspace-executions/{session}/artifacts", api.workspaceExecutionArtifacts)
	r.Get("/workspace-executions/{session}/artifacts/{artifact}", api.readWorkspaceExecutionArtifact)
}

// @Summary Read durable workspace execution artifact bytes
// @ID read-durable-workspace-execution-artifact-bytes
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce application/octet-stream
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Execution session ID" format(uuid)
// @Param artifact path string true "Artifact ID" format(uuid)
// @Param offset query int false "Zero-based byte offset"
// @Param limit query int false "Maximum bytes, up to 1048576; omit to download all bytes"
// @Success 200 {file} binary
// @Failure 410 {object} codersdk.Response
// @Router /api/v2/organizations/{organization}/workspace-executions/{session}/artifacts/{artifact} [get]
func (api *API) readWorkspaceExecutionArtifact(rw http.ResponseWriter, r *http.Request) {
	session, ok := api.workspaceExecutionSession(rw, r, policy.ActionSSH)
	if !ok {
		return
	}
	id, ok := httpmw.ParseUUIDParam(rw, r, "artifact")
	if !ok {
		return
	}
	offset, limit := int64(0), int64(workspaceartifacts.MaxBundleBytes)
	var err error
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.ParseInt(value, 10, 32)
	}
	if err == nil {
		if value := r.URL.Query().Get("limit"); value != "" {
			limit, err = strconv.ParseInt(value, 10, 32)
		}
	}
	if err != nil || offset < 0 || offset >= 1<<31-1 || limit < 1 || limit > workspaceartifacts.MaxBundleBytes || (r.URL.Query().Has("limit") && limit > codersdk.WorkspaceExecutionArtifactReadLimit) {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: "Invalid artifact byte range."})
		return
	}
	artifact, err := api.Database.ReadWorkspaceExecutionArtifact(r.Context(), database.ReadWorkspaceExecutionArtifactParams{ID: id, ByteOffset: int32(offset), ByteLimit: int32(limit)})
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
		} else {
			httpapi.InternalServerError(rw, err)
		}
		return
	}
	if artifact.SessionID != session.ID || artifact.OrganizationID != session.OrganizationID {
		httpapi.ResourceNotFound(rw)
		return
	}
	if artifact.ExpiresAt.Valid && !artifact.ExpiresAt.Time.After(api.Clock.Now()) {
		httpapi.Write(r.Context(), rw, http.StatusGone, codersdk.Response{Message: "Artifact bytes are no longer available."})
		return
	}
	if offset > artifact.SizeBytes {
		httpapi.Write(r.Context(), rw, http.StatusRequestedRangeNotSatisfiable, codersdk.Response{Message: "Offset exceeds artifact size."})
		return
	}
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Disposition", "attachment")
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("X-Artifact-SHA256", hex.EncodeToString(artifact.Sha256))
	rw.Header().Set("X-Artifact-Size", strconv.FormatInt(artifact.SizeBytes, 10))
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(artifact.Data)
}

// @Summary List durable workspace execution artifacts
// @ID list-durable-workspace-execution-artifacts
// @Security CoderSessionToken
// @Tags Workspaces
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param session path string true "Execution session ID" format(uuid)
// @Success 200 {array} codersdk.WorkspaceExecutionArtifact
// @Router /api/v2/organizations/{organization}/workspace-executions/{session}/artifacts [get]
func (api *API) workspaceExecutionArtifacts(rw http.ResponseWriter, r *http.Request) {
	session, ok := api.workspaceExecutionSession(rw, r, policy.ActionRead)
	if !ok {
		return
	}
	artifacts, err := api.Database.GetWorkspaceExecutionArtifactsBySessionID(r.Context(), session.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	result := make([]codersdk.WorkspaceExecutionArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		result = append(result, convertWorkspaceExecutionArtifact(artifact))
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, result)
}

func convertWorkspaceExecutionArtifact(a database.GetWorkspaceExecutionArtifactsBySessionIDRow) codersdk.WorkspaceExecutionArtifact {
	result := codersdk.WorkspaceExecutionArtifact{
		ID: a.ID, OrganizationID: a.OrganizationID, SessionID: a.SessionID, PreservationRevision: a.PreservationRevision,
		SourcePath: a.SourcePath, Name: a.Name, MIMEType: a.Mimetype, SizeBytes: a.SizeBytes, SHA256: hex.EncodeToString(a.Sha256), CreatedAt: a.CreatedAt,
		DownloadURL: fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s/artifacts/%s", a.OrganizationID, a.SessionID, a.ID),
	}
	if a.ExpiresAt.Valid {
		result.ExpiresAt = &a.ExpiresAt.Time
	}
	return result
}
