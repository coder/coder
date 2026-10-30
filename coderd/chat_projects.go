package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary List chat projects
// @ID list-chat-projects
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Success 200 {array} codersdk.ChatProject
// @Router /api/experimental/chats/projects [get]
// @x-apidocgen {"skip": true}
func (api *API) listChatProjects(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	projects, err := api.Database.GetChatProjectsOwnedOrSharedWithUserID(ctx, apiKey.UserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat projects.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, api.chatProjects(r, projects))
}

// @Summary Create chat project
// @ID create-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param request body codersdk.CreateChatProjectRequest true "Create chat project request"
// @Success 201 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects [post]
// @x-apidocgen {"skip": true}
func (api *API) postChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	organization := httpmw.OrganizationParam(r)

	var req codersdk.CreateChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Icon = strings.TrimSpace(req.Icon)
	if req.Name == "" {
		writeChatProjectFieldError(ctx, rw, "name", "Name is required.")
		return
	}
	if !validateChatProjectFields(ctx, rw, req.Name, req.Description, req.Icon) {
		return
	}
	if !api.Authorize(r, policy.ActionCreate, database.ChatProject{
		OrganizationID: organization.ID,
		OwnerID:        apiKey.UserID,
	}.RBACObject()) {
		httpapi.Forbidden(rw)
		return
	}
	modelConfigID := uuid.NullUUID{}
	if req.ModelConfigID != nil {
		if !api.validateChatProjectModelConfig(ctx, rw, apiKey.UserID, organization.ID, *req.ModelConfigID) {
			return
		}
		modelConfigID = uuid.NullUUID{UUID: *req.ModelConfigID, Valid: true}
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organization.ID,
	})
	defer commitAudit()

	var project database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		// The lock serializes a user's creates so concurrent requests cannot
		// both observe room under the cap.
		if err := tx.AcquireLock(ctx, database.GenLockID("chat-projects-owner:"+apiKey.UserID.String())); err != nil {
			return xerrors.Errorf("lock chat projects: %w", err)
		}
		// The count spans the user's projects in every organization, and the
		// caller was already authorized to create one.
		//nolint:gocritic // See above.
		count, err := tx.CountChatProjectsByOwnerID(dbauthz.AsSystemRestricted(ctx), apiKey.UserID)
		if err != nil {
			return xerrors.Errorf("count chat projects: %w", err)
		}
		if count >= maxChatProjectsPerOwner {
			return errChatProjectLimit
		}
		agentUser, err := api.createChatProjectAgentUser(ctx, tx, organization.ID, req.Name, req.Icon)
		if err != nil {
			return err
		}
		project, err = tx.InsertChatProject(ctx, database.InsertChatProjectParams{
			ID:             uuid.NullUUID{},
			OrganizationID: organization.ID,
			OwnerID:        apiKey.UserID,
			Name:           req.Name,
			Description:    req.Description,
			Icon:           req.Icon,
			AgentUserID:    uuid.NullUUID{UUID: agentUser.ID, Valid: true},
			ModelConfigID:  modelConfigID,
		})
		return err
	}, nil)
	if errors.Is(err, errChatProjectLimit) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("You can have at most %d chat projects. Delete a project to create another.", maxChatProjectsPerOwner),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to create chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = project
	httpapi.Write(ctx, rw, http.StatusCreated, api.chatProject(r, project))
}

// @Summary Get chat project
// @ID get-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (api *API) getChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionRead, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, api.chatProject(r, project))
}

