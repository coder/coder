package scim

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/elimity-com/scim"
	scimErrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

var _ scim.ResourceHandler = (*ResourceUser)(nil)

// scimAudit emits an audit log for a SCIM operation. This uses
// BackgroundAudit instead of InitRequest because the elimity-com/scim
// library owns the http.ResponseWriter and does not expose it to
// resource handlers.
func (ru *ResourceUser) scimAudit(ctx context.Context, r *http.Request, action database.AuditAction, old, changed database.User) {
	raw, _ := json.Marshal(map[string]string{
		"automatic_actor":     "coder",
		"automatic_subsystem": "scim",
	})
	auditor := *ru.opts.Auditor.Load()

	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}

	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.User]{
		Audit:            auditor,
		Log:              ru.opts.Logger,
		UserID:           uuid.Nil, // SCIM provisioner, not a real user
		Action:           action,
		Old:              old,
		New:              changed,
		IP:               ip,
		UserAgent:        r.UserAgent(),
		AdditionalFields: raw,
		Status:           http.StatusOK,
	})
}

type ResourceUser struct {
	store database.Store
	opts  *Options
}

// Create implements scim.ResourceHandler. Creates a new Coder user from
// SCIM attributes, or returns the existing user if a duplicate is found.
func (ru *ResourceUser) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	ctx := r.Context()

	// Extract fields from the SCIM attributes.
	// Do our best to match what the OIDC signup flow also does.
	username, _ := attributes["userName"].(string)
	email := primaryEmail(attributes)
	if email == "" {
		// email is required
		return scim.Resource{}, scimErrors.ScimErrorBadRequest("no primary email provided")
	}

	// This comes from userOIDC
	// TODO: Ideally this code would be shared between the two places.
	usernameValidErr := codersdk.NameValid(username)
	if usernameValidErr != nil {
		if username == "" {
			username = email
		}
		username = codersdk.UsernameFrom(username)
	}

	// TODO: OIDC has optional configuration like `EmailDomain` to reject emails outside a specific domain.
	//   We should consider whether we want to support that for SCIM as well, and if so, apply that validation here.

	active := true
	if a, ok := attributes["active"]; ok {
		v, err := BooleanValue(a)
		if err != nil {
			return scim.Resource{}, scimErrors.ScimErrorBadRequest(
				fmt.Sprintf("invalid boolean value for 'active' field: %v", a))
		}
		active = v
	}

	// Check for existing user by email or username.
	dbUser, err := ru.store.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Email:    email,
		Username: username,
	})
	if err == nil {
		// User already exists. Update their status if needed.
		status := scimUserStatus(dbUser, &active)
		if active && dbUser.Status != status {
			newUser, err := ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
				ID:         dbUser.ID,
				Status:     status,
				UpdatedAt:  dbtime.Now(),
				UserIsSeen: false,
			})
			if err != nil {
				return scim.Resource{}, err
			}
			ru.scimAudit(ctx, r, database.AuditActionWrite, dbUser, newUser)
		}

		return userResource(dbUser), nil
	}

	if !xerrors.Is(err, sql.ErrNoRows) {
		// Internal DB errors should be returned.
		// ErrNoRows is expected if the user does not exist.
		return scim.Resource{}, err
	}

	// OIDC login runs org, group, and role sync. SCIM does not have (or not yet) these
	// claims. We only need to sync the default organization if that is enabled.
	//
	// When the user eventually logs in via OIDC, the regular sync will run.
	// However, since org sync can be disabled. We need to assign the default org if
	// that is how we are configured.
	organizations := []uuid.UUID{}
	orgSync, err := ru.opts.IDPSync.OrganizationSyncSettings(ctx, ru.store)
	if err != nil {
		return scim.Resource{}, xerrors.Errorf("get organization sync settings: %w", err)
	}
	if orgSync.AssignDefault {
		// Technically, we could just always assign this. When they eventually log in,
		// the org would be removed if necessary. But to avoid confusion of the user
		// being in the org before they log in, we apply some intelligence to this guess
		// of "Do they belong in the default org".
		defaultOrganization, err := ru.store.GetDefaultOrganization(ctx)
		if err != nil {
			return scim.Resource{}, xerrors.Errorf("get default organization: %w", err)
		}
		organizations = append(organizations, defaultOrganization.ID)
	}

	// CreateUser does InsertOrganizationMember internally, which needs
	// broader permissions than the SCIM provisioner role. Use
	// AsSystemRestricted for this specific call.
	dbUser, err = ru.opts.AGPL.CreateUser(ctx, ru.store, agpl.CreateUserRequest{
		CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
			Username:        username,
			Email:           email,
			OrganizationIDs: organizations,
		},
		LoginType: database.LoginTypeOIDC,
		// Do not send notifications to user admins; SCIM may call this
		// sequentially for many users.
		// TODO: Maybe we should spam them anyway?
		SkipNotifications: true,
	})
	if err != nil {
		return scim.Resource{}, xerrors.Errorf("create user: %w", err)
	}

	ru.scimAudit(ctx, r, database.AuditActionCreate, database.User{}, dbUser)
	return userResource(dbUser), nil
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

	// All of our fields except for active are immutable.
	if !AttributeEqual(dbUser.Username, attributes, "userName") {
		return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("changing the 'userName' field is not supported (current value: %q)", dbUser.Username))
	}

	// TODO: Check primary email
	activeStr, ok := attributes["active"]
	if !ok {
		return scim.Resource{}, scimErrors.ScimErrorBadRequest("missing required 'active' field")
	}

	active, err := BooleanValue(activeStr)
	if err != nil {
		return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("invalid boolean value for 'active' field: %v", activeStr))
	}

	newStatus := scimUserStatus(dbUser, &active)
	if dbUser.Status != newStatus {
		oldUser := dbUser
		dbUser, err = ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     newStatus,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			return scim.Resource{}, err
		}
		ru.scimAudit(ctx, r, database.AuditActionWrite, oldUser, dbUser)
	}

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
		newUser, err := ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     database.UserStatusSuspended,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			return err
		}
		ru.scimAudit(ctx, r, database.AuditActionWrite, dbUser, newUser)
	}

	return nil
}

