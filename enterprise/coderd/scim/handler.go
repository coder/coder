package scim

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/google/uuid"
	"github.com/scim2/filter-parser/v2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

// auditAdditionalFields is the AdditionalFields blob attached to every
// audit log entry produced by SCIM. The fields land in the
// `additional_fields` column of `audit_logs` and let the UI label these
// entries as SCIM-originated.
var auditAdditionalFields = map[string]string{
	"automatic_actor":     "coder",
	"automatic_subsystem": "scim",
}

// AuditAdditionalFields exposes the SCIM audit marker map. The
// returned value is a copy so callers cannot mutate the shared
// constant.
func AuditAdditionalFields() map[string]string {
	out := make(map[string]string, len(auditAdditionalFields))
	for k, v := range auditAdditionalFields {
		out[k] = v
	}
	return out
}

// CreateUserFn is the signature of (*coderd.API).CreateUser. Accepting
// it as a function keeps the package decoupled from the enterprise API
// struct and makes the handler trivially mockable in tests.
type CreateUserFn func(ctx context.Context, store database.Store, req agpl.CreateUserRequest) (database.User, error)

// Options bundles the dependencies a SCIM user handler needs. All
// fields are required unless documented otherwise.
type Options struct {
	Database   database.Store
	Logger     slog.Logger
	Auditor    *atomic.Pointer[audit.Auditor]
	IDPSync    idpsync.IDPSync
	CreateUser CreateUserFn

	// APIKey is the shared header token clients send in the
	// Authorization header. If empty, every authenticated request
	// returns 401.
	APIKey []byte
}

// userHandler implements scim.ResourceHandler for the /Users resource.
//
// All operations run as the system-restricted role via dbauthz so SCIM
// can mutate users without an authenticated request context. We treat
// the SCIM endpoint as a privileged out-of-band channel authenticated
// purely by the shared bearer token.
type userHandler struct {
	opts Options
}

var _ scim.ResourceHandler = (*userHandler)(nil)

// Create handles POST /Users.
//
// To preserve compatibility with Okta cloud, when the request targets a
// userName or primary email that matches an existing Coder user we
// return that user instead of 409 Conflict. The framework will respond
// with 201 Created in both cases. This mirrors the pre-refactor
// behavior described in scim.go's `scimPostUser` comments.
//
//nolint:gocritic // SCIM operations run as the system user.
func (h *userHandler) Create(r *http.Request, attrs scim.ResourceAttributes) (scim.Resource, error) {
	ctx := r.Context()

	userName, _ := attrs["userName"].(string)
	active, hasActive := boolAttr(attrs, "active")
	externalID, _ := attrs["externalId"].(string)
	email := primaryEmail(attrs)

	if !hasActive {
		return scim.Resource{}, badRequest(scimerrors.ScimTypeInvalidValue, "active field is required")
	}
	if email == "" {
		return scim.Resource{}, badRequest(scimerrors.ScimType("invalidEmail"), "no primary email provided")
	}

	dbUser, err := h.opts.Database.GetUserByEmailOrUsername(
		dbauthz.AsSystemRestricted(ctx),
		database.GetUserByEmailOrUsernameParams{
			Email:    email,
			Username: userName,
		},
	)
	switch {
	case err == nil:
		// User already exists. Preserve the Okta-cloud quirk: do not
		// 409, return the existing user. If the existing user is
		// suspended and the IdP is pushing an active=true record,
		// transition them to dormant so they can self-activate on
		// next login. This matches the previous handler.
		oldUser := dbUser
		if active && dbUser.Status == database.UserStatusSuspended {
			newUser, updateErr := h.opts.Database.UpdateUserStatus(
				dbauthz.AsSystemRestricted(ctx),
				database.UpdateUserStatusParams{
					ID:         dbUser.ID,
					Status:     database.UserStatusDormant,
					UpdatedAt:  dbtime.Now(),
					UserIsSeen: false,
				},
			)
			if updateErr != nil {
				return scim.Resource{}, internalError(updateErr)
			}
			dbUser = newUser
			h.audit(ctx, r, http.StatusCreated, database.AuditActionWrite, oldUser, dbUser)
		}
		return userResource(dbUser, externalID), nil

	case errors.Is(err, sql.ErrNoRows):
		// Fall through to create the user.

	default:
		return scim.Resource{}, internalError(err)
	}

	// Username must be valid. Fall back to deriving one from the email
	// when the IdP provided an invalid or empty value. This preserves
	// the previous handler's defensive behavior.
	if codersdk.NameValid(userName) != nil {
		if userName == "" {
			userName = email
		}
		userName = codersdk.UsernameFrom(userName)
	}

	organizations := []uuid.UUID{}
	orgSync, err := h.opts.IDPSync.OrganizationSyncSettings(
		dbauthz.AsSystemRestricted(ctx),
		h.opts.Database,
	)
	if err != nil {
		return scim.Resource{}, internalError(xerrors.Errorf("failed to get organization sync settings: %w", err))
	}
	if orgSync.AssignDefault {
		defaultOrg, err := h.opts.Database.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
		if err != nil {
			return scim.Resource{}, internalError(xerrors.Errorf("failed to get default organization: %w", err))
		}
		organizations = append(organizations, defaultOrg.ID)
	}

	dbUser, err = h.opts.CreateUser(
		dbauthz.AsSystemRestricted(ctx),
		h.opts.Database,
		agpl.CreateUserRequest{
			CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
				Username:        userName,
				Email:           email,
				OrganizationIDs: organizations,
			},
			LoginType: database.LoginTypeOIDC,
			// Skip notifications since SCIM is typically called in
			// bulk during initial provisioning.
			SkipNotifications: true,
		},
	)
	if err != nil {
		return scim.Resource{}, internalError(xerrors.Errorf("failed to create user: %w", err))
	}

	h.audit(ctx, r, http.StatusCreated, database.AuditActionCreate, database.User{}, dbUser)
	return userResource(dbUser, externalID), nil
}

