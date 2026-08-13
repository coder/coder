package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

const maxAIEgressPolicyRules = 128

type aiEgressPolicyAuditFields struct {
	OldRevision int64                   `json:"old_revision"`
	NewRevision int64                   `json:"new_revision"`
	OldRules    []codersdk.AIEgressRule `json:"old_rules"`
	NewRules    []codersdk.AIEgressRule `json:"new_rules"`
}

// @Summary Get template AI egress policy
// @ID get-template-ai-egress-policy
// @Security CoderSessionToken
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Success 200 {object} codersdk.AIEgressPolicy
// @Router /api/v2/templates/{template}/ai-egress-policy [get]
func (api *API) templateAIEgressPolicy(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	template := httpmw.TemplateParam(r)

	policy, err := api.getTemplateAIEgressPolicy(ctx, template.ID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, policy)
}

// @Summary Update template AI egress policy
// @ID update-template-ai-egress-policy
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Templates
// @Param template path string true "Template ID" format(uuid)
// @Param request body codersdk.UpdateAIEgressPolicyRequest true "AI egress policy update request"
// @Success 200 {object} codersdk.AIEgressPolicy
// @Router /api/v2/templates/{template}/ai-egress-policy [put]
func (api *API) putTemplateAIEgressPolicy(rw http.ResponseWriter, r *http.Request) {
	var auditFields aiEgressPolicyAuditFields
	var (
		ctx      = r.Context()
		template = httpmw.TemplateParam(r)
		auditor  = *api.Auditor.Load()
		// Template audit diffs are empty because policy revisions are child rows,
		// so additional fields record the policy change.
		aReq, commitAudit = audit.InitRequest[database.Template](rw, &audit.RequestParams{
			Audit:            auditor,
			Log:              api.Logger,
			Request:          r,
			Action:           database.AuditActionWrite,
			OrganizationID:   template.OrganizationID,
			AdditionalFields: &auditFields,
		})
	)
	defer commitAudit()
	aReq.Old = template

	oldPolicy, err := api.getTemplateAIEgressPolicy(ctx, template.ID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	auditFields.OldRevision = oldPolicy.Revision
	auditFields.OldRules = oldPolicy.Rules

	var req codersdk.UpdateAIEgressPolicyRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	normalizedRules, validations := normalizeAIEgressRules(req.Rules)
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid AI egress policy.",
			Validations: validations,
		})
		return
	}

	rulesJSON, err := json.Marshal(normalizedRules)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("marshal AI egress policy rules: %w", err))
		return
	}

	inserted, err := api.Database.InsertTemplateAIEgressPolicy(ctx, database.InsertTemplateAIEgressPolicyParams{
		TemplateID: template.ID,
		Rules:      rulesJSON,
		CreatedBy:  httpmw.APIKey(r).UserID,
	})
	if err != nil {
		if httpapi.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, xerrors.Errorf("insert template AI egress policy: %w", err))
		return
	}

	policy, err := convertTemplateAIEgressPolicy(inserted)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	auditFields.NewRevision = policy.Revision
	auditFields.NewRules = policy.Rules
	aReq.New = template
	api.publishTemplateAIEgressPolicyUpdate(ctx, template.ID)

	httpapi.Write(ctx, rw, http.StatusOK, policy)
}

// writeUnlessAIBound reports whether the caller may read egress policy, and
// writes a 403 when it may not. Egress policy is the confining party's
// configuration, so the confined party must never receive it: only an unbound
// agent can be an egress supervisor. This is the same predicate that governs
// credential starvation, so policy delivery and credential denial cannot drift
// apart. A confined process can map its own policy by probing, so this is an
// invariant boundary rather than a secrecy boundary.
func writeUnlessAIBound(ctx context.Context, rw http.ResponseWriter, agent database.WorkspaceAgent) bool {
	if aiagentidentity.WorkspaceAgentAllowsOwnerCredentials(agent) {
		return true
	}
	httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
		Message: "AI egress policy is not available to an AI-bound workspace agent.",
		Detail:  "Egress policy is delivered to the supervising agent, never to the confined agent.",
	})
	return false
}

