// Package legacyscim preserves the old imulab/go-scim based SCIM handler.
// It was added in May 2026 to keep an opt-out path available during the
// rollout of the new SCIM 2.0 implementation in
// enterprise/coderd/scim. Once that implementation has run in production
// for a while and the CODER_SCIM_USE_LEGACY default is flipped, remove
// this package in its entirety.
//
// Enabled via the UseLegacySCIM option.
//
// Deprecated: Use the enterprise/coderd/scim package instead.
package legacyscim

import (
	"bytes"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/imulab/go-scim/pkg/v2/handlerutil"
	scimjson "github.com/imulab/go-scim/pkg/v2/json"
	"github.com/imulab/go-scim/pkg/v2/service"
	"github.com/imulab/go-scim/pkg/v2/spec"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

// LegacyServer is the old SCIM handler implementation, kept for backward
// compatibility. It uses the imulab/go-scim library and custom JSON handling.
type LegacyServer struct {
	Logger     slog.Logger
	Database   database.Store
	IDPSync    idpsync.IDPSync
	AGPL       *agpl.API
	AccessURL  *url.URL
	SCIMAPIKey []byte
	Auditor    *atomic.Pointer[audit.Auditor]
}

// Handler returns an http.Handler that serves the legacy SCIM endpoints.
// It should be mounted at /scim/v2.
func (s *LegacyServer) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/ServiceProviderConfig", s.scimServiceProviderConfig)
	r.Post("/Users", s.scimPostUser)
	r.Route("/Users", func(r chi.Router) {
		r.Get("/", s.scimGetUsers)
		r.Post("/", s.scimPostUser)
		r.Get("/{id}", s.scimGetUser)
		r.Patch("/{id}", s.scimPatchUser)
		r.Put("/{id}", s.scimPutUser)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.String()
		httpapi.Write(r.Context(), w, http.StatusNotFound, codersdk.Response{
			Message: "SCIM endpoint not found: " + u,
			Detail:  "This endpoint is not implemented. If it is correct and required, please contact support.",
		})
	})
	return r
}

// AuthMiddleware verifies the SCIM Bearer token.
func (s *LegacyServer) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !s.scimVerifyAuthHeader(r) {
			scimUnauthorized(rw)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

func (s *LegacyServer) scimVerifyAuthHeader(r *http.Request) bool {
	bearer := []byte("bearer ")
	hdr := []byte(r.Header.Get("Authorization"))

	// Use toLower to make the comparison case-insensitive.
	if len(hdr) >= len(bearer) && subtle.ConstantTimeCompare(bytes.ToLower(hdr[:len(bearer)]), bearer) == 1 {
		hdr = hdr[len(bearer):]
	}

	return len(s.SCIMAPIKey) != 0 && subtle.ConstantTimeCompare(hdr, s.SCIMAPIKey) == 1
}

func scimUnauthorized(rw http.ResponseWriter) {
	_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusUnauthorized, "invalidAuthorization", xerrors.New("invalid authorization")))
}

// scimServiceProviderConfig returns a static SCIM service provider configuration.
//
// @Summary SCIM 2.0: Service Provider Config
// @ID scim-get-service-provider-config
// @Produce application/scim+json
// @Tags Enterprise
// @Success 200
// @Router /scim/v2/ServiceProviderConfig [get]
func (s *LegacyServer) scimServiceProviderConfig(rw http.ResponseWriter, _ *http.Request) {
	// No auth needed to query this endpoint.

	rw.Header().Set("Content-Type", spec.ApplicationScimJson)
	rw.WriteHeader(http.StatusOK)

	// providerUpdated is the last time the static provider config was updated.
	// Increment this time if you make any changes to the provider config.
	providerUpdated := time.Date(2024, 10, 25, 17, 0, 0, 0, time.UTC)
	var location string
	locURL, err := s.AccessURL.Parse("/scim/v2/ServiceProviderConfig")
	if err == nil {
		location = locURL.String()
	}

	enc := json.NewEncoder(rw)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(ServiceProviderConfig{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		DocURI:  "https://coder.com/docs/admin/users/oidc-auth#scim",
		Patch: Supported{
			Supported: true,
		},
		Bulk: BulkSupported{
			Supported: false,
		},
		Filter: FilterSupported{
			Supported: false,
		},
		ChangePassword: Supported{
			Supported: false,
		},
		Sort: Supported{
			Supported: false,
		},
		ETag: Supported{
			Supported: false,
		},
		AuthSchemes: []AuthenticationScheme{
			{
				Type:        "oauthbearertoken",
				Name:        "HTTP Header Authentication",
				Description: "Authentication scheme using the Authorization header with the shared token",
				DocURI:      "https://coder.com/docs/admin/users/oidc-auth#scim",
			},
		},
		Meta: ServiceProviderMeta{
			Created:      providerUpdated,
			LastModified: providerUpdated,
			Location:     location,
			ResourceType: "ServiceProviderConfig",
		},
	})
}

