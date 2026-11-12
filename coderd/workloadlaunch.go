package coderd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/wspubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner"
)

// workloadIdentityUsername is the name of the service account that is a
// workload's identity. It is unique because the identity's ID is.
func workloadIdentityUsername(identityID uuid.UUID) string {
	return "workload-" + strings.ReplaceAll(identityID.String(), "-", "")[:12]
}

// workloadOwnerSubject is who acts when coderd makes or deletes a workload's
// workspace: the identity's own account, which may also use the templates
// of its organization. The caller has checked that the workload's sponsor
// may use the template in question. The workload itself never acts as this
// subject: its token is the account's, narrowed to the identity's scopes.
func (api *API) workloadOwnerSubject(ctx context.Context, userID, organizationID uuid.UUID) (rbac.Subject, error) {
	//nolint:gocritic // The account's roles are looked up to act as the account.
	subject, _, err := httpmw.UserRBACSubject(dbauthz.AsSystemRestricted(ctx), api.Database, userID, rbac.ScopeAll)
	if err != nil {
		return rbac.Subject{}, err
	}
	roles, err := subject.Roles.Expand()
	if err != nil {
		return rbac.Subject{}, xerrors.Errorf("expand roles: %w", err)
	}
	roles = append(roles, rbac.Role{
		Identifier:  rbac.RoleIdentifier{Name: "workload-workspace-template", OrganizationID: organizationID},
		DisplayName: "Template of a workload's workspace",
		ByOrgID: map[string]rbac.OrgPermissions{
			organizationID.String(): {
				Org: rbac.Permissions(map[string][]policy.Action{
					rbac.ResourceTemplate.Type: {policy.ActionRead, policy.ActionUse},
				}),
			},
		},
	})
	// A new subject: the one that was looked up carries its roles in a
	// form that is computed once and kept.
	return rbac.Subject{
		FriendlyName: subject.FriendlyName,
		Email:        subject.Email,
		Type:         subject.Type,
		ID:           subject.ID,
		Roles:        rbac.Roles(roles),
		Groups:       subject.Groups,
		Scope:        subject.Scope,
	}.WithCachedASTValue(), nil
}

// awaitProvisionerJob waits for a provisioner job to end and returns an
// error unless it succeeded. It looks whenever changed says so, and now
// and then regardless.
func (api *API) awaitProvisionerJob(ctx context.Context, jobID uuid.UUID, changed <-chan struct{}) error {
	//nolint:gocritic // The job belongs to a build the caller was authorized to start.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	look := time.NewTicker(250 * time.Millisecond)
	defer look.Stop()
	for {
		job, err := api.Database.GetProvisionerJobByID(sysCtx, jobID)
		if err != nil {
			return xerrors.Errorf("get provisioner job: %w", err)
		}
		switch job.JobStatus {
		case database.ProvisionerJobStatusSucceeded:
			return nil
		case database.ProvisionerJobStatusFailed, database.ProvisionerJobStatusCanceled, database.ProvisionerJobStatusCanceling:
			return xerrors.Errorf("the build %s: %s", job.JobStatus, job.Error.String)
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		case <-look.C:
		}
	}
}

