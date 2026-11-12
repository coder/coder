package coderd

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-jose/go-jose/v4"
	"github.com/mitchellh/mapstructure"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/awsidentity"
	"github.com/coder/coder/v2/coderd/azureidentity"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/podidentity"
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
	instanceID, err := azureidentity.Validate(r.Context(), req.Signature, api.AzureCertificates)
	if err != nil {
		// Log the full error for operators but return only a
		// generic message to the caller. Errors from the
		// certificate fetch path may contain fragments of
		// internal HTTP responses, so exposing them would be
		// an information disclosure risk.
		api.Logger.Warn(ctx, "azure identity validation failed",
			slog.Error(err),
		)
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "Invalid Azure identity.",
			Detail:  "Signature verification failed.",
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

	payload, err := api.GoogleTokenValidator.Validate(ctx, req.JSONWebToken, "coder")
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

// maxKubernetesPodIdentityWait bounds how long a pod's request is held.
const maxKubernetesPodIdentityWait = 30 * time.Second

// A workspace daemon in a Kubernetes pod proves which pod it is with the
// token the kubelet obtained for the pod. coderd checks the token against
// the signing keys of the pod's cluster, which an operator in that cluster
// registered. Nothing the pod says about itself is believed.
//
// A pod's daemon starts before the build that makes the pod is over, and
// so before the pod is registered as the workspace's. Such a pod is told
// to wait: the request is held until a workspace build registers the pod,
// or the wait runs out.
//
// @Summary Authenticate agent in a Kubernetes pod
// @ID authenticate-agent-in-a-kubernetes-pod
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Agents
// @Param request body agentsdk.KubernetesPodIdentityToken true "Pod identity token"
// @Success 200 {object} agentsdk.AuthenticateResponse
// @Router /api/v2/workspaceagents/kubernetes-pod-identity [post]
func (api *API) postWorkspaceAuthKubernetesPodIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if api.PodIdentity == nil {
		// Not found would tell a pod to wait to be registered.
		httpapi.Write(ctx, rw, http.StatusNotImplemented, codersdk.Response{
			Message: "Kubernetes pod identity is not configured on this deployment.",
		})
		return
	}
	var req agentsdk.KubernetesPodIdentityToken
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	// The token is checked against the keys of the clusters whose
	// operators registered them. coderd asks no cluster anything.
	pod, err := api.PodIdentity.Attest(req.Token, time.Now())
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "No registered cluster confirms the token as a pod's.",
		})
		return
	}
	instanceID := pod.InstanceID()

	wait := min(time.Duration(req.WaitSeconds)*time.Second, maxKubernetesPodIdentityWait)
	if wait > 0 {
		claimed := make(chan struct{}, 1)
		// Listening comes before looking, so that a claim in between is
		// not missed.
		unsubscribe, err := api.Pubsub.Subscribe(podidentity.RegisteredChannel(instanceID), func(context.Context, []byte) {
			select {
			case claimed <- struct{}{}:
			default:
			}
		})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		defer unsubscribe()
		deadline := time.NewTimer(wait)
		defer deadline.Stop()
		// Look now and then as well, in case an announcement is lost.
		look := time.NewTicker(5 * time.Second)
		defer look.Stop()
	waiting:
		for {
			//nolint:gocritic // Instance identity auth happens before agent auth.
			agents, err := api.Database.GetWorkspaceBuildAgentsByInstanceID(dbauthz.AsSystemRestricted(ctx), instanceID)
			if err != nil || len(agents) > 0 {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-deadline.C:
				break waiting
			case <-claimed:
			case <-look.C:
			}
		}
	}
	api.handleAuthInstanceID(rw, r, instanceID, req.AgentName)
}