// Get handles GET /Users/{id}.
//
// The legacy implementation always returned 404 to force Okta down the
// POST-and-deduplicate path. Returning the real user here is spec
// correct and harmless: POST still preserves the no-409 Okta quirk.
func (h *userHandler) Get(r *http.Request, id string) (scim.Resource, error) {
	ctx := r.Context()

	uid, err := uuid.Parse(id)
	if err != nil {
		return scim.Resource{}, notFound(id)
	}

	//nolint:gocritic // SCIM operations run as the system user.
	dbUser, err := h.opts.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), uid)
	if errors.Is(err, sql.ErrNoRows) {
		return scim.Resource{}, notFound(id)
	}
	if err != nil {
		return scim.Resource{}, internalError(err)
	}
	return userResource(dbUser, ""), nil
}

// GetAll handles GET /Users.
//
// We respect StartIndex (1-based) and Count from the request and never
// honor filter queries because /ServiceProviderConfig advertises
// filter.supported=false.
func (h *userHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	ctx := r.Context()

	// SCIM uses 1-based indexing; the DB query uses 0-based offset.
	offset := params.StartIndex - 1
	if offset < 0 {
		offset = 0
	}
	limit := params.Count
	if limit < 0 {
		limit = 0
	}

	// #nosec G115 - StartIndex and Count are bounded by the SCIM
	// server's MaxResults config; safe to convert to int32.
	//nolint:gocritic // SCIM operations run as the system user.
	rows, err := h.opts.Database.GetUsers(dbauthz.AsSystemRestricted(ctx), database.GetUsersParams{
		OffsetOpt: int32(offset),
		LimitOpt:  int32(limit),
	})
	if err != nil {
		return scim.Page{}, internalError(err)
	}

	page := scim.Page{
		Resources: make([]scim.Resource, 0, len(rows)),
	}
	for _, row := range rows {
		page.TotalResults = int(row.Count)
		page.Resources = append(page.Resources, userResource(getUsersRowToDBUser(row), ""))
	}
	return page, nil
}

// Replace handles PUT /Users/{id}.
//
// Per RFC 7644 Section 3.5.1, PUT replaces the resource. We currently
// honor only the `active` attribute and reject changes to userName as
// immutable; everything else is silently ignored. This matches the
// legacy handler. A future PR will widen the writable set.
//
//nolint:gocritic // SCIM operations run as the system user.
func (h *userHandler) Replace(r *http.Request, id string, attrs scim.ResourceAttributes) (scim.Resource, error) {
	ctx := r.Context()

	active, hasActive := boolAttr(attrs, "active")
	if !hasActive {
		return scim.Resource{}, badRequest(scimerrors.ScimTypeInvalidValue, "active field is required")
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		return scim.Resource{}, badRequest(scimerrors.ScimTypeInvalidValue, "id must be a uuid")
	}

	dbUser, err := h.opts.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), uid)
	if errors.Is(err, sql.ErrNoRows) {
		return scim.Resource{}, notFound(id)
	}
	if err != nil {
		return scim.Resource{}, internalError(err)
	}

	if userName, _ := attrs["userName"].(string); userName != "" && userName != dbUser.Username {
		return scim.Resource{}, scimerrors.ScimErrorMutability
	}

	externalID, _ := attrs["externalId"].(string)
	return h.applyActive(ctx, r, dbUser, active, externalID)
}