// @Summary Get workspace agent AI egress policy
// @ID get-workspace-agent-ai-egress-policy
// @Security CoderSessionToken
// @Produce json
// @Tags Agents
// @Success 200 {object} codersdk.AIEgressPolicy
// @Router /api/v2/workspaceagents/me/ai-egress-policy [get]
func (api *API) workspaceAgentAIEgressPolicy(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !writeUnlessAIBound(ctx, rw, httpmw.WorkspaceAgent(r)) {
		return
	}
	workspace, err := api.workspaceAgentWorkspace(ctx, r)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	policy, err := api.materializedTemplateAIEgressPolicy(ctx, workspace.TemplateID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, policy)
}

// @Summary Watch workspace agent AI egress policy
// @ID watch-workspace-agent-ai-egress-policy
// @Security CoderSessionToken
// @Produce text/event-stream
// @Tags Agents
// @Success 200 {object} codersdk.AIEgressPolicy
// @Router /api/v2/workspaceagents/me/ai-egress-policy/watch [get]
func (api *API) watchWorkspaceAgentAIEgressPolicy(rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)

	workspaceAgent := httpmw.WorkspaceAgent(r)
	if !writeUnlessAIBound(ctx, rw, workspaceAgent) {
		return
	}
	workspace, err := api.workspaceAgentWorkspace(ctx, r)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	log := api.Logger.Named("workspace_agent_ai_egress_policy_watcher").With(
		slog.F("workspace_agent_id", workspaceAgent.ID),
		slog.F("workspace_id", workspace.ID),
		slog.F("template_id", workspace.TemplateID),
	)
	updates := make(chan struct{}, 1)
	cancelSubscribe, err := api.Pubsub.SubscribeWithErr(templateAIEgressPolicyChannel(workspace.TemplateID), func(callbackCtx context.Context, _ []byte, err error) {
		if err != nil {
			log.Warn(callbackCtx, "template AI egress policy update delivered with error", slog.Error(err))
		}
		select {
		case updates <- struct{}{}:
		default:
		}
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("subscribe to template AI egress policy updates: %w", err))
		return
	}
	defer cancelSubscribe()

	policy, err := api.materializedTemplateAIEgressPolicy(ctx, workspace.TemplateID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	sendEvent, senderClosed, err := httpapi.ServerSentEventSender(rw, r)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("create AI egress policy event sender: %w", err))
		return
	}
	if err := sendEvent(codersdk.ServerSentEvent{
		Type: codersdk.ServerSentEventTypeData,
		Data: policy,
	}); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-senderClosed:
			return
		case <-api.ctx.Done():
			return
		case <-updates:
			policy, err := api.materializedTemplateAIEgressPolicy(ctx, workspace.TemplateID)
			if err != nil {
				log.Error(ctx, "fetch updated AI egress policy", slog.Error(err))
				_ = sendEvent(codersdk.ServerSentEvent{
					Type: codersdk.ServerSentEventTypeError,
					Data: codersdk.Response{
						Message: "Internal error fetching AI egress policy.",
						Detail:  err.Error(),
					},
				})
				return
			}
			if err := sendEvent(codersdk.ServerSentEvent{
				Type: codersdk.ServerSentEventTypeData,
				Data: policy,
			}); err != nil {
				return
			}
		}
	}
}

func (api *API) workspaceAgentWorkspace(ctx context.Context, r *http.Request) (database.Workspace, error) {
	workspaceAgent := httpmw.WorkspaceAgent(r)
	latestBuild := httpmw.LatestBuild(r)
	workspace, err := api.Database.GetWorkspaceByID(ctx, latestBuild.WorkspaceID)
	if err != nil {
		return database.Workspace{}, xerrors.Errorf("get workspace for agent %s: %w", workspaceAgent.ID, err)
	}
	return workspace, nil
}

func (api *API) getTemplateAIEgressPolicy(ctx context.Context, templateID uuid.UUID) (codersdk.AIEgressPolicy, error) {
	policy, err := api.Database.GetTemplateAIEgressPolicy(ctx, templateID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return codersdk.AIEgressPolicy{
			TemplateID: templateID,
			Rules:      []codersdk.AIEgressRule{},
		}, nil
	}
	if err != nil {
		return codersdk.AIEgressPolicy{}, xerrors.Errorf("get template AI egress policy: %w", err)
	}
	return convertTemplateAIEgressPolicy(policy)
}

func convertTemplateAIEgressPolicy(policy database.TemplateAIEgressPolicy) (codersdk.AIEgressPolicy, error) {
	rules := make([]codersdk.AIEgressRule, 0)
	if err := json.Unmarshal(policy.Rules, &rules); err != nil {
		return codersdk.AIEgressPolicy{}, xerrors.Errorf("unmarshal template AI egress policy rules: %w", err)
	}
	return codersdk.AIEgressPolicy{
		TemplateID: policy.TemplateID,
		Revision:   policy.Revision,
		Rules:      rules,
		UpdatedAt:  policy.CreatedAt,
		UpdatedBy:  policy.CreatedBy,
	}, nil
}

