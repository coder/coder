package coderd

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/render"

	"github.com/coder/coder/httpapi"

	"github.com/mitchellh/mapstructure"
)

type GoogleInstanceIdentityToken struct {
	JSONWebToken string `json:"json_web_token" validate:"required"`
}

// WorkspaceAgentAuthenticateResponse is returned when an instance ID
// has been exchanged for a session token.
type WorkspaceAgentAuthenticateResponse struct {
	SessionToken string `json:"session_token"`
}

// Google Compute Engine supports instance identity verification:
// https://cloud.google.com/compute/docs/instances/verifying-instance-identity
// Using this, we can exchange a signed instance payload for an agent token.
func (api *api) postAuthenticateWorkspaceAgentUsingGoogleInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	var req GoogleInstanceIdentityToken
	if !httpapi.Read(rw, r, &req) {
		return
	}

	// We leave the audience blank. It's not important we validate who made the token.
	payload, err := api.GoogleTokenValidator.Validate(r.Context(), req.JSONWebToken, "")
	if err != nil {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: fmt.Sprintf("validate: %s", err),
		})
		return
	}
	claims := struct {
		Google struct {
			ComputeEngine struct {
				InstanceID string `mapstructure:"instance_id"`
			} `mapstructure:"compute_engine"`
		} `mapstructure:"google"`
	}{}
	err = mapstructure.Decode(payload.Claims, &claims)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("decode jwt claims: %s", err),
		})
		return
	}
	resource, err := api.Database.GetWorkspaceResourceByInstanceID(r.Context(), claims.Google.ComputeEngine.InstanceID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, httpapi.Response{
			Message: fmt.Sprintf("instance with id %q not found", claims.Google.ComputeEngine.InstanceID),
		})
		return
	}
	resourceHistory, err := api.Database.GetWorkspaceHistoryByID(r.Context(), resource.WorkspaceHistoryID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get workspace history: %s", err),
		})
		return
	}
	// This token should only be exchanged if the instance ID is valid
	// for the latest history. If an instance ID is recycled by a cloud,
	// we'd hate to leak access to a user's workspace.
	latestHistory, err := api.Database.GetWorkspaceHistoryByWorkspaceIDWithoutAfter(r.Context(), resourceHistory.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get latest workspace history: %s", err),
		})
		return
	}
	if latestHistory.ID.String() != resourceHistory.ID.String() {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("resource found for id %q, but isn't registered on the latest history", claims.Google.ComputeEngine.InstanceID),
		})
		return
	}
	render.Status(r, http.StatusOK)
	render.JSON(rw, r, WorkspaceAgentAuthenticateResponse{
		SessionToken: resource.WorkspaceAgentToken,
	})
}
