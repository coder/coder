package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
	"github.com/coder/coder/v2/codersdk"
)

// delegatedIdentityObject is what a caller is judged against when it acts on
// a delegated identity under parentID. An identity is owned by its parent.
// An ancestor further up holds it as its own as well, so that its own
// permissions and scopes decide. Anyone else needs the permission across the
// organization, which is how a trusted service acts on the identities of
// members.
//
// ancestors are the accounts above the identity, nearest first: its parent,
// and everyone above the parent.
func delegatedIdentityObject(caller rbac.Subject, organizationID uuid.UUID, ancestors []uuid.UUID) rbac.Object {
	owner := ancestors[0].String()
	for _, ancestor := range ancestors {
		if ancestor.String() == caller.ID {
			owner = caller.ID
			break
		}
	}
	return rbac.ResourceDelegatedIdentity.InOrg(organizationID).WithOwner(owner)
}

// delegatedIdentityAncestors returns the accounts above the identity at the
// bottom of chain, nearest first. The last one is the root user.
func delegatedIdentityAncestors(chain []database.DelegatedIdentity) []uuid.UUID {
	ancestors := make([]uuid.UUID, 0, len(chain))
	for _, identity := range chain {
		ancestors = append(ancestors, identity.ParentID)
	}
	return ancestors
}

// delegatedIdentityChain returns the delegated identity an account is and the
// identities above it, nearest first. It is empty for any other account.
func (api *API) delegatedIdentityChain(ctx context.Context, userID uuid.UUID) ([]database.DelegatedIdentity, error) {
	if !api.Experiments.Enabled(codersdk.ExperimentDelegatedIdentities) {
		return nil, nil
	}
	//nolint:gocritic // Who is above an identity decides what the caller may do, so it is read before the caller is judged.
	return api.Database.GetDelegatedIdentityChain(dbauthz.AsSystemRestricted(ctx), userID)
}

// onBehalfOfDelegatedIdentity decides who acts when a workspace of ownerID
// is created or built by somebody else. For an owner that is a delegated
// identity, a caller that may use the identity acts as the identity: with
// the identity's ID, and the roles and groups of its root user. The caller
// stays the initiator of what is done.
//
// It returns the identity that owns the workspace and the identities above
// it, which is empty when the owner is not a delegated identity or is the
// caller itself, and the context to act on its behalf in, which is nil when
// the caller may not use the identity.
func (api *API) onBehalfOfDelegatedIdentity(ctx context.Context, ownerID uuid.UUID) ([]database.DelegatedIdentity, context.Context, error) {
	caller, ok := dbauthz.ActorFromContext(ctx)
	if !ok || caller.ID == ownerID.String() {
		return nil, nil, nil
	}
	chain, err := api.delegatedIdentityChain(ctx, ownerID)
	if err != nil {
		return nil, nil, httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching the delegated identity that owns the workspace.",
			Detail:  err.Error(),
		})
	}
	if len(chain) == 0 {
		return nil, nil, nil
	}
	identity := chain[0]
	object := delegatedIdentityObject(caller, identity.OrganizationID, delegatedIdentityAncestors(chain))
	if !api.HTTPAuth.AuthorizeContext(ctx, policy.ActionUse, object) {
		return chain, nil, nil
	}
	subject, _, err := httpmw.UserRBACSubject(ctx, api.Database, identity.ID, rbac.ScopeAll)
	if err != nil {
		return nil, nil, httperror.NewResponseError(http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching the permissions of the delegated identity.",
			Detail:  err.Error(),
		})
	}
	return chain, dbauthz.As(ctx, subject), nil
}

