package coderd

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/imulab/go-scim/pkg/v2/handlerutil"
	scimjson "github.com/imulab/go-scim/pkg/v2/json"
	"github.com/imulab/go-scim/pkg/v2/service"
	"github.com/imulab/go-scim/pkg/v2/spec"

	agpl "github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (api *API) scimEnabledMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		api.entitlementsMu.RLock()
		scim := api.entitlements.Features[codersdk.FeatureSCIM].Enabled
		api.entitlementsMu.RUnlock()

		if !scim {
			httpapi.RouteNotFound(rw)
			return
		}

		next.ServeHTTP(rw, r)
	})
}

func (api *API) scimVerifyAuthHeader(r *http.Request) bool {
	hdr := []byte(r.Header.Get("Authorization"))

	return len(api.SCIMAPIKey) != 0 && subtle.ConstantTimeCompare(hdr, api.SCIMAPIKey) == 1
}

// scimGetUsers intentionally always returns no users. This is done to always force
// Okta to try and create each user individually, this way we don't need to
// implement fetching users twice.
//
// @Summary SCIM 2.0: Get users
// @ID scim-get-users
// @Security CoderSessionToken
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
// scimGetUsers intentionally always returns no users. This is done to always force
// Okta to try and create each user individually, this way we don't need to
// implement fetching users twice.
//
// @Summary SCIM 2.0: Get user by ID
// @ID scim-get-user-by-id
// @Security CoderSessionToken
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

// scimPostUser creates a new user, or returns the existing user if it exists.
//
// @Summary SCIM 2.0: Create new user
// @ID scim-create-new-user
// @Security CoderSessionToken
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

	//nolint:gocritic // needed for SCIM
	user, _, err := api.AGPL.CreateUser(dbauthz.AsSystemRestricted(ctx), api.Database, agpl.CreateUserRequest{
		CreateUserRequest: codersdk.CreateUserRequest{
			Username: sUser.UserName,
			Email:    email,
		},
		LoginType: database.LoginTypeOIDC,
	})
	if err != nil {
		_ = handlerutil.WriteError(rw, err)
		return
	}

	sUser.ID = user.ID.String()
	sUser.UserName = user.Username

	httpapi.Write(ctx, rw, http.StatusOK, sUser)
}

// scimPatchUser supports suspending and activating users only.
//
// @Summary SCIM 2.0: Update user account
// @ID scim-update-user-status
// @Security CoderSessionToken
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

	var status database.UserStatus
	if sUser.Active {
		status = database.UserStatusActive
	} else {
		status = database.UserStatusSuspended
	}

	//nolint:gocritic // needed for SCIM
	_, err = api.Database.UpdateUserStatus(dbauthz.AsSystemRestricted(r.Context()), database.UpdateUserStatusParams{
		ID:        dbUser.ID,
		Status:    status,
		UpdatedAt: database.Now(),
	})
	if err != nil {
		_ = handlerutil.WriteError(rw, err)
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, sUser)
}
