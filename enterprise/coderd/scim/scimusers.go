package scim

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/elimity-com/scim"
	scimErrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

var _ scim.ResourceHandler = (*ResourceUser)(nil)

var scimAuditAdditionalFields = map[string]string{
	"automatic_actor":     "coder",
	"automatic_subsystem": "scim",
}

type ResourceUser struct {
	store database.Store
	opts  *Options
}

func (ru *ResourceUser) Create(_ *http.Request, _ scim.ResourceAttributes) (scim.Resource, error) {
	// Creating a new user from SCIM is not currently supported.
	return scim.Resource{}, scimErrors.ScimError{
		Status: http.StatusNotImplemented,
		Detail: "User creation via SCIM is not supported",
	}
}

// Get implements scim.ResourceHandler. Returns a single user by ID.
func (ru *ResourceUser) Get(r *http.Request, idStr string) (scim.Resource, error) {
	ctx := r.Context()
	id, err := uuid.Parse(idStr)
	if err != nil {
		return scim.Resource{}, BadUUID(idStr, err)
	}

	usr, err := ru.store.GetUserByID(ctx, id)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return scim.Resource{}, scimErrors.ScimErrorResourceNotFound(idStr)
		}
		return scim.Resource{}, err
	}

	return userResource(usr), nil
}

// GetAll implements scim.ResourceHandler. Returns a paginated list of users.
func (ru *ResourceUser) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	ctx := r.Context()

	users, err := ru.store.GetUsers(ctx, database.GetUsersParams{
		OffsetOpt: int32(params.StartIndex - 1), //nolint:gosec
		LimitOpt:  int32(params.Count),          //nolint:gosec
	})
	if err != nil {
		return scim.Page{}, err
	}

	totalCount, err := ru.store.GetUserCount(ctx, false)
	if err != nil {
		return scim.Page{}, err
	}

	resources := make([]scim.Resource, 0, len(users))
	for _, u := range users {
		resources = append(resources, userResourceFromGetUsersRow(u))
	}

	return scim.Page{
		TotalResults: int(totalCount),
		Resources:    resources,
	}, nil
}

// Replace implements scim.ResourceHandler (PUT). Replaces user attributes.
// Currently only supports changing the active status per existing behavior.
func (ru *ResourceUser) Replace(r *http.Request, idStr string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	ctx := r.Context()

	auditor := *ru.opts.Auditor.Load()
	aReq, commitAudit := audit.InitRequestWithCancel[database.User](nil, &audit.RequestParams{
		Audit:            auditor,
		Log:              ru.opts.Logger,
		Request:          r,
		Action:           database.AuditActionWrite,
		AdditionalFields: scimAuditAdditionalFields,
	})
	defer commitAudit(true)

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return scim.Resource{}, BadUUID(idStr, err)
	}

	dbUser, err := ru.store.GetUserByID(ctx, uid)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return scim.Resource{}, scimErrors.ScimErrorResourceNotFound(idStr)
		}
		return scim.Resource{}, err
	}
	aReq.Old = dbUser
	aReq.UserID = dbUser.ID

	// All of our fields except for active are immutable.
	if !AttributeEqual(dbUser.Username, attributes, "userName") {
		return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("changing the 'userName' field is not supported (current value: %q)", dbUser.Username))
	}

	// TODO: Check primary email

	active := true
	if activeStr, ok := attributes["active"]; ok {
		active, err = BooleanValue(activeStr)
		if err != nil {
			return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("invalid boolean value for 'active' field: %v", activeStr))
		}
	}

	newStatus := scimUserStatus(dbUser, &active)
	if dbUser.Status != newStatus {
		dbUser, err = ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     newStatus,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			return scim.Resource{}, err
		}
	} else {
		// No change, skip audit log.
		commitAudit(false)
	}

	aReq.New = dbUser
	return userResource(dbUser), nil
}

// Delete implements scim.ResourceHandler. Suspends the user (Coder does
// not hard-delete users).
func (ru *ResourceUser) Delete(r *http.Request, idStr string) error {
	ctx := r.Context()

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return scimErrors.ScimErrorResourceNotFound(idStr)
	}

	dbUser, err := ru.store.GetUserByID(ctx, uid)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return scimErrors.ScimErrorResourceNotFound(idStr)
		}
		return err
	}

	if dbUser.Status != database.UserStatusSuspended {
		// Audit log the change to suspended status
		auditor := *ru.opts.Auditor.Load()
		aReq, commitAudit := audit.InitRequestWithCancel[database.User](nil, &audit.RequestParams{
			Audit:            auditor,
			Log:              ru.opts.Logger,
			Request:          r,
			Action:           database.AuditActionWrite,
			AdditionalFields: scimAuditAdditionalFields,
		})
		defer commitAudit(true)

		newUser, err := ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     database.UserStatusSuspended,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			return err
		}
		aReq.Old = dbUser
		aReq.New = newUser
		aReq.UserID = dbUser.ID
	}

	return nil
}