// materializedTemplateAIEgressPolicy reads with system privileges, so callers
// must derive templateID from the authenticated workspace agent's own build.
func (api *API) materializedTemplateAIEgressPolicy(ctx context.Context, templateID uuid.UUID) (codersdk.AIEgressPolicy, error) {
	//nolint:gocritic // The policy is the authenticated agent's own confinement input.
	policy, err := api.getTemplateAIEgressPolicy(dbauthz.AsSystemRestricted(ctx), templateID)
	if err != nil {
		return codersdk.AIEgressPolicy{}, err
	}

	port, err := accessURLPort(api.AccessURL.Scheme, api.AccessURL.Port())
	if err != nil {
		return codersdk.AIEgressPolicy{}, err
	}
	policy.Rules = append(policy.Rules, codersdk.AIEgressRule{
		Host:  strings.ToLower(api.AccessURL.Hostname()),
		Ports: []int{port},
	})
	return policy, nil
}

func accessURLPort(scheme, explicitPort string) (int, error) {
	if explicitPort != "" {
		port, err := strconv.Atoi(explicitPort)
		if err != nil || port < 1 || port > 65535 {
			return 0, xerrors.Errorf("invalid access URL port %q", explicitPort)
		}
		return port, nil
	}

	switch strings.ToLower(scheme) {
	case "https":
		return 443, nil
	case "http":
		return 80, nil
	default:
		return 0, xerrors.Errorf("unsupported access URL scheme %q", scheme)
	}
}

func normalizeAIEgressRules(rules []codersdk.AIEgressRule) ([]codersdk.AIEgressRule, []codersdk.ValidationError) {
	normalized := make([]codersdk.AIEgressRule, len(rules))
	validations := make([]codersdk.ValidationError, 0)
	if len(rules) > maxAIEgressPolicyRules {
		validations = append(validations, codersdk.ValidationError{
			Field:  "rules",
			Detail: fmt.Sprintf("Must contain no more than %d rules.", maxAIEgressPolicyRules),
		})
	}

	for i, rule := range rules {
		host := strings.ToLower(rule.Host)
		hostField := fmt.Sprintf("rules[%d].host", i)
		switch {
		case host == "":
			validations = append(validations, codersdk.ValidationError{Field: hostField, Detail: "Host must not be empty."})
		case len(host) > 253:
			validations = append(validations, codersdk.ValidationError{Field: hostField, Detail: "Host must be no more than 253 characters."})
		case strings.IndexFunc(host, unicode.IsSpace) >= 0:
			validations = append(validations, codersdk.ValidationError{Field: hostField, Detail: "Host must not contain whitespace."})
		case strings.ContainsAny(host, "/@:"):
			validations = append(validations, codersdk.ValidationError{Field: hostField, Detail: "Host must not contain a scheme, path, port, or user information."})
		case strings.ContainsRune(host, '*') && (!strings.HasPrefix(host, "*.") || strings.Count(host, "*") != 1 || len(host) == 2 || strings.HasPrefix(host[2:], ".")):
			validations = append(validations, codersdk.ValidationError{Field: hostField, Detail: "Wildcard must be a single leading '*.' label."})
		}

		ports := append([]int(nil), rule.Ports...)
		for j, port := range ports {
			if port < 1 || port > 65535 {
				validations = append(validations, codersdk.ValidationError{
					Field:  fmt.Sprintf("rules[%d].ports[%d]", i, j),
					Detail: "Port must be between 1 and 65535.",
				})
			}
		}
		normalized[i] = codersdk.AIEgressRule{Host: host, Ports: ports}
	}

	return normalized, validations
}

func templateAIEgressPolicyChannel(templateID uuid.UUID) string {
	return fmt.Sprintf("template-ai-egress-policy:%s", templateID)
}

func (api *API) publishTemplateAIEgressPolicyUpdate(ctx context.Context, templateID uuid.UUID) {
	err := api.Pubsub.Publish(templateAIEgressPolicyChannel(templateID), []byte{})
	if err != nil {
		api.Logger.Warn(ctx, "failed to publish template AI egress policy update",
			slog.F("template_id", templateID), slog.Error(err))
	}
}
