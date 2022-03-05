package httpmw

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

// parseUUID consumes a url parameter and parses it as a UUID.
func parseUUID(rw http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	rawID := chi.URLParam(r, param)
	if rawID == "" {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("%s must be provided", param),
		})
		return uuid.UUID{}, false
	}
	parsed, err := uuid.Parse(rawID)
	if err != nil {
		httpapi.Write(rw, http.StatusBadRequest, httpapi.Response{
			Message: fmt.Sprintf("%s must be a uuid", param),
		})
		return uuid.UUID{}, false
	}
	return parsed, true
}

func fetchOrganization(rw http.ResponseWriter, r *http.Request, db database.Store, organizationID string) (database.Organization, database.OrganizationMember, bool) {
	apiKey := APIKey(r)
	organization, err := db.GetOrganizationByID(r.Context(), organizationID)
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization: %s", err.Error()),
		})
		return organization, database.OrganizationMember{}, false
	}
	organizationMember, err := db.GetOrganizationMemberByUserID(r.Context(), database.GetOrganizationMemberByUserIDParams{
		OrganizationID: organization.ID,
		UserID:         apiKey.UserID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
			Message: "not a member of the organization",
		})
		return organization, organizationMember, false
	}
	if err != nil {
		httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
			Message: fmt.Sprintf("get organization member: %s", err.Error()),
		})
		return organization, organizationMember, false
	}
	return organization, organizationMember, true
}