// Patch implements scim.ResourceHandler. Updates user attributes based on
// SCIM PatchOp operations. Currently, supports changing the active status.
func (ru *ResourceUser) Patch(r *http.Request, idStr string, operations []scim.PatchOperation) (scim.Resource, error) {
	ctx := r.Context()

	auditor := *ru.opts.Auditor.Load()
	aReq, commitAudit := audit.InitRequestWithCancel[database.User](nil, &audit.RequestParams{
		Audit:            auditor,
		Log:              ru.opts.Logger,
		Request:          r,
		Action:           database.AuditActionWrite,
		AdditionalFields: scimAuditAdditionalFields,
	})
	defer commitAudit(true)

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return scim.Resource{}, BadUUID(idStr, err)
	}

	dbUser, err := ru.store.GetUserByID(ctx, uid)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return scim.Resource{}, scimErrors.ScimErrorResourceNotFound(idStr)
		}
		return scim.Resource{}, err
	}
	aReq.Old = dbUser
	aReq.UserID = dbUser.ID

	// Process operations. Currently, we only handle the "active" attribute.
	var activeSet *bool
	for _, op := range operations {
		path := ""
		if op.Path != nil {
			path = op.Path.String()
		}
		if path == "active" {
			v, err := BooleanValue(op.Value)
			if err != nil {
				return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("invalid boolean value for 'active' field: %v", op.Value))
			}
			activeSet = &v
		}
		// TODO: Should we log other unsupported operations or silently ignore them? For now, we ignore them.
	}

	newStatus := scimUserStatus(dbUser, activeSet)
	if dbUser.Status != newStatus {
		dbUser, err = ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     newStatus,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			return scim.Resource{}, err
		}
	} else {
		// No meaningful change, skip audit log.
		commitAudit(false)
	}

	aReq.New = dbUser
	return userResource(dbUser), nil
}

// scimUserStatus maps the SCIM "active" boolean to Coder's internal user status.
// It preserves the active/dormant distinction: active users stay active,
// dormant or suspended users become dormant when re-activated (they become
// active after their next login).
//
//nolint:revive // active is not a control flag
func scimUserStatus(user database.User, active *bool) database.UserStatus {
	if active == nil {
		return user.Status
	}

	if !(*active) {
		// SCIM "active: false" means the user should be suspended
		return database.UserStatusSuspended
	}

	switch user.Status {
	case database.UserStatusActive:
		// Active users stay active
		return database.UserStatusActive
	case database.UserStatusDormant, database.UserStatusSuspended:
		// Dormant or suspended users become dormant when re-activated
		// The user can then become active by doing something in the product.
		return database.UserStatusDormant
	default:
		return database.UserStatusDormant
	}
}

// userResource converts a database.User into a SCIM Resource.
func userResource(u database.User) scim.Resource {
	return scim.Resource{
		ID:         u.ID.String(),
		ExternalID: optional.String{},
		Attributes: scim.ResourceAttributes{
			"userName": u.Username,
			"name": map[string]interface{}{
				"givenName":  u.Name,
				"familyName": "",
			},
			"emails": []map[string]interface{}{
				{
					"primary": true,
					"value":   u.Email,
				},
			},
			"active": u.Status == database.UserStatusActive ||
				u.Status == database.UserStatusDormant,
		},
		Meta: scim.Meta{
			Created:      &u.CreatedAt,
			LastModified: &u.UpdatedAt,
		},
	}
}

// userResourceFromGetUsersRow converts a database.GetUsersRow into a SCIM Resource.
func userResourceFromGetUsersRow(u database.GetUsersRow) scim.Resource {
	return scim.Resource{
		ID:         u.ID.String(),
		ExternalID: optional.String{},
		Attributes: scim.ResourceAttributes{
			"userName": u.Username,
			"name": map[string]interface{}{
				"givenName":  u.Name,
				"familyName": "",
			},
			"emails": []map[string]interface{}{
				{
					"primary": true,
					"value":   u.Email,
				},
			},
			"active": u.Status == database.UserStatusActive ||
				u.Status == database.UserStatusDormant,
		},
		Meta: scim.Meta{
			Created:      &u.CreatedAt,
			LastModified: &u.UpdatedAt,
		},
	}
}

func BadUUID(idStr string, err error) scimErrors.ScimError {
	return scimErrors.ScimErrorBadRequest(fmt.Sprintf("expected a UUID but got %q: %w", idStr, err))
}

func BooleanValue(v interface{}) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		return strconv.ParseBool(b)
	default:
		return false, fmt.Errorf("expected boolean or string value, got %T", v)
	}
}

func AttributeEqual[T comparable](existing T, attrs scim.ResourceAttributes, key string) bool {
	found, ok := attrs[key]
	if !ok {
		return true // No change if the attribute is not present in the request
	}

	sameType, ok := found.(T)
	if !ok {
		return false // Type mismatch, consider it a change
	}

	return existing == sameType
}
