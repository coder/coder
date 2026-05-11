package coderd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"

	"github.com/coder/coder/v2/coderd/awsidentity"
	"github.com/coder/coder/v2/coderd/azureidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
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
// @Param request body agentsdk.AzureInstanceIdentityToken true "Instance identity token. The optional agent_name field disambiguates when multiple agents share the same instance ID."
// @Success 200 {object} agentsdk.AuthenticateResponse
// @Router /api/v2/workspaceagents/azure-instance-identity [post]
func (api *API) postWorkspaceAuthAzureInstanceIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req agentsdk.AzureInstanceIdentityToken
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	instanceID, err := azureidentity.Validate(r.Context(), req.Signature, azureidentity.Options{
		VerifyOptions: api.AzureCertificates,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Invalid Azure identity.",
			Detail:  err.Error(),
		})
		return
	}
	api.handleAuthInstanceID(rw, r, instanceID, req.AgentName)
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
// @Param request body agentsdk.AWSInstanceIdentityToken true "Instance identity token. The optional agent_name field disambiguates when multiple agents share the same instance ID."
// @Success 200 {object} agentsdk.AuthenticateResponse
// @Router /api/v2/workspaceagents/aws-instance-identity [post]
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
	api.handleAuthInstanceID(rw, r, identity.InstanceID, req.AgentName)
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
// @Param request body agentsdk.GoogleInstanceIdentityToken true "Instance identity token. The optional agent_name field disambiguates when multiple agents share the same instance ID."
// @Success 200 {object} agentsdk.AuthenticateResponse
// @Router /api/v2/workspaceagents/google-instance-identity [post]
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
	api.handleAuthInstanceID(rw, r, claims.Google.ComputeEngine.InstanceID, req.AgentName)
}

func (api *API) handleAuthInstanceID(rw http.ResponseWriter, r *http.Request, instanceID string, agentName string) {
	ctx := r.Context()
	// Instance identity auth happens before the agent has a session token, so
	// these lookups must use a restricted system context.
	//nolint:gocritic // Instance identity auth happens before agent auth.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	agentName = strings.TrimSpace(agentName)

	agents, err := api.Database.GetWorkspaceAgentsByInstanceID(systemCtx, instanceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching provisioner job agent.",
			Detail:  err.Error(),
		})
		return
	}

	// Template version agents can share an instance ID with workspace build
	// agents. Keep only workspace build agents before resolving ambiguity so
	// template version agents do not force CODER_AGENT_NAME.
	//
	// Every rebuild of a workspace inserts new workspace_agents rows that
	// keep the original auth_instance_id, so historical agents accumulate
	// on long-lived instances. Restrict candidates to agents whose build
	// is still the latest for their workspace; stale agents from prior
	// builds must not authenticate (and would otherwise spuriously trip
	// the multi-agent ambiguity branch below). Pre-v2.33 the SQL query
	// masked this with ORDER BY created_at DESC LIMIT 1; the move to
	// :many to support multi-agent templates (#24325) removed that mask.
	//
	// This also subsumes the "instance ID recycled across workspaces"
	// guard the post-selection code used to perform: a recycled instance
	// only survives the filter if it points at the current build of its
	// current workspace.
	//
	// We attach the provisioner job and workspace build to each candidate
	// during the filter loop so the post-selection code below can read
	// them directly instead of re-querying.
	type instanceCandidate struct {
		agent          database.WorkspaceAgent
		job            database.ProvisionerJob
		workspaceBuild database.WorkspaceBuild
	}
	buildCandidates := make([]instanceCandidate, 0, len(agents))
	// Cache the latest build per workspace so an instance ID shared across
	// many stale agents on the same workspace only triggers one lookup.
	latestBuildByWorkspace := make(map[uuid.UUID]uuid.UUID)
	for _, candidate := range agents {
		resource, err := api.Database.GetWorkspaceResourceByID(systemCtx, candidate.ResourceID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching provisioner job resource.",
				Detail:  err.Error(),
			})
			return
		}
		job, err := api.Database.GetProvisionerJobByID(systemCtx, resource.JobID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching provisioner job.",
				Detail:  err.Error(),
			})
			return
		}
		if job.Type != database.ProvisionerJobTypeWorkspaceBuild {
			continue
		}
		var jobData provisionerdserver.WorkspaceProvisionJob
		if err := json.Unmarshal(job.Input, &jobData); err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error extracting job data.",
				Detail:  err.Error(),
			})
			return
		}
		workspaceBuild, err := api.Database.GetWorkspaceBuildByID(systemCtx, jobData.WorkspaceBuildID)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error fetching workspace build.",
				Detail:  err.Error(),
			})
			return
		}
		latestBuildID, ok := latestBuildByWorkspace[workspaceBuild.WorkspaceID]
		if !ok {
			latest, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(systemCtx, workspaceBuild.WorkspaceID)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching the latest workspace build.",
					Detail:  err.Error(),
				})
				return
			}
			latestBuildID = latest.ID
			latestBuildByWorkspace[workspaceBuild.WorkspaceID] = latestBuildID
		}
		if latestBuildID != workspaceBuild.ID {
			continue
		}
		buildCandidates = append(buildCandidates, instanceCandidate{
			agent:          candidate,
			job:            job,
			workspaceBuild: workspaceBuild,
		})
	}
	if len(buildCandidates) == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Instance with id %q not found.", instanceID),
		})
		return
	}

	var selected instanceCandidate
	if agentName != "" {
		for _, candidate := range buildCandidates {
			if candidate.agent.Name == agentName {
				selected = candidate
				break
			}
		}
		if selected.agent.ID == uuid.Nil {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("No agent found with instance ID %q and name %q.", instanceID, agentName),
			})
			return
		}
	} else {
		if len(buildCandidates) != 1 {
			// Include agent names in the error message to help operators
			// configure CODER_AGENT_NAME. The caller has already proven
			// cloud instance identity, so agent names are not sensitive
			// here.
			names := make([]string, len(buildCandidates))
			for i, candidate := range buildCandidates {
				names[i] = candidate.agent.Name
			}
			sort.Strings(names)
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: fmt.Sprintf(
					"Multiple agents found with instance ID %q. Set CODER_AGENT_NAME to one of: %s",
					instanceID,
					strings.Join(names, ", "),
				),
			})
			return
		}
		selected = buildCandidates[0]
	}
	// Recycled-instance protection and stale-build rejection were applied
	// during candidate filtering: selected.workspaceBuild is the latest
	// build of its workspace, so we can issue the session token directly.
	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.AuthenticateResponse{
		SessionToken: selected.agent.AuthToken.String(),
	})
}