// @Summary Update chat project
// @ID update-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Param request body codersdk.UpdateChatProjectRequest true "Update chat project request"
// @Success 200 {object} codersdk.ChatProject
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionUpdate, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

	var req codersdk.UpdateChatProjectRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
	}
	if req.Icon != nil {
		*req.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Name != nil && *req.Name == "" {
		writeChatProjectFieldError(ctx, rw, "name", "Name must not be blank.")
		return
	}
	name := ptr.NilToDefault(req.Name, project.Name)
	description := ptr.NilToDefault(req.Description, project.Description)
	icon := ptr.NilToDefault(req.Icon, project.Icon)
	if !validateChatProjectFields(ctx, rw, name, description, icon) {
		return
	}

	allowedTemplateIDs := ptr.NilToDefault(req.AllowedTemplateIDs, project.AllowedTemplateIDs)
	mcpServerIDs := ptr.NilToDefault(req.MCPServerIDs, project.MCPServerIDs)
	externalAuthProviderIDs := ptr.NilToDefault(req.ExternalAuthProviderIDs, project.ExternalAuthProviderIDs)
	allowedTemplateIDs, mcpServerIDs, externalAuthProviderIDs, ok := api.validateChatProjectRestrictions(
		ctx, rw, project.OrganizationID, req, allowedTemplateIDs, mcpServerIDs, externalAuthProviderIDs,
	)
	if !ok {
		return
	}
	parameterRules := project.TemplateParameterRules
	if req.TemplateParameterRules != nil {
		parameterRules, ok = api.validateChatProjectTemplateParameterRules(
			ctx, rw, project.OrganizationID, allowedTemplateIDs, *req.TemplateParameterRules,
		)
		if !ok {
			return
		}
	} else if len(allowedTemplateIDs) > 0 {
		// Rules for templates the agent can no longer use have no effect.
		parameterRules = slices.DeleteFunc(slices.Clone(parameterRules), func(rule database.ChatProjectTemplateParameterRule) bool {
			return !slices.Contains(allowedTemplateIDs, rule.TemplateID)
		})
	}
	invokableAgentIDs := project.InvokableAgentIDs
	if req.InvokableAgentIDs != nil {
		invokableAgentIDs, ok = api.validateChatProjectInvokableAgents(r, rw, project, *req.InvokableAgentIDs)
		if !ok {
			return
		}
	}
	modelConfigID := project.ModelConfigID
	if req.ModelConfigID != nil {
		modelConfigID = uuid.NullUUID{}
		if raw := strings.TrimSpace(*req.ModelConfigID); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				writeChatProjectFieldError(ctx, rw, "model_config_id", "Model must be a model ID, or empty to use the default model.")
				return
			}
			// Saving other settings keeps a model that has since become
			// unavailable; only a newly chosen model is checked.
			if id != project.ModelConfigID.UUID || !project.ModelConfigID.Valid {
				if !api.validateChatProjectModelConfig(ctx, rw, httpmw.APIKey(r).UserID, project.OrganizationID, id) {
					return
				}
			}
			modelConfigID = uuid.NullUUID{UUID: id, Valid: true}
		}
	}

	var updated database.ChatProject
	err := api.Database.InTx(func(tx database.Store) error {
		var err error
		updated, err = tx.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
			ID:                      project.ID,
			Name:                    name,
			Description:             description,
			Icon:                    icon,
			AllowedTemplateIDs:      allowedTemplateIDs,
			MCPServerIDs:            mcpServerIDs,
			ExternalAuthProviderIDs: externalAuthProviderIDs,
			TemplateParameterRules:  parameterRules,
			InvokableAgentIDs:       invokableAgentIDs,
			WorkspaceToolsEnabled:   ptr.NilToDefault(req.WorkspaceToolsEnabled, project.WorkspaceToolsEnabled),
			ModelConfigID:           modelConfigID,
		})
		if err != nil {
			return err
		}
		if err := deleteDetachedAgentMCPCredentials(ctx, tx, project, updated.MCPServerIDs); err != nil {
			return err
		}
		// The agent's name is its user's display name, so both change
		// together.
		return syncChatProjectAgentUser(ctx, tx, updated)
	}, nil)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat project.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = updated
	httpapi.Write(ctx, rw, http.StatusOK, api.chatProject(r, updated))
}

// deleteDetachedAgentMCPCredentials removes the agent user's stored token or
// GitHub App credential for every MCP server the update detached, so
// attaching the server again needs the owner to provide a credential again.
func deleteDetachedAgentMCPCredentials(ctx context.Context, tx database.Store, before database.ChatProject, attached []uuid.UUID) error {
	if !before.AgentUserID.Valid {
		return nil
	}
	//nolint:gocritic // The caller may update the agent, which owns these credentials through its user.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	for _, id := range before.MCPServerIDs {
		if slices.Contains(attached, id) {
			continue
		}
		err := ignoreNoRows(tx.DeleteMCPServerUserToken(sysCtx, database.DeleteMCPServerUserTokenParams{
			MCPServerConfigID: id,
			UserID:            before.AgentUserID.UUID,
		}))
		if err != nil {
			return xerrors.Errorf("delete agent MCP server token: %w", err)
		}
		err = ignoreNoRows(tx.DeleteMCPServerGitHubAppCredential(sysCtx, database.DeleteMCPServerGitHubAppCredentialParams{
			MCPServerConfigID: id,
			UserID:            before.AgentUserID.UUID,
		}))
		if err != nil {
			return xerrors.Errorf("delete agent GitHub App credential: %w", err)
		}
	}
	return nil
}