// chatOnBehalfOfDelegatedIdentity lets an account that may use a delegated
// identity work with the chats the identity owns. The rest of the request
// runs as the identity, and the caller stays whoever its API key says. It
// runs before the chat is looked up: the caller may not be able to read it.
func (api *API) chatOnBehalfOfDelegatedIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		chatID, err := uuid.Parse(chi.URLParam(r, "chat"))
		if err != nil || !api.Experiments.Enabled(codersdk.ExperimentDelegatedIdentities) {
			next.ServeHTTP(rw, r)
			return
		}
		//nolint:gocritic // Who owns the chat decides whom the caller acts as. The caller is judged right after.
		chat, err := api.Database.GetChatByID(dbauthz.AsChatd(ctx), chatID)
		if err != nil {
			// The lookup that follows reports what is wrong.
			next.ServeHTTP(rw, r)
			return
		}
		_, identityCtx, err := api.onBehalfOfDelegatedIdentity(ctx, chat.OwnerID)
		if err != nil {
			httperror.WriteResponseError(ctx, rw, err)
			return
		}
		if identityCtx != nil {
			r = r.WithContext(identityCtx)
		}
		next.ServeHTTP(rw, r)
	})
}

// actsAsChatOwner reports whether a request is made by the owner of a chat
// or on the owner's behalf. The second is the case when the chat is owned by
// a delegated identity the caller may use: the chat routes then run the
// request as that identity. Chat handlers refuse anyone else, whatever RBAC
// says, because a chat runs with what its owner has.
func actsAsChatOwner(ctx context.Context, callerID uuid.UUID, chat database.Chat) bool {
	if callerID == chat.OwnerID {
		return true
	}
	actor, ok := dbauthz.ActorFromContext(ctx)
	return ok && actor.Type == rbac.SubjectTypeUser && actor.ID == chat.OwnerID.String()
}

// chatToolSubject returns who the tools of a chat act as when they create
// and build workspaces: the chat's owner. An owner that is a delegated
// identity is limited to what the identity may do, as a credential of the
// identity would be. It also returns the identity and the identities above
// it, which is empty for any other owner.
func (api *API) chatToolSubject(ctx context.Context, ownerID uuid.UUID) (rbac.Subject, []database.DelegatedIdentity, error) {
	chain, err := api.delegatedIdentityChain(ctx, ownerID)
	if err != nil {
		return rbac.Subject{}, nil, xerrors.Errorf("get the delegated identity of the chat owner: %w", err)
	}
	if len(chain) > 0 {
		subject, _, err := httpmw.DelegatedIdentityRBACSubject(ctx, api.Database, ownerID)
		return subject, chain, err
	}
	subject, _, err := httpmw.UserRBACSubject(ctx, api.Database, ownerID, rbac.ScopeAll)
	return subject, nil, err
}

// delegatedIdentityWorkspaceSharer may share workspaces and do nothing else.
var delegatedIdentityWorkspaceSharer = rbac.Subject{
	Type:         rbac.SubjectTypeSystemRestricted,
	FriendlyName: "Delegated Identity Workspace Sharer",
	ID:           uuid.Nil.String(),
	Roles: rbac.Roles([]rbac.Role{{
		Identifier:  rbac.RoleIdentifier{Name: "delegated-identity-workspace-sharer"},
		DisplayName: "Delegated Identity Workspace Sharer",
		Site: rbac.Permissions(map[string][]policy.Action{
			rbac.ResourceWorkspace.Type: {policy.ActionRead, policy.ActionShare},
		}),
		User:    []rbac.Permission{},
		ByOrgID: map[string]rbac.OrgPermissions{},
	}}),
	Scope: rbac.ScopeAll,
}.WithCachedASTValue()

// shareWithDelegatedIdentityAncestors lets the accounts above a delegated
// identity enter and stop a workspace that was created on its behalf. The
// identity gets nothing in return: sharing goes one way.
func shareWithDelegatedIdentityAncestors(ctx context.Context, tx database.Store, workspaceID uuid.UUID, ancestors []uuid.UUID) error {
	acl := database.WorkspaceACL{}
	for _, ancestor := range ancestors {
		acl[ancestor.String()] = database.WorkspaceACLEntry{
			Permissions: db2sdk.WorkspaceRoleActions(codersdk.WorkspaceRoleUse),
		}
	}
	// Sharing with those who answer for the identity is part of creating
	// its workspace. It is not a choice of the identity, whose root user
	// may not be allowed to share workspaces at all.
	err := tx.UpdateWorkspaceACLByID(dbauthz.As(ctx, delegatedIdentityWorkspaceSharer), database.UpdateWorkspaceACLByIDParams{
		ID:       workspaceID,
		UserACL:  acl,
		GroupACL: database.WorkspaceACL{},
	})
	if err != nil {
		return xerrors.Errorf("share the workspace with the accounts above its owner: %w", err)
	}
	return nil
}