// @Summary Run a workload in a workspace of its own (experimental)
// @ID launch-workload
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Workspaces
// @Param request body codersdk.LaunchWorkloadRequest true "Launch workload request"
// @Success 201 {object} codersdk.Workload
// @Router /api/v2/workloads [post]
// @x-apidocgen {"skip": true}
func (api *API) postWorkload(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	// A workload cannot sponsor another: nothing says yet what it may
	// hand on.
	if isWorkloadCaller(r) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "A workload cannot sponsor a workload.",
		})
		return
	}
	var req codersdk.LaunchWorkloadRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	scopes, refusal := workloadRequestScopes(req)
	if refusal != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *refusal)
		return
	}
	id := uuid.New()
	name := req.Name
	if name == "" {
		name = "workload-" + id.String()[:8]
	}
	if !provisioner.AgentNameRegex.MatchString(name) || len(name) > 40 || name != strings.ToLower(name) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The workload name must be lowercase letters, digits and single hyphens, and at most 40 characters.",
		})
		return
	}
	workspaceName := req.WorkspaceName
	if workspaceName == "" {
		workspaceName = name
	}
	if err := codersdk.NameValid(workspaceName); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%q cannot be the name of the workload's workspace.", workspaceName),
			Detail:  err.Error(),
		})
		return
	}

	// The sponsor's own rights decide whether such a workspace may exist:
	// the sponsor must be allowed to make one from this template.
	template, err := api.Database.GetTemplateByID(ctx, req.TemplateID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	if !api.Authorize(r, policy.ActionUse, template) ||
		!api.Authorize(r, policy.ActionCreate, rbac.ResourceWorkspace.InOrg(template.OrganizationID).WithOwner(apiKey.UserID.String())) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You cannot make workspaces from this template, so you cannot sponsor a workload in one.",
		})
		return
	}

	//nolint:gocritic // The caller was authorized above. The identity is made on the caller's behalf.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	sponsor, err := api.Database.GetUserByID(sysCtx, apiKey.UserID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// The identity is an account of its own. It has no way to sign in, no
	// credential of its sponsor's, and no role beyond membership of the
	// organization.
	user, err := api.CreateUser(sysCtx, api.Database, CreateUserRequest{
		CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
			Username:        workloadIdentityUsername(id),
			Name:            name,
			ServiceAccount:  true,
			OrganizationIDs: []uuid.UUID{template.OrganizationID},
		},
		LoginType:         database.LoginTypeNone,
		SkipNotifications: true,
	})
	if err != nil {
		httpapi.InternalServerError(rw, xerrors.Errorf("create the workload's identity: %w", err))
		return
	}
	retireIdentity := func() {
		//nolint:gocritic // The caller may be gone; the identity must still be retired.
		if err := api.Database.UpdateUserDeletedByID(dbauthz.AsSystemRestricted(context.WithoutCancel(ctx)), user.ID); err != nil {
			api.Logger.Warn(ctx, "retire a workload identity that got no workspace", slog.F("user_id", user.ID), slog.Error(err))
		}
	}
	// From here on the identity acts, not its sponsor. The sponsor's
	// access to the template, checked above, is what lets the identity use
	// it: the identity is in none of the groups the template is open to.
	subject, err := api.workloadOwnerSubject(ctx, user.ID, template.OrganizationID)
	if err != nil {
		retireIdentity()
		httpapi.InternalServerError(rw, xerrors.Errorf("get the workload identity's permissions: %w", err))
		return
	}
	identityCtx := dbauthz.As(ctx, subject)

	// Hear about the workspace's build before it can be missed.
	changed := make(chan struct{}, 1)
	unsubscribe, err := api.Pubsub.Subscribe(wspubsub.WorkspaceEventChannel(user.ID), func(context.Context, []byte) {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	if err != nil {
		retireIdentity()
		httpapi.InternalServerError(rw, err)
		return
	}
	defer unsubscribe()

	auditReq, commitAudit := audit.InitRequest[database.WorkspaceTable](rw, &audit.RequestParams{
		Audit:   *api.Auditor.Load(),
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
		AdditionalFields: audit.AdditionalFields{
			WorkspaceOwner: user.Username,
		},
	})
	defer commitAudit()

	var identity database.WorkloadIdentity
	workspace, err := createWorkspace(identityCtx, auditReq, sponsor.ID, api,
		workspaceOwner{ID: user.ID, Username: user.Username},
		codersdk.CreateWorkspaceRequest{
			TemplateID:              template.ID,
			Name:                    workspaceName,
			TemplateVersionPresetID: req.TemplateVersionPresetID,
		},
		&createWorkspaceOptions{
			remoteAddr: r.RemoteAddr,
			postBuildInTX: func(ctx context.Context, tx database.Store, workspace database.Workspace, _ database.WorkspaceBuild) error {
				// The identity shares its workspace with its sponsor, who
				// can then enter and stop it, where the organization
				// lets workspaces be shared.
				if api.HTTPAuth.AuthorizeContext(ctx, policy.ActionShare, workspace) {
					err := tx.UpdateWorkspaceACLByID(ctx, database.UpdateWorkspaceACLByIDParams{
						ID: workspace.ID,
						UserACL: database.WorkspaceACL{
							sponsor.ID.String(): {Permissions: db2sdk.WorkspaceRoleActions(codersdk.WorkspaceRoleUse)},
						},
						GroupACL: database.WorkspaceACL{},
					})
					if err != nil {
						return xerrors.Errorf("share the workspace with the sponsor: %w", err)
					}
				} else {
					api.Logger.Warn(ctx, "the workload's workspace is not shared with its sponsor: the organization does not allow it",
						slog.F("workspace_id", workspace.ID), slog.F("sponsor_id", sponsor.ID))
				}
				// The record exists before anything is built. It has no
				// daemon until the workspace does.
				var err error
				//nolint:gocritic // Identities have no user-facing permission of their own yet.
				identity, err = tx.InsertWorkloadIdentity(dbauthz.AsSystemRestricted(ctx), database.InsertWorkloadIdentityParams{
					ID:             id,
					CreatedAt:      dbtime.Now(),
					OrganizationID: workspace.OrganizationID,
					SponsorID:      sponsor.ID,
					WorkspaceID:    workspace.ID,
					// It has no daemon until the workspace does.
					WorkspaceAgentID: uuid.NullUUID{},
					UserID:           uuid.NullUUID{UUID: user.ID, Valid: true},
					Name:             name,
					Command:          req.Command,
					Scopes:           scopes,
					Interactive:      req.Interactive,
					RemoveWorkspace:  !req.Keep,
				})
				if err != nil {
					return xerrors.Errorf("record the workload: %w", err)
				}
				return nil
			},
		})
	if err != nil {
		retireIdentity()
		httperror.WriteResponseError(ctx, rw, err)
		return
	}

	// Ending the workload removes its workspace, unless it is kept.
	giveUp := func(status int, message, detail string) {
		//nolint:gocritic // The caller may be gone; the workload must still be ended.
		if _, _, err := api.finishWorkload(context.WithoutCancel(ctx), identity, workloadEnd{
			Reason: codersdk.WorkloadEndReasonFailed, OutputTail: detail,
		}); err != nil {
			api.Logger.Error(ctx, "end a workload whose workspace was not built", slog.Error(err))
		}
		if ctx.Err() != nil {
			return
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{Message: message, Detail: detail})
	}
	buildCtx, cancel := context.WithTimeout(ctx, workloadStartTimeout)
	defer cancel()
	if err := api.awaitProvisionerJob(buildCtx, workspace.LatestBuild.Job.ID, changed); err != nil {
		giveUp(http.StatusBadGateway, "The workload's workspace could not be built.", err.Error())
		return
	}
	daemon, status, refusal := api.workloadDaemon(sysCtx, workspace.ID, "")
	if refusal != nil {
		giveUp(status, refusal.Message, refusal.Detail)
		return
	}
	identity, err = api.Database.UpdateWorkloadIdentityWorkspaceAgent(sysCtx, database.UpdateWorkloadIdentityWorkspaceAgentParams{
		ID:               identity.ID,
		WorkspaceAgentID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
	})
	if err != nil {
		giveUp(http.StatusInternalServerError, "The workload could not be handed to the workspace daemon.", err.Error())
		return
	}
	api.awaitWorkloadStart(rw, r, identity, daemon, sponsor)
}
