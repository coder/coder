package coderd

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/imulab/go-scim/pkg/v2/handlerutil"
	scimjson "github.com/imulab/go-scim/pkg/v2/json"
	"github.com/imulab/go-scim/pkg/v2/service"
	"github.com/imulab/go-scim/pkg/v2/spec"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/scim"
)

func (api *API) scimVerifyAuthHeader(r *http.Request) bool {
	bearer := []byte("Bearer ")
	hdr := []byte(r.Header.Get("Authorization"))

	if len(hdr) >= len(bearer) && subtle.ConstantTimeCompare(hdr[:len(bearer)], bearer) == 1 {
		hdr = hdr[len(bearer):]
	}

	return len(api.SCIMAPIKey) != 0 && subtle.ConstantTimeCompare(hdr, api.SCIMAPIKey) == 1
}

// scimServiceProviderConfig returns a static SCIM service provider configuration.
//
// @Summary SCIM 2.0: Service Provider Config
// @ID scim-get-service-provider-config
// @Produce application/scim+json
// @Tags Enterprise
// @Success 200
// @Router /scim/v2/ServiceProviderConfig [get]
func (api *API) scimServiceProviderConfig(rw http.ResponseWriter, _ *http.Request) {
	// No auth needed to query this endpoint.

	rw.Header().Set("Content-Type", spec.ApplicationScimJson)
	rw.WriteHeader(http.StatusOK)

	// providerUpdated is the last time the static provider config was updated.
	// Increment this time if you make any changes to the provider config.
	providerUpdated := time.Date(2024, 10, 25, 17, 0, 0, 0, time.UTC)
	var location string
	locURL, err := api.AccessURL.Parse("/scim/v2/ServiceProviderConfig")
	if err == nil {
		location = locURL.String()
	}

	enc := json.NewEncoder(rw)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(scim.ServiceProviderConfig{
		Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		DocURI:  "https://coder.com/docs/admin/users/oidc-auth#scim-enterprise-premium",
		Patch: scim.Supported{
			Supported: true,
		},
		Bulk: scim.BulkSupported{
			Supported: false,
		},
		Filter: scim.FilterSupported{
			Supported: false,
		},
		ChangePassword: scim.Supported{
			Supported: false,
		},
		Sort: scim.Supported{
			Supported: false,
		},
		ETag: scim.Supported{
			Supported: false,
		},
		AuthSchemes: []scim.AuthenticationScheme{
			{
				Type:        "oauthbearertoken",
				Name:        "HTTP Header Authentication",
				Description: "Authentication scheme using the Authorization header with the shared token",
				DocURI:      "https://coder.com/docs/admin/users/oidc-auth#scim-enterprise-premium",
			},
		},
		Meta: scim.ServiceProviderMeta{
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
func (api *API) scimGetUsers(rw http.ResponseWriter, r *http.Request) {
	if !api.scimVerifyAuthHeader(r) {
		_ = handlerutil.WriteError(rw, spec.Error{Status: http.StatusUnauthorized, Type: "invalidAuthorization"})
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
func (api *API) scimGetUser(rw http.ResponseWriter, r *http.Request) {
	if !api.scimVerifyAuthHeader(r) {
		_ = handlerutil.WriteError(rw, spec.Error{Status: http.StatusUnauthorized, Type: "invalidAuthorization"})
		return
	}

	_ = handlerutil.WriteError(rw, spec.ErrNotFound)
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
	Active bool          `json:"active"`
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
// @Param request body coderd.SCIMUser true "New user"
// @Success 200 {object} coderd.SCIMUser
// @Router /scim/v2/Users [post]
func (api *API) scimPostUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.scimVerifyAuthHeader(r) {
		_ = handlerutil.WriteError(rw, spec.Error{Status: http.StatusUnauthorized, Type: "invalidAuthorization"})
		return
	}

	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.User](rw, &audit.RequestParams{
		Audit:            auditor,
		Log:              api.Logger,
		Request:          r,
		Action:           database.AuditActionCreate,
		AdditionalFields: SCIMAuditAdditionalFields,
	})
	defer commitAudit()

	var sUser SCIMUser
	err := json.NewDecoder(r.Body).Decode(&sUser)
	if err != nil {
		_ = handlerutil.WriteError(rw, err)
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
		_ = handlerutil.WriteError(rw, spec.Error{Status: http.StatusBadRequest, Type: "invalidEmail"})
		return
	}

	//nolint:gocritic
	dbUser, err := api.Database.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
		Email:    email,
		Username: sUser.UserName,
	})
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		_ = handlerutil.WriteError(rw, err)
		return
	}
	if err == nil {
		sUser.ID = dbUser.ID.String()
		sUser.UserName = dbUser.Username

		if sUser.Active && dbUser.Status == database.UserStatusSuspended {
			//nolint:gocritic
			newUser, err := api.Database.UpdateUserStatus(dbauthz.AsSystemRestricted(r.Context()), database.UpdateUserStatusParams{
				ID: dbUser.ID,
				// The user will get transitioned to Active after logging in.
				Status:    database.UserStatusDormant,
				UpdatedAt: dbtime.Now(),
			})
			if err != nil {
				_ = handlerutil.WriteError(rw, err)
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
	if api.IDPSync.AssignDefaultOrganization() {
		//nolint:gocritic // SCIM operations are a system user
		defaultOrganization, err := api.Database.GetDefaultOrganization(dbauthz.AsSystemRestricted(ctx))
		if err != nil {
			_ = handlerutil.WriteError(rw, err)
			return
		}
		organizations = append(organizations, defaultOrganization.ID)
	}

	//nolint:gocritic // needed for SCIM
	dbUser, err = api.AGPL.CreateUser(dbauthz.AsSystemRestricted(ctx), api.Database, agpl.CreateUserRequest{
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
		_ = handlerutil.WriteError(rw, err)
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
// @Param request body coderd.SCIMUser true "Update user request"
// @Success 200 {object} codersdk.User
// @Router /scim/v2/Users/{id} [patch]
func (api *API) scimPatchUser(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !api.scimVerifyAuthHeader(r) {
		_ = handlerutil.WriteError(rw, spec.Error{Status: http.StatusUnauthorized, Type: "invalidAuthorization"})
		return
	}

	auditor := *api.AGPL.Auditor.Load()
	aReq, commitAudit := audit.InitRequestWithCancel[database.User](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})

	defer commitAudit(true)

	id := chi.URLParam(r, "id")

	var sUser SCIMUser
	err := json.NewDecoder(r.Body).Decode(&sUser)
	if err != nil {
		_ = handlerutil.WriteError(rw, err)
		return
	}
	sUser.ID = id

	uid, err := uuid.Parse(id)
	if err != nil {
		_ = handlerutil.WriteError(rw, spec.Error{Status: http.StatusBadRequest, Type: "invalidId"})
		return
	}

	//nolint:gocritic // needed for SCIM
	dbUser, err := api.Database.GetUserByID(dbauthz.AsSystemRestricted(ctx), uid)
	if err != nil {
		_ = handlerutil.WriteError(rw, err)
		return
	}
	aReq.Old = dbUser
	aReq.UserID = dbUser.ID

	var status database.UserStatus
	if sUser.Active {
		switch dbUser.Status {
		case database.UserStatusActive:
			// Keep the user active
			status = database.UserStatusActive
		case database.UserStatusDormant, database.UserStatusSuspended:
			// Move (or keep) as dormant
			status = database.UserStatusDormant
		default:
			// If the status is unknown, just move them to dormant.
			// The user will get transitioned to Active after logging in.
			status = database.UserStatusDormant
		}
	} else {
		status = database.UserStatusSuspended
	}

	if dbUser.Status != status {
		//nolint:gocritic // needed for SCIM
		userNew, err := api.Database.UpdateUserStatus(dbauthz.AsSystemRestricted(r.Context()), database.UpdateUserStatusParams{
			ID:        dbUser.ID,
			Status:    status,
			UpdatedAt: dbtime.Now(),
		})
		if err != nil {
			_ = handlerutil.WriteError(rw, err)
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