// scimGetUsers intentionally always returns no users. This is done to always force
// Okta to try and create each user individually, this way we don't need to
// implement fetching users twice.
//
// @Summary SCIM 2.0: Get users
// @ID scim-get-users
// @Security Authorization
// @Produce application/scim+json
// @Tags Enterprise
// @Success 200
// @Router /scim/v2/Users [get]
//
//nolint:revive
func (s *LegacyServer) scimGetUsers(rw http.ResponseWriter, r *http.Request) {
	if !s.scimVerifyAuthHeader(r) {
		scimUnauthorized(rw)
		return
	}

	_ = handlerutil.WriteSearchResultToResponse(rw, &service.QueryResponse{
		TotalResults: 0,
		StartIndex:   1,
		ItemsPerPage: 0,
		Resources:    []scimjson.Serializable{},
	})
}

// scimGetUser intentionally always returns an error saying the user wasn't found.
// This is done to always force Okta to try and create the user, this way we
// don't need to implement fetching users twice.
//
// @Summary SCIM 2.0: Get user by ID
// @ID scim-get-user-by-id
// @Security Authorization
// @Produce application/scim+json
// @Tags Enterprise
// @Param id path string true "User ID" format(uuid)
// @Failure 404
// @Router /scim/v2/Users/{id} [get]
//
//nolint:revive
func (s *LegacyServer) scimGetUser(rw http.ResponseWriter, r *http.Request) {
	if !s.scimVerifyAuthHeader(r) {
		scimUnauthorized(rw)
		return
	}

	_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusNotFound, spec.ErrNotFound.Type, xerrors.New("endpoint will always return 404")))
}

// We currently use our own struct instead of using the SCIM package. This was
// done mostly because the SCIM package was almost impossible to use. We only
// need these fields, so it was much simpler to use our own struct. This was
// tested only with Okta.
type SCIMUser struct {
	Schemas  []string `json:"schemas"`
	ID       string   `json:"id"`
	UserName string   `json:"userName"`
	Name     struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
	Emails []struct {
		Primary bool   `json:"primary"`
		Value   string `json:"value" format:"email"`
		Type    string `json:"type"`
		Display string `json:"display"`
	} `json:"emails"`
	// Active is a ptr to prevent the empty value from being interpreted as false.
	Active *bool         `json:"active"`
	Groups []interface{} `json:"groups"`
	Meta   struct {
		ResourceType string `json:"resourceType"`
	} `json:"meta"`
}

var SCIMAuditAdditionalFields = map[string]string{
	"automatic_actor":     "coder",
	"automatic_subsystem": "scim",
}