// @Summary Delete chat project
// @ID delete-chat-project
// @Security CoderSessionToken
// @Tags Chats
// @Param organization path string true "Organization ID" format(uuid)
// @Param project path string true "Chat project ID" format(uuid)
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/projects/{project} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteChatProject(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	project := httpmw.ChatProjectParam(r)
	if !api.Authorize(r, policy.ActionDelete, project.RBACObject()) {
		httpapi.ResourceNotFound(rw)
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatProject](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: project.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = project

	err := api.Database.InTx(func(tx database.Store) error {
		if err := tx.RemoveChatProjectFromInvokableAgents(ctx, project.ID); err != nil {
			return xerrors.Errorf("remove agent from invokable agents: %w", err)
		}
		return tx.DeleteChatProjectByID(ctx, project.ID)
	}, nil)
	if errors.Is(err, sql.ErrNoRows) || httpapi.Is404Error(err) {
		httpapi.ResourceNotFound(rw)
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat project.",
			Detail:  err.Error(),
		})
		return
	}
	if err := suspendChatProjectAgentUser(ctx, api.Database, project); err != nil {
		api.Logger.Warn(ctx, "suspend agent user", slog.F("project_id", project.ID), slog.Error(err))
	}
	rw.WriteHeader(http.StatusNoContent)
}

// maxChatProjectsPerOwner caps how many projects one user owns across all
// organizations, which bounds the project list every agents page loads.
const maxChatProjectsPerOwner = 100

var errChatProjectLimit = xerrors.New("chat project limit reached")

const (
	// The project name is embedded in every generation's system prompt and
	// memory tool descriptions, so it is kept short.
	chatProjectNameMaxChars        = 64
	chatProjectDescriptionMaxChars = 1024
	chatProjectIconMaxChars        = 256
)

// validateChatProjectFields writes a 400 response and returns false when a
// sanitized field exceeds its limit.
func validateChatProjectFields(ctx context.Context, rw http.ResponseWriter, name, description, icon string) bool {
	if utf8.RuneCountInString(name) > chatProjectNameMaxChars {
		writeChatProjectFieldError(ctx, rw, "name", fmt.Sprintf("Name must be at most %d characters.", chatProjectNameMaxChars))
		return false
	}
	// The name is also the agent user's display name, so it follows the
	// same rules and is never altered to fit them.
	if err := codersdk.UserRealNameValid(name); err != nil {
		writeChatProjectFieldError(ctx, rw, "name", fmt.Sprintf("Name is not a valid display name: %s.", err))
		return false
	}
	if utf8.RuneCountInString(description) > chatProjectDescriptionMaxChars {
		writeChatProjectFieldError(ctx, rw, "description", fmt.Sprintf("Description must be at most %d characters.", chatProjectDescriptionMaxChars))
		return false
	}
	if utf8.RuneCountInString(icon) > chatProjectIconMaxChars {
		writeChatProjectFieldError(ctx, rw, "icon", fmt.Sprintf("Icon must be at most %d characters.", chatProjectIconMaxChars))
		return false
	}
	return true
}

// maxChatProjectRestrictionEntries caps each restriction list on a project.
const maxChatProjectRestrictionEntries = 100