// delegatedIdentityRequestScopes validates the scopes a delegated identity is
// to hold. parentChain is the delegated identity that is its parent and the
// identities above that, and is empty when the parent is a user.
func delegatedIdentityRequestScopes(requested []codersdk.APIKeyScope, parentChain []database.DelegatedIdentity) ([]string, *codersdk.Response) {
	scopes := make([]string, 0, len(requested))
	names := make([]rbac.ScopeName, 0, len(requested))
	for _, scope := range requested {
		name := rbac.CanonicalScopeName(rbac.ScopeName(scope))
		if !rbac.IsExternalScope(name) {
			return nil, &codersdk.Response{
				Message: fmt.Sprintf("%q is not a scope a delegated identity can be given.", scope),
				Detail:  "Use the scope names API tokens accept, for example workspace:read or template:read.",
			}
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
			scopes = append(scopes, string(name))
		}
	}
	if len(parentChain) == 0 {
		// The parent is a user. The identity is judged with that user's
		// roles, so it cannot hold more than the user whatever it is given.
		return scopes, nil
	}
	held := httpmw.DelegatedIdentityScopes(parentChain)
	missing, err := rbac.FirstScopeNotCovered(held, names)
	if err != nil || missing != "" {
		return nil, &codersdk.Response{
			Message: fmt.Sprintf("The parent identity %q does not hold the scope %q, so it cannot be handed on.", parentChain[0].Name, missing),
			Detail:  "A delegated identity can never hold more than its parent.",
		}
	}
	return scopes, nil
}

func delegatedIdentityScopes(scopes []rbac.ScopeName) []codersdk.APIKeyScope {
	out := make([]codersdk.APIKeyScope, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, codersdk.APIKeyScope(scope))
	}
	return out
}

// delegatedIdentitiesToSDK converts identities, looking up the names of
// their parents and root users.
func (api *API) delegatedIdentitiesToSDK(ctx context.Context, identities []database.DelegatedIdentity) ([]codersdk.DelegatedIdentity, error) {
	//nolint:gocritic // The caller was authorized to read the identities. The names of those above them are part of the record.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	ids := make([]uuid.UUID, 0, 2*len(identities))
	for _, identity := range identities {
		ids = append(ids, identity.ParentID, identity.RootUserID)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	ids = slices.Compact(ids)
	usernames := make(map[uuid.UUID]string, len(ids))
	if len(ids) > 0 {
		users, err := api.Database.GetUsersByIDs(sysCtx, ids)
		if err != nil {
			return nil, xerrors.Errorf("get the accounts above the identities: %w", err)
		}
		for _, user := range users {
			usernames[user.ID] = user.Username
		}
	}
	out := make([]codersdk.DelegatedIdentity, 0, len(identities))
	for _, identity := range identities {
		chain, err := api.Database.GetDelegatedIdentityChain(sysCtx, identity.ID)
		if err != nil {
			return nil, xerrors.Errorf("get the identities above %q: %w", identity.Name, err)
		}
		scopes := make([]rbac.ScopeName, 0, len(identity.Scopes))
		for _, scope := range identity.Scopes {
			scopes = append(scopes, rbac.ScopeName(scope))
		}
		out = append(out, codersdk.DelegatedIdentity{
			ID:              identity.ID,
			OrganizationID:  identity.OrganizationID,
			Name:            identity.Name,
			ParentID:        identity.ParentID,
			ParentName:      usernames[identity.ParentID],
			RootUserID:      identity.RootUserID,
			RootUsername:    usernames[identity.RootUserID],
			CreatedBy:       identity.CreatedBy,
			Scopes:          delegatedIdentityScopes(scopes),
			EffectiveScopes: delegatedIdentityScopes(httpmw.DelegatedIdentityScopes(chain)),
			CreatedAt:       identity.CreatedAt,
			UpdatedAt:       identity.UpdatedAt,
		})
	}
	return out, nil
}

func (api *API) writeDelegatedIdentity(ctx context.Context, rw http.ResponseWriter, status int, identity database.DelegatedIdentity) {
	converted, err := api.delegatedIdentitiesToSDK(ctx, []database.DelegatedIdentity{identity})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, status, converted[0])
}

