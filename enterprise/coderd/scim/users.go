package scim

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/elimity-com/scim"
	scimErrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

var _ scim.ResourceHandler = (*ResourceUser)(nil)

// auditUser emits an audit log for a SCIM operation. This uses
// BackgroundAudit instead of InitRequest because the elimity-com/scim
// library owns the http.ResponseWriter and does not expose it to
// resource handlers.
func (ru *ResourceUser) auditUser(ctx context.Context, r *http.Request, action database.AuditAction, old, changed database.User) {
	raw, _ := json.Marshal(map[string]string{
		"automatic_actor":     "coder",
		"automatic_subsystem": "scim",
	})
	auditor := *ru.opts.Auditor.Load()

	// This is a best effort
	// TODO: Check X-Forwarded-For and others for proxied requests
	ip := r.RemoteAddr

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
	username, _ := attributeAsString(attributes, "userName")
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
	if a, ok := attribute(attributes, "active"); ok {
		v, err := booleanValue(a)
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
		// SCIM spec says to return a StatusConflict if the user already exists.
		// However, Coder never deletes a user. So suspended **is** deleted.
		// If the user is not suspended, we return a conflict.
		if dbUser.Status != database.UserStatusSuspended {
			return scim.Resource{}, scimErrors.ScimError{
				ScimType: scimErrors.ScimTypeUniqueness,
				Detail:   fmt.Sprintf("user already exists with email %q or username %q", email, username),
				Status:   http.StatusConflict,
			}
		}

		// If the user is suspended, then they might be deleted on the SCIM side.
		// We can just update their status and return the user as they exist.
		status := scimUserStatus(dbUser, &active)
		dbUser, err = ru.updateUserStatus(ctx, r, dbUser, status)
		if err != nil {
			return scim.Resource{}, err
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

	// CreateUser does InsertOrganizationMember internally, and InsertUser
	// implicitly assigns the member role at site scope. The SCIM provisioner
	// role cannot assign either, so escalate to a system context for this
	// specific call, matching the legacy SCIM handler.
	//nolint:gocritic // SCIM bearer token authenticates as the SCIM provisioner; user creation needs broader rights to assign default roles.
	dbUser, err = ru.opts.AGPL.CreateUser(dbauthz.AsSystemRestricted(ctx), ru.store, agpl.CreateUserRequest{
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

	ru.auditUser(ctx, r, database.AuditActionCreate, database.User{}, dbUser)
	return userResource(dbUser), nil
}

// Get implements scim.ResourceHandler. Returns a single user by ID.
func (ru *ResourceUser) Get(r *http.Request, idStr string) (scim.Resource, error) {
	ctx := r.Context()
	usr, err := ru.user(ctx, idStr)
	if err != nil {
		return scim.Resource{}, err
	}

	return userResource(usr), nil
}

// GetAll implements scim.ResourceHandler. Returns a paginated list of users.
func (ru *ResourceUser) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	ctx := r.Context()

	var qry database.GetUsersParams
	if params.FilterValidator != nil {
		var err error
		qry, err = userQuery(params.FilterValidator.GetFilter())
		if err != nil {
			return scim.Page{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("invalid filter: %v", err))
		}
	}

	qry.LimitOpt = int32(params.Count)           //nolint:gosec
	qry.OffsetOpt = int32(params.StartIndex - 1) //nolint:gosec

	if qry.LimitOpt < 0 {
		qry.LimitOpt = 100
	}

	users, err := ru.store.GetUsers(ctx, qry)
	if err != nil {
		return scim.Page{}, err
	}

	totalCount := int64(len(users))
	if len(users) == int(qry.LimitOpt) {
		// If the limit is not reached, that is the count
		// TODO: If there is a query and the limit is reached, this is inaccurate.
		totalCount, err = ru.store.GetUserCount(ctx, false)
		if err != nil {
			return scim.Page{}, err
		}
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

	dbUser, err := ru.user(ctx, idStr)
	if err != nil {
		return scim.Resource{}, err
	}

	// All of our fields except for active are immutable.
	if !attributeEqual(dbUser.Username, attributes, "userName") {
		return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("changing the 'userName' field is not supported (current value: %q)", dbUser.Username))
	}

	// TODO: Check if the primary email has changed. If it has, should we do something?

	activeInterface, ok := attribute(attributes, "active")
	if !ok {
		return scim.Resource{}, scimErrors.ScimErrorBadRequest("missing required 'active' field")
	}

	active, err := booleanValue(activeInterface)
	if err != nil {
		return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("invalid boolean value for 'active' field: %v", activeInterface))
	}

	newStatus := scimUserStatus(dbUser, &active)
	dbUser, err = ru.updateUserStatus(ctx, r, dbUser, newStatus)
	if err != nil {
		return scim.Resource{}, err
	}

	return userResource(dbUser), nil
}

// Delete implements scim.ResourceHandler. Suspends the user (Coder does
// not hard-delete users).
func (ru *ResourceUser) Delete(r *http.Request, idStr string) error {
	ctx := r.Context()

	dbUser, err := ru.user(ctx, idStr)
	if err != nil {
		return err
	}

	_, err = ru.updateUserStatus(ctx, r, dbUser, database.UserStatusSuspended)
	if err != nil {
		return err
	}

	return nil
}

// Patch implements scim.ResourceHandler. Updates user attributes based on
// SCIM PatchOp operations. Currently, supports changing the active status.
func (ru *ResourceUser) Patch(r *http.Request, idStr string, operations []scim.PatchOperation) (scim.Resource, error) {
	ctx := r.Context()

	uid, err := uuid.Parse(idStr)
	if err != nil {
		return scim.Resource{}, badUUID(idStr, err)
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
			// TODO: Currently we do not support the adding of attributes.
		case "remove":
			// TODO: If the path is unspecified, we should fail with the status code 400.
			//  Today, we only accept the 'active' field and silently drop the rest.
			if op.Path != nil && strings.EqualFold(op.Path.String(), "active") {
				activeSet = ptr.Ref(false)
			}
		case "replace":
			// TODO: Honor mutability rules of fields like `userName` and `email`.
			//  Should scim be able to change those fields?

			// SCIM PATCH replace can come in two forms:
			// 1. Path set: {"op":"replace","path":"active","value":false}
			// 2. No path, value is a map: {"op":"replace","value":{"active":false}}
			if op.Path != nil && strings.EqualFold(op.Path.String(), "active") {
				v, err := booleanValue(op.Value)
				if err != nil {
					return scim.Resource{}, scimErrors.ScimErrorBadRequest(fmt.Sprintf("invalid boolean value for 'active' field: %v", op.Value))
				}
				activeSet = &v
			} else if m, ok := op.Value.(map[string]interface{}); ok {
				if actV, ok := attribute(m, "active"); ok {
					v, err := booleanValue(actV)
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
	dbUser, err = ru.updateUserStatus(ctx, r, dbUser, newStatus)
	if err != nil {
		return scim.Resource{}, err
	}

	return userResource(dbUser), nil
}

func (ru *ResourceUser) user(ctx context.Context, idStr string) (database.User, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return database.User{}, badUUID(idStr, err)
	}

	usr, err := ru.store.GetUserByID(ctx, id)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return database.User{}, scimErrors.ScimErrorResourceNotFound(idStr)
		}
		return database.User{}, err
	}

	return usr, nil
}

// updateUserStatus is a no-op if the status did not change.
func (ru *ResourceUser) updateUserStatus(ctx context.Context, r *http.Request, u database.User, status database.UserStatus) (database.User, error) {
	if u.Status == status {
		return u, nil
	}
	newUser, err := ru.store.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
		ID: u.ID, Status: status, UpdatedAt: dbtime.Now(), UserIsSeen: false,
	})
	if err != nil {
		return database.User{}, err
	}
	ru.auditUser(ctx, r, database.AuditActionWrite, u, newUser)
	return newUser, nil
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

func attributeAsBool(attrs scim.ResourceAttributes, key string) (value bool, exists bool) {
	val, ok := attribute(attrs, key)
	if !ok {
		return false, false
	}

	switch v := val.(type) {
	case string:
		pv, err := strconv.ParseBool(v)
		return pv, err == nil
	case bool:
		return v, true
	default:
		return false, false
	}
}

func attributeAsString(attrs scim.ResourceAttributes, key string) (string, bool) {
	val, ok := attribute(attrs, key)
	if !ok {
		return "", false
	}

	switch v := val.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	default:
		return "", false
	}
}