// Patch handles PATCH /Users/{id} per RFC 7644 Section 3.5.2.
//
// We accept `replace` operations targeting the `active` attribute. All
// other ops or paths return the spec-defined error. The previous
// handler decoded a full SCIMUser payload (effectively PUT semantics on
// the PATCH verb), which was a known 2.0 noncompliance.
//
//nolint:gocritic // SCIM operations run as the system user.
func (h *userHandler) Patch(r *http.Request, id string, ops []scim.PatchOperation) (scim.Resource, error) {
	ctx := r.Context()

	uid, err := uuid.Parse(id)
	if err != nil {
		return scim.Resource{}, badRequest(scimerrors.ScimTypeInvalidValue, "id must be a uuid")
	}

	dbUser, err := h.opts.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), uid)
	if errors.Is(err, sql.ErrNoRows) {
		return scim.Resource{}, notFound(id)
	}
	if err != nil {
		return scim.Resource{}, internalError(err)
	}

	var (
		nextActive = dbUser.Status != database.UserStatusSuspended
		sawActive  bool
		externalID string
	)
	for _, op := range ops {
		path := strings.ToLower(strings.TrimSpace(patchPathString(op.Path)))
		switch {
		case strings.EqualFold(op.Op, scim.PatchOperationReplace) && path == "active":
			b, ok := op.Value.(bool)
			if !ok {
				return scim.Resource{}, scimerrors.ScimErrorInvalidValue
			}
			nextActive = b
			sawActive = true

		case strings.EqualFold(op.Op, scim.PatchOperationReplace) && path == "externalid":
			s, ok := op.Value.(string)
			if !ok {
				return scim.Resource{}, scimerrors.ScimErrorInvalidValue
			}
			externalID = s

		case strings.EqualFold(op.Op, scim.PatchOperationReplace) && path == "":
			// Replace with no path: the value is a map of attributes
			// to overwrite. Only honor `active` for now.
			m, ok := op.Value.(map[string]interface{})
			if !ok {
				return scim.Resource{}, scimerrors.ScimErrorInvalidValue
			}
			if b, present := boolAttr(m, "active"); present {
				nextActive = b
				sawActive = true
			}
			if s, _ := m["externalId"].(string); s != "" {
				externalID = s
			}

		default:
			return scim.Resource{}, scimerrors.ScimErrorMutability
		}
	}

	if !sawActive {
		// No-op patch (or only externalId): nothing to persist yet
		// because externalId isn't stored in this PR.
		return userResource(dbUser, externalID), nil
	}

	return h.applyActive(ctx, r, dbUser, nextActive, externalID)
}

// Delete handles DELETE /Users/{id}. Not yet implemented; SCIM clients
// typically deprovision via PATCH active=false, so a 501 is acceptable
// for the first compliance pass.
func (*userHandler) Delete(_ *http.Request, _ string) error {
	return scimerrors.ScimError{
		Status: http.StatusNotImplemented,
		Detail: "DELETE is not implemented; use PATCH active=false to deprovision users",
	}
}

// applyActive transitions a user to the appropriate status given the
// IdP's desired active flag, emits an audit log, and returns the
// resource for the response.
//
//nolint:gocritic // SCIM operations run as the system user.
func (h *userHandler) applyActive(ctx context.Context, r *http.Request, dbUser database.User, active bool, externalID string) (scim.Resource, error) {
	newStatus := scimUserStatus(dbUser, active)
	if dbUser.Status == newStatus {
		return userResource(dbUser, externalID), nil
	}
	oldUser := dbUser
	updated, err := h.opts.Database.UpdateUserStatus(
		dbauthz.AsSystemRestricted(ctx),
		database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     newStatus,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		},
	)
	if err != nil {
		return scim.Resource{}, internalError(err)
	}
	h.audit(ctx, r, http.StatusOK, database.AuditActionWrite, oldUser, updated)
	return userResource(updated, externalID), nil
}

