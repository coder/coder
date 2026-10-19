package coderd

import (
	"database/sql"
	"errors"
	"maps"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get an AI model ACL
// @ID get-an-ai-model-acl
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Success 200 {object} codersdk.ChatModelACL
// @Router /api/v2/organizations/{organization}/chats/models/{model}/acl [get]
// @x-apidocgen {"skip": true}
func (api *API) chatModelConfigACLHandler(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	users, err := resolveACLUsers(ctx, api.Database, config.UserACL, chatACLUser)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	groups, err := resolveACLGroups(ctx, api.Database, config.OrganizationID, config.GroupACL, chatACLGroup)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.ChatModelACL{
		Users:  users,
		Groups: groups,
	})
}

// @Summary Get available AI model ACL users and groups
// @ID get-available-ai-model-acl-users-and-groups
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Param q query string false "User search query; free-text search also applies to groups"
// @Param after_id query string false "User after ID" format(uuid)
// @Param limit query int false "Page limit for users and groups, if 0 returns all candidates"
// @Param offset query int false "User page offset"
// @Success 200 {object} codersdk.ACLAvailable
// @Router /api/v2/organizations/{organization}/chats/models/{model}/acl/available [get]
// @x-apidocgen {"skip": true}
func (api *API) chatModelConfigACLAvailable(rw http.ResponseWriter, r *http.Request) {
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	api.writeOrganizationACLAvailable(rw, r, config.OrganizationID)
}

type chatModelACLValidationError struct {
	Validations []codersdk.ValidationError
}

func (*chatModelACLValidationError) Error() string {
	return "invalid chat model ACL"
}

// @Summary Update an AI model ACL
// @ID update-an-ai-model-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Param request body codersdk.UpdateChatModelACLRequest true "Sparse model ACL update"
// @Success 204
// @Router /api/v2/organizations/{organization}/chats/models/{model}/acl [patch]
// @x-apidocgen {"skip": true}
func (api *API) updateChatModelConfigACL(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	auditor := api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.ChatModelConfig](rw, &audit.RequestParams{
		Audit:          *auditor,
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: config.OrganizationID,
	})
	defer commitAudit()
	aReq.Old = config

	var req codersdk.UpdateChatModelACLRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	var updated database.ChatModelConfig
	err := api.inChatModelConfigWriteTx(ctx, config.OrganizationID, func(tx database.Store) error {
		//nolint:gocritic // The ACL write below reauthorizes the locked row for share.
		current, err := tx.GetChatModelConfigByID(dbauthz.AsSystemRestricted(ctx), config.ID)
		if err != nil {
			return xerrors.Errorf("get chat model config for ACL update: %w", err)
		}
		aReq.Old = current

		userRoles, groupRoles, validations := validateOrganizationACLUpdate(ctx, tx, config.OrganizationID, ChatModelACLUpdateValidator(req))
		if len(validations) > 0 {
			return &chatModelACLValidationError{Validations: validations}
		}

		userACL := maps.Clone(current.UserACL)
		groupACL := maps.Clone(current.GroupACL)
		applyACLReadRoles(userACL, userRoles)
		applyACLReadRoles(groupACL, groupRoles)

		updated, err = tx.UpdateChatModelConfigACLByID(ctx, database.UpdateChatModelConfigACLByIDParams{
			ID:        config.ID,
			UserACL:   userACL,
			GroupACL:  groupACL,
			UpdatedBy: uuid.NullUUID{UUID: apiKey.UserID, Valid: apiKey.UserID != uuid.Nil},
		})
		return err
	})
	if err != nil {
		var validationErr *chatModelACLValidationError
		switch {
		case errors.As(err, &validationErr):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid request to update chat model ACL.",
				Validations: validationErr.Validations,
			})
		case dbauthz.IsNotAuthorizedError(err):
			httpapi.Forbidden(rw)
		case xerrors.Is(err, sql.ErrNoRows), xerrors.Is(err, errChatModelConfigNotFound), httpapi.Is404Error(err):
			httpapi.ResourceNotFound(rw)
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to update chat model ACL.",
				Detail:  err.Error(),
			})
		}
		return
	}

	aReq.New = updated
	publishChatConfigEvent(api.Logger, api.Pubsub, pubsub.ChatConfigEventModelConfig, updated.ID)
	rw.WriteHeader(http.StatusNoContent)
}

func reducedUserFromPaginatedOrganizationMember(member database.PaginatedOrganizationMembersRow) codersdk.ReducedUser {
	return codersdk.ReducedUser{
		MinimalUser: codersdk.MinimalUser{
			ID:        member.OrganizationMember.UserID,
			Username:  member.Username,
			Name:      member.Name,
			AvatarURL: member.AvatarURL,
		},
		Email:            member.Email,
		CreatedAt:        member.UserCreatedAt,
		UpdatedAt:        member.UserUpdatedAt,
		LastSeenAt:       member.LastSeenAt,
		Status:           codersdk.UserStatus(member.Status),
		LoginType:        codersdk.LoginType(member.LoginType),
		IsServiceAccount: member.IsServiceAccount,
	}
}

type ChatModelACLUpdateValidator codersdk.UpdateChatModelACLRequest

var _ acl.UpdateValidator[codersdk.ChatRole] = ChatModelACLUpdateValidator{}

func (c ChatModelACLUpdateValidator) Users() (map[string]codersdk.ChatRole, string) {
	return c.UserRoles, "user_roles"
}

func (c ChatModelACLUpdateValidator) Groups() (map[string]codersdk.ChatRole, string) {
	return c.GroupRoles, "group_roles"
}

func (ChatModelACLUpdateValidator) ValidateRole(role codersdk.ChatRole) error {
	if role == codersdk.ChatRoleRead || role == codersdk.ChatRoleDeleted {
		return nil
	}
	return xerrors.Errorf("role %q is not a valid chat model role", role)
}