// validateChatProjectRestrictions deduplicates the restriction lists and
// checks that every changed entry refers to something the caller can use in
// the project's organization. It writes a 400 response and returns false on
// failure.
func (api *API) validateChatProjectRestrictions(
	ctx context.Context,
	rw http.ResponseWriter,
	organizationID uuid.UUID,
	req codersdk.UpdateChatProjectRequest,
	templateIDs, mcpServerIDs []uuid.UUID,
	externalAuthProviderIDs []string,
) (normalizedTemplateIDs, normalizedMCPServerIDs []uuid.UUID, normalizedExternalAuthProviderIDs []string, ok bool) {
	templateIDs = slice.Unique(templateIDs)
	mcpServerIDs = slice.Unique(mcpServerIDs)
	externalAuthProviderIDs = slice.Unique(externalAuthProviderIDs)
	for field, n := range map[string]int{
		"allowed_template_ids":       len(templateIDs),
		"mcp_server_ids":             len(mcpServerIDs),
		"external_auth_provider_ids": len(externalAuthProviderIDs),
	} {
		if n > maxChatProjectRestrictionEntries {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("At most %d entries are allowed.", maxChatProjectRestrictionEntries))
			return nil, nil, nil, false
		}
	}

	if req.AllowedTemplateIDs != nil {
		for _, id := range templateIDs {
			template, err := api.Database.GetTemplateByID(ctx, id)
			if err != nil || template.OrganizationID != organizationID || template.Deleted {
				writeChatProjectFieldError(ctx, rw, "allowed_template_ids", fmt.Sprintf("Template %s was not found in this organization.", id))
				return nil, nil, nil, false
			}
		}
	}
	if req.MCPServerIDs != nil {
		normalized, invalid, err := validateChatMCPServerIDs(ctx, api.Database, organizationID, mcpServerIDs)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return nil, nil, nil, false
		}
		if len(invalid) > 0 {
			writeChatProjectFieldError(ctx, rw, "mcp_server_ids", fmt.Sprintf("MCP server %s was not found or is disabled.", invalid[0]))
			return nil, nil, nil, false
		}
		mcpServerIDs = normalized
	}
	if req.ExternalAuthProviderIDs != nil {
		known := make(map[string]struct{}, len(api.ExternalAuthConfigs))
		for _, cfg := range api.ExternalAuthConfigs {
			known[cfg.ID] = struct{}{}
		}
		for _, id := range externalAuthProviderIDs {
			if _, ok := known[id]; !ok {
				writeChatProjectFieldError(ctx, rw, "external_auth_provider_ids", fmt.Sprintf("External auth provider %q is not configured.", id))
				return nil, nil, nil, false
			}
		}
	}
	if templateIDs == nil {
		templateIDs = []uuid.UUID{}
	}
	if mcpServerIDs == nil {
		mcpServerIDs = []uuid.UUID{}
	}
	if externalAuthProviderIDs == nil {
		externalAuthProviderIDs = []string{}
	}
	return templateIDs, mcpServerIDs, externalAuthProviderIDs, true
}

// maxChatProjectInvokableAgents matches how many invokable agents chatd
// describes to the model.
const maxChatProjectInvokableAgents = 20

// validateChatProjectInvokableAgents deduplicates the agents a project may
// invoke and checks that each is another agent in the same organization.
// Letting an agent invoke another lends the invoked agent's identity, so
// newly added agents need the caller's edit access, not just use access.
// It writes a 400 response and returns false on failure.
func (api *API) validateChatProjectInvokableAgents(
	r *http.Request,
	rw http.ResponseWriter,
	project database.ChatProject,
	ids []uuid.UUID,
) ([]uuid.UUID, bool) {
	ctx := r.Context()
	const field = "invokable_agent_ids"
	ids = slice.Unique(ids)
	if len(ids) > maxChatProjectInvokableAgents {
		writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("An agent can invoke at most %d other agents.", maxChatProjectInvokableAgents))
		return nil, false
	}
	for _, id := range ids {
		if id == project.ID {
			writeChatProjectFieldError(ctx, rw, field, "An agent cannot invoke itself.")
			return nil, false
		}
		// The caller's authorization applies, so agents they cannot read
		// are reported as not found.
		agent, err := api.Database.GetChatProjectByID(ctx, id)
		if err != nil && !httpapi.Is404Error(err) {
			httpapi.InternalServerError(rw, err)
			return nil, false
		}
		if err != nil || agent.OrganizationID != project.OrganizationID {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Agent %s was not found in this organization.", id))
			return nil, false
		}
		// Agents already listed stay, so losing edit access to one does not
		// block unrelated changes. Chatd rechecks access at invoke time.
		if !slices.Contains(project.InvokableAgentIDs, id) && !api.Authorize(r, policy.ActionUpdate, agent.RBACObject()) {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("You need edit access to agent %s to let another agent invoke it.", agent.Name))
			return nil, false
		}
	}
	if ids == nil {
		ids = []uuid.UUID{}
	}
	return ids, true
}

const (
	maxChatProjectParameterRuleValues     = 100
	maxChatProjectParameterRuleValueBytes = 1024
)