// delegatedIdentityParam resolves the identity a request names, by ID, by
// name, or as "me", and judges the caller against it. It writes the response
// when there is no such identity for the caller.
func (api *API) delegatedIdentityParam(rw http.ResponseWriter, r *http.Request, action policy.Action) ([]database.DelegatedIdentity, bool) {
	ctx := r.Context()
	caller := httpmw.UserAuthorization(ctx)
	//nolint:gocritic // The identity is looked up to judge the caller against it.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	raw := chi.URLParam(r, "identity")
	var id uuid.UUID
	switch raw {
	case codersdk.DelegatedIdentityMe:
		parsed, err := uuid.Parse(caller.ID)
		if err != nil {
			httpapi.ResourceNotFound(rw)
			return nil, false
		}
		id = parsed
	default:
		parsed, err := uuid.Parse(raw)
		if err != nil {
			user, err := api.Database.GetUserByEmailOrUsername(sysCtx, database.GetUserByEmailOrUsernameParams{Username: raw})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.ResourceNotFound(rw)
					return nil, false
				}
				httpapi.InternalServerError(rw, err)
				return nil, false
			}
			parsed = user.ID
		}
		id = parsed
	}

	chain, err := api.Database.GetDelegatedIdentityChain(sysCtx, id)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return nil, false
	}
	if len(chain) == 0 {
		httpapi.ResourceNotFound(rw)
		return nil, false
	}
	identity := chain[0]
	user, err := api.Database.GetUserByID(sysCtx, identity.ID)
	if err != nil || user.Deleted {
		httpapi.ResourceNotFound(rw)
		return nil, false
	}

	// An identity may look at itself, and at nothing else about itself:
	// raising its own scopes is for those above it.
	if action == policy.ActionRead && caller.ID == identity.ID.String() {
		return chain, true
	}
	object := delegatedIdentityObject(caller, identity.OrganizationID, delegatedIdentityAncestors(chain))
	if !api.Authorize(r, action, object) {
		// Whoever may not read the identity is not told that it exists.
		if action == policy.ActionRead || !api.Authorize(r, policy.ActionRead, object) {
			httpapi.ResourceNotFound(rw)
			return nil, false
		}
		httpapi.Forbidden(rw)
		return nil, false
	}
	return chain, true
}

