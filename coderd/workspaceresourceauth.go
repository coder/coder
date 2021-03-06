package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd/awsidentity"
	"github.com/coder/coder/coderd/azureidentity"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"

	"github.com/mitchellh/mapstructure"
)

// Azure supports instance identity verification:
// https://docs.microsoft.com/en-us/azure/virtual-machines/windows/instance-metadata-service?tabs=linux#tabgroup_14
func (api *API) postWorkspaceAuthAzureInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.AzureInstanceIdentityToken
	if !httpapi.Read(rw, r, &req) {
		return
	}
	instanceID, err := azureidentity.Validate(r.Context(), req.Signature, api.AzureCertificates)
	if err != nil {
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Invalid Azure identity.",
			Detail:  err.Error(),
		})
		return
	}
	api.handleAuthInstanceID(rw, r, instanceID)
}

// AWS supports instance identity verification:
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-identity-documents.html
// Using this, we can exchange a signed instance payload for an agent token.
func (api *API) postWorkspaceAuthAWSInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.AWSInstanceIdentityToken
	if !httpapi.Read(rw, r, &req) {
		return
	}
	identity, err := awsidentity.Validate(req.Signature, req.Document, api.AWSCertificates)
	if err != nil {
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Invalid AWS identity.",
			Detail:  err.Error(),
		})
		return
	}
	api.handleAuthInstanceID(rw, r, identity.InstanceID)
}

// Google Compute Engine supports instance identity verification:
// https://cloud.google.com/compute/docs/instances/verifying-instance-identity
// Using this, we can exchange a signed instance payload for an agent token.
func (api *API) postWorkspaceAuthGoogleInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	var req codersdk.GoogleInstanceIdentityToken
	if !httpapi.Read(rw, r, &req) {
		return
	}

	// We leave the audience blank. It's not important we validate who made the token.
	payload, err := api.GoogleTokenValidator.Validate(r.Context(), req.JSONWebToken, "")
	if err != nil {
		httpapi.Write(rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Invalid GCP identity.",
			Detail:  err.Error(),
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
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: "Error decoding JWT claims.",
			Detail:  err.Error(),
		})
		return
	}
	api.handleAuthInstanceID(rw, r, claims.Google.ComputeEngine.InstanceID)
}

func (api *API) handleAuthInstanceID(rw http.ResponseWriter, r *http.Request, instanceID string) {
	agent, err := api.Database.GetWorkspaceAgentByInstanceID(r.Context(), instanceID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Instance with id %q not found.", instanceID),
		})
		return
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job agent.",
			Detail:  err.Error(),
		})
		return
	}
	resource, err := api.Database.GetWorkspaceResourceByID(r.Context(), agent.ResourceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job resource.",
			Detail:  err.Error(),
		})
		return
	}
	job, err := api.Database.GetProvisionerJobByID(r.Context(), resource.JobID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if job.Type != database.ProvisionerJobTypeWorkspaceBuild {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%q jobs cannot be authenticated.", job.Type),
		})
		return
	}
	var jobData workspaceProvisionJob
	err = json.Unmarshal(job.Input, &jobData)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error extracting job data.",
			Detail:  err.Error(),
		})
		return
	}
	resourceHistory, err := api.Database.GetWorkspaceBuildByID(r.Context(), jobData.WorkspaceBuildID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	// This token should only be exchanged if the instance ID is valid
	// for the latest history. If an instance ID is recycled by a cloud,
	// we'd hate to leak access to a user's workspace.
	latestHistory, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(r.Context(), resourceHistory.WorkspaceID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching the latest workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	if latestHistory.ID != resourceHistory.ID {
		httpapi.Write(rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Resource found for id %q, but isn't registered on the latest history.", instanceID),
		})
		return
	}

	httpapi.Write(rw, http.StatusOK, codersdk.WorkspaceAgentAuthenticateResponse{
		SessionToken: agent.AuthToken.String(),
	})
}