func attribute(attrs scim.ResourceAttributes, key string) (interface{}, bool) {
	// attribute names are case-insensitive per SCIM spec
	val, ok := attrs[key]
	if ok {
		return val, true
	}

	// This is terrible, but we need to iterate the map to find the key in a case-insensitive way.
	// The scim Spec says attribute names are case-insensitive.
	for k, v := range attrs {
		if k == key {
			return v, true
		}
		if len(k) == len(key) && strings.EqualFold(k, key) {
			return v, true
		}
	}

	return nil, false
}

// badUUID returns a 404 not-found error for non-UUID identifiers.
// SCIM clients may send arbitrary strings as IDs; returning 404
// (rather than 400) signals that no resource matches.
func badUUID(idStr string, _ error) scimErrors.ScimError {
	return scimErrors.ScimError{
		Detail: fmt.Sprintf("%q is not a valid uuid; resource not found", idStr),
		Status: http.StatusNotFound,
	}
}

func booleanValue(v interface{}) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		return strconv.ParseBool(b)
	default:
		return false, xerrors.Errorf("expected boolean or string value, got %T", v)
	}
}

func attributeEqual[T comparable](existing T, attrs scim.ResourceAttributes, key string) bool {
	found, ok := attribute(attrs, key)
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
	emailsRaw, ok := attribute(attributes, "emails")
	if !ok {
		return ""
	}

	emails, ok := emailsRaw.([]interface{})
	if !ok {
		return ""
	}

	var fallback string
	for _, e := range emails {
		emailMap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		val, ok := attributeAsString(emailMap, "value")
		if !ok {
			continue
		}
		if primary, _ := attributeAsBool(emailMap, "primary"); primary {
			return val
		}
		fallback = val
	}

	return fallback
}