// @Summary Create a delegated identity (experimental)
// @ID create-delegated-identity
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param request body codersdk.CreateDelegatedIdentityRequest true "Create delegated identity request"
// @Success 201 {object} codersdk.DelegatedIdentity
// @Router /api/v2/identities [post]
// @x-apidocgen {"skip": true}
func (api *API) postDelegatedIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller := httpmw.UserAuthorization(ctx)
	callerID, err := uuid.Parse(caller.ID)
	if err != nil {
		httpapi.Forbidden(rw)
		return
	}
	var req codersdk.CreateDelegatedIdentityRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	//nolint:gocritic // The parent is looked up to judge the caller against it.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	parentID := req.ParentID
	if parentID == uuid.Nil {
		parentID = callerID
	}
	parent, err := api.Database.GetUserByID(sysCtx, parentID)
	if err != nil || parent.Deleted || parent.IsSystem {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			httpapi.InternalServerError(rw, err)
			return
		}
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "There is no such account to create the delegated identity under.",
		})
		return
	}
	parentChain, err := api.Database.GetDelegatedIdentityChain(sysCtx, parentID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// An identity lives in one organization: its parent's, when the parent
	// is an identity.
	organizationID := req.OrganizationID
	rootUserID := parentID
	if len(parentChain) > 0 {
		rootUserID = parentChain[0].RootUserID
		if organizationID != uuid.Nil && organizationID != parentChain[0].OrganizationID {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "A delegated identity is in the organization of its parent.",
			})
			return
		}
		organizationID = parentChain[0].OrganizationID
	} else {
		memberships, err := api.Database.GetOrganizationIDsByMemberIDs(sysCtx, []uuid.UUID{parentID})
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		var organizationIDs []uuid.UUID
		if len(memberships) > 0 {
			organizationIDs = memberships[0].OrganizationIDs
		}
		switch {
		case organizationID == uuid.Nil && len(organizationIDs) == 1:
			organizationID = organizationIDs[0]
		case organizationID == uuid.Nil:
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Name the organization of the delegated identity: its parent is not in exactly one.",
			})
			return
		case !slices.Contains(organizationIDs, organizationID):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "The parent is not a member of that organization.",
			})
			return
		}
	}

	// The accounts above the new identity: its parent, and everyone above
	// the parent.
	ancestors := append([]uuid.UUID{parentID}, delegatedIdentityAncestors(parentChain)...)
	if !api.Authorize(r, policy.ActionCreate, delegatedIdentityObject(caller, organizationID, ancestors)) {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "You can't create a delegated identity under this account.",
			Detail: "A delegated identity is created by its parent or an account above its parent, " +
				"or by an account that may create delegated identities across the organization.",
		})
		return
	}

	scopes, refusal := delegatedIdentityRequestScopes(req.Scopes, parentChain)
	if refusal != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *refusal)
		return
	}

	name := req.Name
	if name == "" {
		name = "identity-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	}
	if err := codersdk.NameValid(name); err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     fmt.Sprintf("%q cannot be the name of a delegated identity.", name),
			Validations: []codersdk.ValidationError{{Field: "name", Detail: err.Error()}},
		})
		return
	}
	_, err = api.Database.GetUserByEmailOrUsername(sysCtx, database.GetUserByEmailOrUsernameParams{Username: name})
	if err == nil {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message:     fmt.Sprintf("The name %q is taken.", name),
			Validations: []codersdk.ValidationError{{Field: "name", Detail: "This value is already in use and should be unique."}},
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpapi.InternalServerError(rw, err)
		return
	}

	auditReq, commitAudit := audit.InitRequest[database.DelegatedIdentity](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: organizationID,
	})
	defer commitAudit()

	var identity database.DelegatedIdentity
	err = api.Database.InTx(func(tx database.Store) error {
		// The identity is an account of its own. It has no way to sign in
		// and none of its parent's credentials. It holds no roles: it is
		// judged with those of its root user.
		user, err := api.CreateUser(sysCtx, tx, CreateUserRequest{
			CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
				Username:        name,
				Name:            name,
				ServiceAccount:  true,
				OrganizationIDs: []uuid.UUID{organizationID},
			},
			LoginType:         database.LoginTypeNone,
			SkipNotifications: true,
		})
		if err != nil {
			return xerrors.Errorf("create the account of the identity: %w", err)
		}
		identity, err = tx.InsertDelegatedIdentity(sysCtx, database.InsertDelegatedIdentityParams{
			ID:             user.ID,
			OrganizationID: organizationID,
			ParentID:       parentID,
			RootUserID:     rootUserID,
			CreatedBy:      callerID,
			Name:           user.Username,
			Scopes:         scopes,
			CreatedAt:      dbtime.Now(),
		})
		if err != nil {
			return xerrors.Errorf("record the identity: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	auditReq.New = identity
	api.writeDelegatedIdentity(ctx, rw, http.StatusCreated, identity)
}

// @Summary List delegated identities (experimental)
// @ID list-delegated-identities
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Success 200 {array} codersdk.DelegatedIdentity
// @Router /api/v2/identities [get]
// @x-apidocgen {"skip": true}
func (api *API) delegatedIdentities(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller := httpmw.UserAuthorization(ctx)
	callerID, err := uuid.Parse(caller.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusOK, []codersdk.DelegatedIdentity{})
		return
	}
	//nolint:gocritic // The identities below the caller are the caller's to list. Each is still judged below.
	identities, err := api.Database.GetDelegatedIdentitiesByAncestorID(dbauthz.AsSystemRestricted(ctx), callerID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	// Everything listed is below the caller, so the caller holds it as
	// its own. The caller's scopes still decide whether it may be read.
	readable := identities[:0]
	for _, identity := range identities {
		if api.Authorize(r, policy.ActionRead, rbac.ResourceDelegatedIdentity.InOrg(identity.OrganizationID).WithOwner(caller.ID)) {
			readable = append(readable, identity)
		}
	}
	converted, err := api.delegatedIdentitiesToSDK(ctx, readable)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, converted)
}

// @Summary Get a delegated identity (experimental)
// @ID get-delegated-identity
// @Security CoderSessionToken
// @Produce json
// @Tags Users
// @Param identity path string true "Identity ID, name, or me"
// @Success 200 {object} codersdk.DelegatedIdentity
// @Router /api/v2/identities/{identity} [get]
// @x-apidocgen {"skip": true}
func (api *API) delegatedIdentity(rw http.ResponseWriter, r *http.Request) {
	chain, ok := api.delegatedIdentityParam(rw, r, policy.ActionRead)
	if !ok {
		return
	}
	api.writeDelegatedIdentity(r.Context(), rw, http.StatusOK, chain[0])
}

// @Summary Update a delegated identity (experimental)
// @ID update-delegated-identity
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Users
// @Param identity path string true "Identity ID or name"
// @Param request body codersdk.UpdateDelegatedIdentityRequest true "Update delegated identity request"
// @Success 200 {object} codersdk.DelegatedIdentity
// @Router /api/v2/identities/{identity} [patch]
// @x-apidocgen {"skip": true}
func (api *API) patchDelegatedIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chain, ok := api.delegatedIdentityParam(rw, r, policy.ActionUpdate)
	if !ok {
		return
	}
	var req codersdk.UpdateDelegatedIdentityRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	scopes, refusal := delegatedIdentityRequestScopes(req.Scopes, chain[1:])
	if refusal != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, *refusal)
		return
	}
	auditReq, commitAudit := audit.InitRequest[database.DelegatedIdentity](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: chain[0].OrganizationID,
	})
	defer commitAudit()
	auditReq.Old = chain[0]

	//nolint:gocritic // The caller was judged against the identity above.
	identity, err := api.Database.UpdateDelegatedIdentityScopes(dbauthz.AsSystemRestricted(ctx), database.UpdateDelegatedIdentityScopesParams{
		ID:        chain[0].ID,
		Scopes:    scopes,
		UpdatedAt: dbtime.Now(),
	})
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	auditReq.New = identity
	api.writeDelegatedIdentity(ctx, rw, http.StatusOK, identity)
}