// Patch implements scim.ResourceHandler. Updates user attributes based on
// SCIM PatchOp operations. Currently, supports changing the active status.
func (ru *ResourceUser) Patch(r *http.Request, idStr string, operations []scim.PatchOperation) (scim.Resource, error) {
	ctx := r.Context()

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

	// Process operations. Currently, we only handle the "active" attribute.
	var activeSet *bool
	for _, op := range operations {
		switch op.Op {
		case "add":
		case "remove":
			if op.Path.String() == "active" {
				activeSet = ptr.Ref(false)
			}
		case "replace":
			if m, ok := op.Value.(map[string]interface{}); ok {
				// TODO: Should we log other unsupported operations or silently ignore them? For now, we ignore them.
				if actV, ok := m["active"]; ok {
					v, err := BooleanValue(actV)
					if err != nil {
						return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("invalid boolean value for 'active' field: %v", actV))
					}
					activeSet = &v
				}
			}
		default:
		}
	}

	newStatus := scimUserStatus(dbUser, activeSet)
	if dbUser.Status != newStatus {
		oldUser := dbUser
		dbUser, err = ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     newStatus,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			return scim.Resource{}, err
		}
		ru.scimAudit(ctx, r, database.AuditActionWrite, oldUser, dbUser)
	}

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
				"formatted": u.Name,
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
				"formatted": u.Name,
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

// BadUUID returns a 404 not-found error for non-UUID identifiers.
// SCIM clients may send arbitrary strings as IDs; returning 404
// (rather than 400) signals that no resource matches.
func BadUUID(idStr string, _ error) scimErrors.ScimError {
	return scimErrors.ScimErrorResourceNotFound(idStr)
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

// primaryEmail extracts the primary email from SCIM resource attributes.
func primaryEmail(attributes scim.ResourceAttributes) string {
	emailsRaw, ok := attributes["emails"]
	if !ok {
		return ""
	}

	emails, ok := emailsRaw.([]interface{})
	if !ok {
		return ""
	}

	for _, e := range emails {
		emailMap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if primary, _ := emailMap["primary"].(bool); primary {
			if val, ok := emailMap["value"].(string); ok {
				return val
			}
		}
	}

	// Fallback: if no email is marked primary, use the first one.
	if len(emails) > 0 {
		if emailMap, ok := emails[0].(map[string]interface{}); ok {
			if val, ok := emailMap["value"].(string); ok {
				return val
			}
		}
	}

	return ""
}