func (api *API) handleAuthInstanceID(rw http.ResponseWriter, r *http.Request, instanceID string, agentName string) {
	ctx := r.Context()
	// Instance identity auth happens before the agent has a session token, so
	// these lookups must use a restricted system context.
	//nolint:gocritic // Instance identity auth happens before agent auth.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	agentName = strings.TrimSpace(agentName)

	agents, err := api.Database.GetWorkspaceBuildAgentsByInstanceID(systemCtx, instanceID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspace agent.",
			Detail:  err.Error(),
		})
		return
	}
	if len(agents) == 0 {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
			Message: fmt.Sprintf("Instance with id %q not found.", instanceID),
		})
		return
	}

	selected := agents[0]
	if agentName != "" {
		found := false
		for _, candidate := range agents {
			if candidate.WorkspaceAgent.Name == agentName {
				selected = candidate
				found = true
				break
			}
		}
		if !found {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: fmt.Sprintf("No agent found with instance ID %q and name %q.", instanceID, agentName),
			})
			return
		}
	} else if len(agents) != 1 {
		// Include agent names in the error message to help operators
		// configure CODER_AGENT_NAME. The caller has already proven
		// cloud instance identity, so agent names are not sensitive
		// here.
		names := make([]string, len(agents))
		for i, candidate := range agents {
			names[i] = candidate.WorkspaceAgent.Name
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
	agent := selected.WorkspaceAgent
	// This token should only be exchanged if the instance ID is valid
	// for the latest history. If an instance ID is recycled by a cloud,
	// we'd hate to leak access to a user's workspace.
	latestHistory, err := api.Database.GetLatestWorkspaceBuildByWorkspaceID(systemCtx, selected.WorkspaceTable.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching the latest workspace build.",
			Detail:  err.Error(),
		})
		return
	}
	if latestHistory.ID != selected.WorkspaceBuildID {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Resource found for id %q, but isn't registered on the latest history.", instanceID),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, agentsdk.AuthenticateResponse{
		SessionToken: agent.AuthToken.String(),
	})
}

// The operator of a cluster tells coderd how to recognize the cluster's pod
// tokens: the issuer, its public signing keys, and the namespaces whose
// pods may be workspace daemons. It renews this regularly. coderd believes
// a cluster for as long as its operator does.
//
// @Summary Register a cluster that issues pod identities
// @ID register-pod-identity-issuer
// @Accept json
// @Produce json
// @Tags Agents
// @Param issuer path string true "Name of the cluster"
// @Param request body codersdk.RegisterPodIdentityIssuerRequest true "The cluster's issuer and keys"
// @Success 200 {object} codersdk.PodIdentityIssuer
// @Router /api/v2/podidentity/issuers/{issuer} [put]
// @x-apidocgen {"skip": true}
func (api *API) putPodIdentityIssuer(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if api.PodIdentity == nil || api.PodIdentityOperatorKey == "" {
		httpapi.Write(ctx, rw, http.StatusNotImplemented, codersdk.Response{
			Message: "Kubernetes pod identity is not configured on this deployment.",
		})
		return
	}
	// The operator proves itself with a key that is good for this and
	// nothing else.
	presented := r.Header.Get(codersdk.PodIdentityOperatorKeyHeader)
	if subtle.ConstantTimeCompare([]byte(presented), []byte(api.PodIdentityOperatorKey)) != 1 {
		httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
			Message: "The operator key is not the one this deployment was given.",
		})
		return
	}
	var req codersdk.RegisterPodIdentityIssuerRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal(req.JWKS, &keys); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The cluster's signing keys are not a JSON Web Key Set.",
			Detail:  err.Error(),
		})
		return
	}
	issuer := podidentity.Issuer{
		Name:       chi.URLParam(r, "issuer"),
		URL:        req.IssuerURL,
		Keys:       keys,
		Namespaces: req.Namespaces,
		SeenAt:     time.Now(),
	}
	if err := api.PodIdentity.Put(issuer); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The cluster cannot be registered.",
			Detail:  err.Error(),
		})
		return
	}
	// Every replica must know the cluster. They hear of it here.
	if raw, err := podidentity.MarshalIssuer(issuer); err == nil {
		if err := api.Pubsub.Publish(podidentity.IssuerChannel, raw); err != nil {
			api.Logger.Warn(ctx, "announce a registered cluster", slog.Error(err))
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, podIdentityIssuer(issuer))
}

// @Summary List the clusters that issue pod identities
// @ID list-pod-identity-issuers
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Success 200 {array} codersdk.PodIdentityIssuer
// @Router /api/v2/podidentity/issuers [get]
// @x-apidocgen {"skip": true}
func (api *API) podIdentityIssuers(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceDeploymentConfig) {
		httpapi.Forbidden(rw)
		return
	}
	issuers := []codersdk.PodIdentityIssuer{}
	if api.PodIdentity != nil {
		for _, issuer := range api.PodIdentity.Issuers(time.Now()) {
			issuers = append(issuers, podIdentityIssuer(issuer))
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, issuers)
}

func podIdentityIssuer(issuer podidentity.Issuer) codersdk.PodIdentityIssuer {
	keyIDs := make([]string, 0, len(issuer.Keys.Keys))
	for _, key := range issuer.Keys.Keys {
		keyIDs = append(keyIDs, key.KeyID)
	}
	return codersdk.PodIdentityIssuer{
		Name:       issuer.Name,
		IssuerURL:  issuer.URL,
		KeyIDs:     keyIDs,
		Namespaces: issuer.Namespaces,
		SeenAt:     issuer.SeenAt,
	}
}