// @Summary Delete a delegated identity (experimental)
// @ID delete-delegated-identity
// @Security CoderSessionToken
// @Tags Users
// @Param identity path string true "Identity ID or name"
// @Success 204
// @Router /api/v2/identities/{identity} [delete]
// @x-apidocgen {"skip": true}
func (api *API) deleteDelegatedIdentity(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chain, ok := api.delegatedIdentityParam(rw, r, policy.ActionDelete)
	if !ok {
		return
	}
	identity := chain[0]
	//nolint:gocritic // The caller was judged against the identity above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	usage, err := api.Database.GetDelegatedIdentityUsage(sysCtx, identity.ID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	if usage.Workspaces > 0 || usage.Chats > 0 || usage.Children > 0 {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("The delegated identity %q is in use.", identity.Name),
			Detail: fmt.Sprintf("It owns %d workspaces and %d chats, and has %d delegated identities below it. Delete those first.",
				usage.Workspaces, usage.Chats, usage.Children),
		})
		return
	}
	auditReq, commitAudit := audit.InitRequest[database.DelegatedIdentity](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: identity.OrganizationID,
	})
	defer commitAudit()
	auditReq.Old = identity

	// The account goes, and with it its credentials. The record stays, so
	// that audit entries can still be traced to a root user.
	if err := api.Database.UpdateUserDeletedByID(sysCtx, identity.ID); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}
