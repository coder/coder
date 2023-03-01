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
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/provisionerdserver"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"

	"github.com/mitchellh/mapstructure"
)

// Azure supports instance identity verification:
// https://docs.microsoft.com/en-us/azure/virtual-machines/windows/instance-metadata-service?tabs=linux#tabgroup_14
//
// @Summary Authenticate agent on Azure instance
// @ID authenticate-agent-on-azure-instance
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.AzureInstanceIdentityToken true "Instance identity token"
// @Success 200 {object} agentsdk.AuthenticateResponse
// @Router /workspaceagents/azure-instance-identity [post]
func (api *API) postWorkspaceAuthAzureInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req agentsdk.AzureInstanceIdentityToken
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	instanceID, err := azureidentity.Validate(req.Signature, api.AzureCertificates)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
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
//
// @Summary Authenticate agent on AWS instance
// @ID authenticate-agent-on-aws-instance
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.AWSInstanceIdentityToken true "Instance identity token"
// @Success 200 {object} agentsdk.AuthenticateResponse
// @Router /workspaceagents/aws-instance-identity [post]
func (api *API) postWorkspaceAuthAWSInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req agentsdk.AWSInstanceIdentityToken
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	identity, err := awsidentity.Validate(req.Signature, req.Document, api.AWSCertificates)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
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
//
// @Summary Authenticate agent on Google Cloud instance
// @ID authenticate-agent-on-google-cloud-instance
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.GoogleInstanceIdentityToken true "Instance identity token"
// @Success 200 {object} agentsdk.AuthenticateResponse
// @Router /workspaceagents/google-instance-identity [post]
func (api *API) postWorkspaceAuthGoogleInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req agentsdk.GoogleInstanceIdentityToken
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// We leave the audience blank. It's not important we validate who made the token.
	payload, err := api.GoogleTokenValidator.Validate(ctx, req.JSONWebToken, "")
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
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
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Error decoding JWT claims.",
			Detail:  err.Error(),
		})
		return
	}
	api.handleAuthInstanceID(rw, r, claims.Google.ComputeEngine.InstanceID)
}

func (api *API) handleAuthInstanceID(rw http.ResponseWriter, r *http.Request, instanceID string) {
	ctx := r.Context()
	//nolint:gocritic // needed for auth instance id
	agent, err := api.Database.GetWorkspaceAgentByInstanceID(dbauthz.AsSystemRestricted(ctx), instanceID)
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Instance with id %q not found.", instanceID),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job agent.",
			Detail:  err.Error(),
		})
		return
	}
	//nolint:gocritic // needed for auth instance id
	resource, err := api.Database.GetWorkspaceResourceByID(dbauthz.AsSystemRestricted(ctx), agent.ResourceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job resource.",
			Detail:  err.Error(),
		})
		return
	}
	//nolint:gocritic // needed for auth instance id
	job, err := api.Database.GetProvisionerJobByID(dbauthz.AsSystemRestricted(ctx), resource.JobID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job.",
			Detail:  err.Error(),
		})
		return
	}
	if job.Type != database.ProvisionerJobTypeWorkspaceBuild {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%q jobs cannot be authenticated.", job.Type),
		})
		return
	}
	var jobData provisionerdserver.WorkspaceProvisionJob
	err = json.Unmarshal(job.Input, &jobData)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error extracting job data.",
			Detail:  err.Error(),
		})
		return
	}
	//nolint:gocritic // needed for auth instance id
	resourceHistory, err := api.Database.GetWorkspaceBuildByID(dbauthz.AsSystemRestricted(ctx), jobData.WorkspaceBuildID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	// This token should only be exchanged if the instance ID is valid
	// for the latest history. If an instance ID is recycled by a cloud,
	// we'd hate to leak access to a user's workspace.
	//nolint:gocritic // needed for auth instance id
	latestHistory, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(dbauthz.AsSystemRestricted(ctx), resourceHistory.WorkspaceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching the latest workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	if latestHistory.ID != resourceHistory.ID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Resource found for id %q, but isn't registered on the latest history.", instanceID),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.AuthenticateResponse{
		SessionToken: agent.AuthToken.String(),
	})
}