// validateChatProjectTemplateParameterRules checks that each rule names a
// parameter on the active version of a template the project may use, and
// that every allowed value is valid for that parameter. It writes a 400
// response and returns false on failure.
func (api *API) validateChatProjectTemplateParameterRules(
	ctx context.Context,
	rw http.ResponseWriter,
	organizationID uuid.UUID,
	allowedTemplateIDs []uuid.UUID,
	rules []codersdk.ChatProjectTemplateParameterRule,
) (database.ChatProjectTemplateParameterRules, bool) {
	const field = "template_parameter_rules"
	if len(rules) > maxChatProjectRestrictionEntries {
		writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("At most %d rules are allowed.", maxChatProjectRestrictionEntries))
		return nil, false
	}

	type ruleKey struct {
		templateID uuid.UUID
		name       string
	}
	seen := make(map[ruleKey]struct{}, len(rules))
	paramsByTemplate := make(map[uuid.UUID]map[string]database.TemplateVersionParameter)
	out := make(database.ChatProjectTemplateParameterRules, 0, len(rules))
	for _, rule := range rules {
		name := strings.TrimSpace(rule.ParameterName)
		key := ruleKey{templateID: rule.TemplateID, name: name}
		if _, dup := seen[key]; dup {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Parameter %q has more than one rule for the same template.", name))
			return nil, false
		}
		seen[key] = struct{}{}
		if len(allowedTemplateIDs) > 0 && !slices.Contains(allowedTemplateIDs, rule.TemplateID) {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Template %s is not in the allowed templates.", rule.TemplateID))
			return nil, false
		}

		params, ok := paramsByTemplate[rule.TemplateID]
		if !ok {
			template, err := api.Database.GetTemplateByID(ctx, rule.TemplateID)
			if err != nil || template.OrganizationID != organizationID || template.Deleted {
				writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Template %s was not found in this organization.", rule.TemplateID))
				return nil, false
			}
			versionParams, err := api.Database.GetTemplateVersionParameters(ctx, template.ActiveVersionID)
			if err != nil {
				httpapi.InternalServerError(rw, err)
				return nil, false
			}
			params = make(map[string]database.TemplateVersionParameter, len(versionParams))
			for _, p := range versionParams {
				params[p.Name] = p
			}
			paramsByTemplate[rule.TemplateID] = params
		}
		param, ok := params[name]
		if !ok {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Parameter %q does not exist on the template's active version.", name))
			return nil, false
		}

		values := slice.Unique(rule.AllowedValues)
		if len(values) == 0 {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Parameter %q needs at least one allowed value.", name))
			return nil, false
		}
		if len(values) > maxChatProjectParameterRuleValues {
			writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Parameter %q may have at most %d allowed values.", name, maxChatProjectParameterRuleValues))
			return nil, false
		}
		var options []string
		if len(param.Options) > 0 {
			var parsed []struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(param.Options, &parsed); err != nil {
				httpapi.InternalServerError(rw, xerrors.Errorf("parse options for parameter %q: %w", name, err))
				return nil, false
			}
			for _, o := range parsed {
				options = append(options, o.Value)
			}
		}
		for _, v := range values {
			if len(v) > maxChatProjectParameterRuleValueBytes {
				writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Allowed values for %q must be at most %d bytes.", name, maxChatProjectParameterRuleValueBytes))
				return nil, false
			}
			if len(options) > 0 && !slices.Contains(options, v) {
				writeChatProjectFieldError(ctx, rw, field, fmt.Sprintf("Value %q is not an option for parameter %q.", v, name))
				return nil, false
			}
		}
		out = append(out, database.ChatProjectTemplateParameterRule{
			TemplateID:    rule.TemplateID,
			ParameterName: name,
			AllowedValues: values,
		})
	}
	return out, true
}

// validateChatProjectModelConfig checks that the person setting an agent's
// model can use it, and writes a field error when they cannot.
func (api *API) validateChatProjectModelConfig(ctx context.Context, rw http.ResponseWriter, userID, organizationID, modelConfigID uuid.UUID) bool {
	_, reason, err := api.userCanUseChatModelConfig(ctx, userID, organizationID, modelConfigID)
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("check model availability: %w", err))
		return false
	}
	if reason != chatModelConfigAvailable {
		writeChatProjectFieldError(ctx, rw, "model_config_id", chatProjectModelUnavailableMessage(reason))
		return false
	}
	return true
}

// chatProjectModelUnavailableMessage explains why a model cannot be used.
func chatProjectModelUnavailableMessage(reason chatModelConfigUnavailableReason) string {
	switch reason {
	case chatModelConfigUnavailableProviderDisabled:
		return "The model's provider is disabled."
	case chatModelConfigUnavailableCredentialsMissing:
		return "The model's provider has no API key that can be used."
	default:
		return "The model was not found or is disabled."
	}
}

func writeChatProjectFieldError(ctx context.Context, rw http.ResponseWriter, field, detail string) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message:     detail,
		Validations: []codersdk.ValidationError{{Field: field, Detail: detail}},
	})
}
