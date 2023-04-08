package coderd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var (
	ErrAccountNotFound = errors.New("system account not found")
)

type SystemAccount struct {
	Name           string    `json:"name"`
	ID             uuid.UUID `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	OrganizationID uuid.UUID `json:"organization_id"`
	CreatedBy      string    `json:"created_by"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

type apiResponse struct {
	Error  string      `json:"error,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

var db *sql.DB

// Returns whether the systemAccount has been created or not.
//
// @Summary create system account
// @Security CoderSessionToken
// @Produce json
// @Tags SystemAccounts
// @Param request body codersdk.CreateSystemAccountRequest true "Create user request"
// @Success 200 {object} codersdk.Response
// @Router /systemaccounts [post]
func (api *API) createSystemAccount(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")

	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	id := uuid.New()
	now := time.Now()

	account := &SystemAccount{
		Name:      name,
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: r.Header.Get("User-Agent"),
	}

	// Validate if the user has owner role to be able to create SystemAccount
	// user ID should be available from context, if not add to middleware
	newAccount, err := createSystemAccountQuery(account * SystemAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(newAccount); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// @Summary Update systemaccount profile
// @ID update-systemaccount-profile
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags SystemAccount
// @Param user path string true "SystemAccount ID"
// @Param request body codersdk.UpdateSystemAccountRequest true "Updated profile"
// @Success 200 {object} codersdk.SystemAccount
// @Router /systemaccounts/{id}[put]
func (api *API) updateSystemAccount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := uuid.Parse(idStr)

	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	account, err := getSystemAccount(id)

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "account not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	name := r.FormValue("name")

	if name != "" {
		account.Name = name
		account.UpdatedAt = time.Now()

		updateSystemAccountQuery(name, time.Now(), account.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(account); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// @Summary Delete SystemAccount
// @ID delete-systemaccount
// @Security CoderSessionToken
// @Produce json
// @Tags SystemAccounts
// @Param systemaccount path string true "SystemAccount ID"
// @Success 200 {object} codersdk.SystemAccount
// @Router /systemaccount/{id} [delete]
func (api *API) deleteSystemAccount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := uuid.Parse(idStr)

	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	err := deleteSystemAccountQuery(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Returns whether the systemAccount token has been created or not.
//
// @Summary create system account token
// @Security CoderSessionToken
// @Produce json
// @Tags SystemAccounts token
// @Param request body codersdk.CreateSystemAccountRequest true "Create user request"
// @Success 200 {object} codersdk.Response
// @Router /systemaccounts/{id}/tokens [post]
func (api *API) createSystemToken(w http.ResponseWriter, r *http.Request) {
	// Get system account ID from request parameters
	vars := mux.Vars(r)
	id := vars["id"]

	// Create JWT token for system account
	token, err := systemAccountTokenProvider.CreateSystemAccountJWTToken(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(apiResponse{Error: err.Error()})
		return
	}

	// Write token response
	json.NewEncoder(w).Encode(apiResponse{Result: tokenResponse{Token: token}})
}

// @Summary Delete SystemAccountToken
// @ID delete-systemaccounttoken
// @Security CoderSessionToken
// @Produce json
// @Tags SystemAccounts
// @Param systemaccount path string true "SystemAccount ID"
// @Success 200 {object} codersdk.SystemAccount
// @Router /systemaccount/{id}/tokens/{tokenid} [delete]
func (api *API) invalidateSystemToken(w http.ResponseWriter, r *http.Request) {
	// Get token ID from request parameters
	vars := mux.Vars(r)
	id := vars["id"]

	// Invalidate JWT token for system account
	err := systemAccountTokenProvider.InvalidateSystemAccountJWTToken(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(apiResponse{Error: err.Error()})
		return
	}

	// Write success response
	json.NewEncoder(w).Encode(apiResponse{})
}