// audit emits a single audit log entry. We use BackgroundAudit rather
// than InitRequest because elimity's ResourceHandler interface does not
// expose the response writer, which InitRequest requires.
func (h *userHandler) audit(ctx context.Context, r *http.Request, status int, action database.AuditAction, oldUser, newUser database.User) {
	auditor := h.opts.Auditor.Load()
	if auditor == nil {
		return
	}
	rawFields, _ := json.Marshal(auditAdditionalFields)

	userID := newUser.ID
	if userID == uuid.Nil {
		userID = oldUser.ID
	}

	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.User]{
		Audit:            *auditor,
		Log:              h.opts.Logger,
		UserID:           userID,
		RequestID:        httpmw.RequestID(r),
		Status:           status,
		Action:           action,
		IP:               r.RemoteAddr,
		UserAgent:        r.UserAgent(),
		AdditionalFields: rawFields,
		Old:              oldUser,
		New:              newUser,
	})
}

// scimUserStatus computes the next Coder user status given the IdP's
// `active` flag. Active users stay active, suspended/dormant users
// transition to dormant so they self-activate on next login. This
// matches the legacy behavior.
//
//nolint:revive // active is not a control flag.
func scimUserStatus(user database.User, active bool) database.UserStatus {
	if !active {
		return database.UserStatusSuspended
	}
	switch user.Status {
	case database.UserStatusActive:
		return database.UserStatusActive
	case database.UserStatusDormant, database.UserStatusSuspended:
		return database.UserStatusDormant
	default:
		return database.UserStatusDormant
	}
}

// userResource translates a Coder database user into a SCIM resource
// shape consumable by the elimity framework. The framework will add
// `meta.location`, `schemas`, and (if non-empty) `externalId` when
// rendering the response, so we only populate the user-defined
// attributes here.
func userResource(u database.User, externalID string) scim.Resource {
	created := u.CreatedAt
	modified := u.UpdatedAt

	res := scim.Resource{
		ID: u.ID.String(),
		Attributes: scim.ResourceAttributes{
			"userName": u.Username,
			"active":   u.Status != database.UserStatusSuspended,
			"emails": []map[string]interface{}{
				{
					"primary": true,
					"value":   u.Email,
					"type":    "work",
				},
			},
			"name": map[string]interface{}{
				"formatted": u.Name,
			},
		},
		Meta: scim.Meta{
			Created:      &created,
			LastModified: &modified,
		},
	}
	if externalID != "" {
		res.ExternalID = optional.NewString(externalID)
	}
	return res
}

// getUsersRowToDBUser narrows a GetUsersRow to the database.User fields
// SCIM cares about. We intentionally do not roundtrip every column;
// only the user-visible subset.
func getUsersRowToDBUser(row database.GetUsersRow) database.User {
	return database.User{
		ID:        row.ID,
		Email:     row.Email,
		Username:  row.Username,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		Status:    row.Status,
		LoginType: row.LoginType,
		AvatarURL: row.AvatarURL,
		Deleted:   row.Deleted,
		Name:      row.Name,
	}
}

// boolAttr returns a bool attribute and whether it was present. SCIM
// validation guarantees the value is `bool` when present, so the second
// return value distinguishes "unset" from "false".
func boolAttr(m scim.ResourceAttributes, key string) (value, present bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// primaryEmail extracts the value of the first email entry with
// primary=true from a validated SCIM attribute map.
func primaryEmail(attrs scim.ResourceAttributes) string {
	emails, ok := attrs["emails"].([]interface{})
	if !ok {
		return ""
	}
	for _, e := range emails {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		primary, _ := em["primary"].(bool)
		value, _ := em["value"].(string)
		if primary && value != "" {
			return value
		}
	}
	return ""
}

// patchPathString flattens a *filter.Path into a dotted attribute
// reference like "active" or "name.givenName". Returns "" when no path
// is present (i.e. the PATCH operation has no `path` field).
func patchPathString(path *filter.Path) string {
	if path == nil {
		return ""
	}
	return path.String()
}

// badRequest builds a SCIM error with status 400 and the given
// scimType/detail. We use scimType strings rather than the package's
// predefined errors when our message is more specific.
func badRequest(scimType scimerrors.ScimType, detail string) error {
	return scimerrors.ScimError{
		ScimType: scimType,
		Detail:   detail,
		Status:   http.StatusBadRequest,
	}
}

// notFound builds a SCIM 404 error for the given resource id. The
// elimity package exports a function rather than a sentinel value, so
// we wrap it here for consistency with the other helpers.
func notFound(id string) error {
	return scimerrors.ScimErrorResourceNotFound(id)
}

// internalError wraps a Go error in a SCIM 500 response while
// preserving the original error message in `detail`.
func internalError(err error) error {
	return scimerrors.ScimError{
		Status: http.StatusInternalServerError,
		Detail: err.Error(),
	}
}