// scimPostUser creates a new user, or returns the existing user if it exists.
//
// @Summary SCIM 2.0: Create new user
// @ID scim-create-new-user
// @Security Authorization
// @Produce json
// @Tags Enterprise
// @Param request body legacyscim.SCIMUser true "New user"
// @Success 200 {object} legacyscim.SCIMUser
// @Router /scim/v2/Users [post]
func (s *LegacyServer) scimPostUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.scimVerifyAuthHeader(r) {
		scimUnauthorized(rw)
		return
	}

	auditor := *s.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.User](rw, &audit.RequestParams{
		Audit:            auditor,
		Log:              s.Logger,
		Request:          r,
		Action:           database.AuditActionCreate,
		AdditionalFields: SCIMAuditAdditionalFields,
	})
	defer commitAudit()

	var sUser SCIMUser
	err := json.NewDecoder(r.Body).Decode(&sUser)
	if err != nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidRequest", err))
		return
	}

	if sUser.Active == nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidRequest", xerrors.New("active field is required")))
		return
	}

	email := ""
	for _, e := range sUser.Emails {
		if e.Primary {
			email = e.Value
			break
		}
	}

	if email == "" {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidEmail", xerrors.New("no primary email provided")))
		return
	}

	//nolint:gocritic
	dbUser, err := s.Database.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
		Email:    email,
		Username: sUser.UserName,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		_ = handlerutil.WriteError(rw, err) // internal error
		return
	}
	if err == nil {
		sUser.ID = dbUser.ID.String()
		sUser.UserName = dbUser.Username

		if *sUser.Active && dbUser.Status == database.UserStatusSuspended {
			//nolint:gocritic
			newUser, err := s.Database.UpdateUserStatus(dbauthz.AsSystemRestricted(r.Context()), database.UpdateUserStatusParams{
				ID: dbUser.ID,
				// The user will get transitioned to Active after logging in.
				Status:     database.UserStatusDormant,
				UpdatedAt:  dbtime.Now(),
				UserIsSeen: false,
			})
			if err != nil {
				_ = handlerutil.WriteError(rw, err) // internal error
				return
			}
			aReq.New = newUser
		} else {
			aReq.New = dbUser
		}

		aReq.Old = dbUser

		httpapi.Write(ctx, rw, http.StatusOK, sUser)
		return
	}

	// The username is a required property in Coder. We make a best-effort
	// attempt at using what the claims provide, but if that fails we will
	// generate a random username.
	usernameValid := codersdk.NameValid(sUser.UserName)
	if usernameValid != nil {
		// If no username is provided, we can default to use the email address.
		// This will be converted in the from function below, so it's safe
		// to keep the domain.
		if sUser.UserName == "" {
			sUser.UserName = email
		}
		sUser.UserName = codersdk.UsernameFrom(sUser.UserName)
	}

	// If organization sync is enabled, the user's organizations will be
	// corrected on login. If including the default org, then always assign
	// the default org, regardless if sync is enabled or not.
	// This is to preserve single org deployment behavior.
	organizations := []uuid.UUID{}
	//nolint:gocritic // SCIM operations are a system user
	orgSync, err := s.IDPSync.OrganizationSyncSettings(dbauthz.AsSystemRestricted(ctx), s.Database)
	if err != nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusInternalServerError, "internalError", xerrors.Errorf("failed to get organization sync settings: %w", err)))
		return
	}
	if orgSync.AssignDefault {
		//nolint:gocritic // SCIM operations are a system user
		defaultOrganization, err := s.Database.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
		if err != nil {
			_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusInternalServerError, "internalError", xerrors.Errorf("failed to get default organization: %w", err)))
			return
		}
		organizations = append(organizations, defaultOrganization.ID)
	}

	//nolint:gocritic // needed for SCIM
	dbUser, err = s.AGPL.CreateUser(dbauthz.AsSystemRestricted(ctx), s.Database, agpl.CreateUserRequest{
		CreateUserRequestWithOrgs: codersdk.CreateUserRequestWithOrgs{
			Username:        sUser.UserName,
			Email:           email,
			OrganizationIDs: organizations,
		},
		LoginType: database.LoginTypeOIDC,
		// Do not send notifications to user admins as SCIM endpoint might be called sequentially to all users.
		SkipNotifications: true,
	})
	if err != nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusInternalServerError, "internalError", xerrors.Errorf("failed to create user: %w", err)))
		return
	}
	aReq.New = dbUser
	aReq.UserID = dbUser.ID

	sUser.ID = dbUser.ID.String()
	sUser.UserName = dbUser.Username

	httpapi.Write(ctx, rw, http.StatusOK, sUser)
}

// scimPatchUser supports suspending and activating users only.
//
// @Summary SCIM 2.0: Update user account
// @ID scim-update-user-status
// @Security Authorization
// @Produce application/scim+json
// @Tags Enterprise
// @Param id path string true "User ID" format(uuid)
// @Param request body legacyscim.SCIMUser true "Update user request"
// @Success 200 {object} codersdk.User
// @Router /scim/v2/Users/{id} [patch]
func (s *LegacyServer) scimPatchUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.scimVerifyAuthHeader(r) {
		scimUnauthorized(rw)
		return
	}

	auditor := *s.Auditor.Load()
	aReq, commitAudit := audit.InitRequestWithCancel[database.User](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     s.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})

	defer commitAudit(true)

	id := chi.URLParam(r, "id")

	var sUser SCIMUser
	err := json.NewDecoder(r.Body).Decode(&sUser)
	if err != nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidRequest", err))
		return
	}
	sUser.ID = id

	uid, err := uuid.Parse(id)
	if err != nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidId", xerrors.Errorf("id must be a uuid: %w", err)))
		return
	}

	//nolint:gocritic // needed for SCIM
	dbUser, err := s.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), uid)
	if err != nil {
		_ = handlerutil.WriteError(rw, err) // internal error
		return
	}
	aReq.Old = dbUser
	aReq.UserID = dbUser.ID

	if sUser.Active == nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidRequest", xerrors.New("active field is required")))
		return
	}

	newStatus := scimUserStatus(dbUser, *sUser.Active)
	if dbUser.Status != newStatus {
		//nolint:gocritic // needed for SCIM
		userNew, err := s.Database.UpdateUserStatus(dbauthz.AsSystemRestricted(r.Context()), database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     newStatus,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			_ = handlerutil.WriteError(rw, err) // internal error
			return
		}
		dbUser = userNew
	} else {
		// Do not push an audit log if there is no change.
		commitAudit(false)
	}

	aReq.New = dbUser
	httpapi.Write(ctx, rw, http.StatusOK, sUser)
}

