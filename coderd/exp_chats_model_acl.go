package coderd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac/acl"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Get an AI model ACL
// @ID get-ai-model-acl
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Success 200 {object} codersdk.ChatModelACL
// @Router /api/experimental/organizations/{organization}/chats/models/{model}/acl [get]
// @x-apidocgen {"skip": true}
func (api *API) chatModelConfigACLHandler(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := httpmw.ChatModelConfigParam(r)
	if !api.Authorize(r, policy.ActionShare, chatModelConfigRBACObject(config)) {
		httpapi.ResourceNotFound(rw)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, chatModelConfigACL(config))
}

type chatModelACLValidationError struct {
	Validations []codersdk.ValidationError
}

func (*chatModelACLValidationError) Error() string {
	return "invalid chat model ACL"
}

// EXPERIMENTAL: this endpoint is experimental and is subject to change.
//
// @Summary Update an AI model ACL
// @ID update-ai-model-acl
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Param organization path string true "Organization name or ID"
// @Param model path string true "Model ID" format(uuid)
// @Param request body codersdk.UpdateChatModelACLRequest true "Sparse model ACL update"
// @Success 204
// @Router /api/experimental/organizations/{organization}/chats/models/{model}/acl [patch]
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

	userRoles, validations := canonicalChatModelACLRoles("user_roles", req.UserRoles)
	groupRoles, duplicateErrors := canonicalChatModelACLRoles("group_roles", req.GroupRoles)
	validations = append(validations, duplicateErrors...)
	if len(validations) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Invalid request to update chat model ACL.",
			Validations: validations,
		})
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

		validations := acl.Validate(ctx, tx, ChatModelACLUpdateValidator(req))
		validations = append(validations, validateChatModelACLOrganization(
			ctx,
			tx,
			config.OrganizationID,
			userRoles,
			groupRoles,
		)...)
		if len(validations) > 0 {
			return &chatModelACLValidationError{Validations: validations}
		}

		userACL := maps.Clone(current.UserACL)
		if userACL == nil {
			userACL = database.ChatACL{}
		}
		groupACL := maps.Clone(current.GroupACL)
		if groupACL == nil {
			groupACL = database.ChatACL{}
		}
		applyChatModelACLRoles(userACL, userRoles)
		applyChatModelACLRoles(groupACL, groupRoles)

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

func chatModelConfigACL(config database.ChatModelConfig) codersdk.ChatModelACL {
	return codersdk.ChatModelACL{
		UserRoles:  chatModelACLRoles(config.UserACL),
		GroupRoles: chatModelACLRoles(config.GroupACL),
	}
}

func chatModelACLRoles(entries database.ChatACL) map[string]codersdk.ChatRole {
	roles := make(map[string]codersdk.ChatRole, len(entries))
	for id, entry := range entries {
		roles[id] = convertToChatRole(entry.Permissions)
	}
	return roles
}

func applyChatModelACLRoles(entries database.ChatACL, roles map[string]codersdk.ChatRole) {
	for id, role := range roles {
		if role == codersdk.ChatRoleDeleted {
			delete(entries, id)
			continue
		}
		entries[id] = database.ChatACLEntry{Permissions: db2sdk.ChatRoleActions(role)}
	}
}

func canonicalChatModelACLRoles(field string, roles map[string]codersdk.ChatRole) (map[string]codersdk.ChatRole, []codersdk.ValidationError) {
	canonical := make(map[string]codersdk.ChatRole, len(roles))
	var validations []codersdk.ValidationError
	for rawID, role := range roles {
		parsed, err := uuid.Parse(rawID)
		if err != nil {
			continue
		}
		id := parsed.String()
		if _, ok := canonical[id]; ok {
			validations = append(validations, codersdk.ValidationError{
				Field:  field,
				Detail: fmt.Sprintf("duplicate entries for ID %s", id),
			})
			continue
		}
		canonical[id] = role
	}
	return canonical, validations
}

func validateChatModelACLOrganization(
	ctx context.Context,
	db database.Store,
	organizationID uuid.UUID,
	userRoles map[string]codersdk.ChatRole,
	groupRoles map[string]codersdk.ChatRole,
) []codersdk.ValidationError {
	var validations []codersdk.ValidationError
	userIDs := chatModelACLRoleIDs(userRoles)
	if len(userIDs) > 0 {
		//nolint:gocritic // Principal validation requires organization membership visibility.
		memberships, err := db.GetOrganizationIDsByMemberIDs(dbauthz.AsSystemRestricted(ctx), userIDs)
		if err != nil {
			validations = append(validations, codersdk.ValidationError{Field: "user_roles", Detail: err.Error()})
		} else {
			byUser := make(map[uuid.UUID][]uuid.UUID, len(memberships))
			for _, membership := range memberships {
				byUser[membership.UserID] = membership.OrganizationIDs
			}
			for _, id := range userIDs {
				if !slices.Contains(byUser[id], organizationID) {
					validations = append(validations, codersdk.ValidationError{
						Field:  "user_roles",
						Detail: "user " + id.String() + " does not belong to organization " + organizationID.String(),
					})
				}
			}
		}
	}

	groupIDs := chatModelACLRoleIDs(groupRoles)
	if len(groupIDs) > 0 {
		//nolint:gocritic // Principal validation requires group organization visibility.
		groups, err := db.GetGroups(dbauthz.AsSystemRestricted(ctx), database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil {
			validations = append(validations, codersdk.ValidationError{Field: "group_roles", Detail: err.Error()})
		} else {
			for _, group := range groups {
				if group.Group.OrganizationID != organizationID {
					validations = append(validations, codersdk.ValidationError{
						Field:  "group_roles",
						Detail: "group " + group.Group.ID.String() + " does not belong to organization " + organizationID.String(),
					})
				}
			}
		}
	}
	return validations
}

func chatModelACLRoleIDs(roles map[string]codersdk.ChatRole) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(roles))
	for rawID, role := range roles {
		if role == codersdk.ChatRoleDeleted {
			continue
		}
		if id, err := uuid.Parse(rawID); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
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
