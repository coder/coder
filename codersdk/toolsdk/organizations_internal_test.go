package toolsdk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestResolveOrganization(t *testing.T) {
	t.Parallel()
	first, second := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name, organizationID string
		memberships          []uuid.UUID
		lookupFailure        bool
		want                 uuid.UUID
		wantError            string
	}{
		{name: "Explicit", organizationID: second.String(), memberships: []uuid.UUID{first}, want: second},
		{name: "Invalid", organizationID: "invalid", wantError: "organization_id must be a valid UUID"},
		{name: "ZeroUUID", organizationID: uuid.Nil.String(), wantError: "organization_id must be a valid nonzero UUID"},
		{name: "NoMemberships", wantError: "no organizations; use " + ToolNameListOrganizations},
		{name: "SoleMembership", memberships: []uuid.UUID{first}, want: first},
		{name: "MultipleMemberships", memberships: []uuid.UUID{first, second}, wantError: "multiple organizations; use " + ToolNameListOrganizations},
		{name: "LookupFailure", lookupFailure: true, wantError: "lookup failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Empty(t, tc.organizationID, "explicit IDs must not trigger a user lookup")
				assert.Equal(t, "/api/v2/users/me", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if tc.lookupFailure {
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "lookup failed"})
					return
				}
				_ = json.NewEncoder(w).Encode(codersdk.User{OrganizationIDs: tc.memberships})
			}))
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			deps, err := NewDeps(codersdk.New(serverURL))
			require.NoError(t, err)
			got, err := resolveOrganization(t.Context(), deps, tc.organizationID)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}