// scimPutUser supports suspending and activating users only.
// TODO: SCIM specification requires that the PUT method should replace the entire user object.
// At present, our fields read as 'immutable' except for the 'active' field.
// See: https://datatracker.ietf.org/doc/html/rfc7644#section-3.5.1
//
// @Summary SCIM 2.0: Replace user account
// @ID scim-replace-user-status
// @Security Authorization
// @Produce application/scim+json
// @Tags Enterprise
// @Param id path string true "User ID" format(uuid)
// @Param request body legacyscim.SCIMUser true "Replace user request"
// @Success 200 {object} codersdk.User
// @Router /scim/v2/Users/{id} [put]
func (s *LegacyServer) scimPutUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.scimVerifyAuthHeader(r) {
		scimUnauthorized(rw)
		return
	}

	auditor := *s.Auditor.Load()
	aReq, commitAudit := audit.InitRequestWithCancel[database.User](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     s.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})

	defer commitAudit(true)

	id := chi.URLParam(r, "id")

	var sUser SCIMUser
	err := json.NewDecoder(r.Body).Decode(&sUser)
	if err != nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidRequest", err))
		return
	}
	sUser.ID = id
	if sUser.Active == nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidRequest", xerrors.New("active field is required")))
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "invalidId", xerrors.Errorf("id must be a uuid: %w", err)))
		return
	}

	//nolint:gocritic // needed for SCIM
	dbUser, err := s.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), uid)
	if err != nil {
		_ = handlerutil.WriteError(rw, err) // internal error
		return
	}
	aReq.Old = dbUser
	aReq.UserID = dbUser.ID

	// Technically our immutability rules dictate that we should not allow
	// fields to be changed. According to the SCIM specification, this error should
	// be returned.
	// This immutability enforcement only exists because we have not implemented it
	// yet. If these rules are causing errors, this code should be updated to allow
	// the fields to be changed.
	// TODO: Currently ignoring a lot of the SCIM fields. Coder's SCIM implementation
	// is very basic and only supports active status changes.
	if immutabilityViolation(dbUser.Username, sUser.UserName) {
		_ = handlerutil.WriteError(rw, NewHTTPError(http.StatusBadRequest, "mutability", xerrors.Errorf("username is currently an immutable field, and cannot be changed. Current: %s, New: %s", dbUser.Username, sUser.UserName)))
		return
	}

	newStatus := scimUserStatus(dbUser, *sUser.Active)
	if dbUser.Status != newStatus {
		//nolint:gocritic // needed for SCIM
		userNew, err := s.Database.UpdateUserStatus(dbauthz.AsSystemRestricted(r.Context()), database.UpdateUserStatusParams{
			ID:         dbUser.ID,
			Status:     newStatus,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		if err != nil {
			_ = handlerutil.WriteError(rw, err) // internal error
			return
		}
		dbUser = userNew
	} else {
		// Do not push an audit log if there is no change.
		commitAudit(false)
	}

	aReq.New = dbUser
	httpapi.Write(ctx, rw, http.StatusOK, sUser)
}

func immutabilityViolation[T comparable](old, newVal T) bool {
	var empty T
	if newVal == empty {
		// No change
		return false
	}
	return old != newVal
}

//nolint:revive // active is not a control flag
func scimUserStatus(user database.User, active bool) database.UserStatus {
	if !active {
		return database.UserStatusSuspended
	}

	switch user.Status {
	case database.UserStatusActive:
		// Keep the user active
		return database.UserStatusActive
	case database.UserStatusDormant, database.UserStatusSuspended:
		// Move (or keep) as dormant
		return database.UserStatusDormant
	default:
		// If the status is unknown, just move them to dormant.
		// The user will get transitioned to Active after logging in.
		return database.UserStatusDormant
	}
}
