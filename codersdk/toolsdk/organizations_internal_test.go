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
	first := codersdk.Organization{MinimalOrganization: codersdk.MinimalOrganization{ID: uuid.New(), Name: "first"}}
	second := codersdk.Organization{MinimalOrganization: codersdk.MinimalOrganization{ID: uuid.New(), Name: "second"}}
	for _, tc := range []struct {
		name, organization string
		memberships        []codersdk.Organization
		lookupFailure      bool
		wantRequest        string
		want               uuid.UUID
		wantError          []string
	}{
		{name: "ExplicitID", organization: second.ID.String(), memberships: []codersdk.Organization{first}, want: second.ID},
		{name: "ExplicitName", organization: second.Name, wantRequest: "/api/v2/organizations/second", want: second.ID},
		{name: "UnknownName", organization: "missing", wantRequest: "/api/v2/organizations/missing", wantError: []string{`resolve organization "missing"`}},
		{name: "InvalidName", organization: "../users/me", wantError: []string{"organization_id must be an organization UUID or name"}},
		{name: "ZeroUUID", organization: uuid.Nil.String(), wantError: []string{"organization_id must be a valid nonzero UUID"}},
		{name: "NoMemberships", wantRequest: "/api/v2/users/me/organizations", wantError: []string{"no organizations; use " + ToolNameListOrganizations}},
		{name: "SoleMembership", memberships: []codersdk.Organization{first}, wantRequest: "/api/v2/users/me/organizations", want: first.ID},
		{
			name:        "MultipleMemberships",
			memberships: []codersdk.Organization{second, first},
			wantRequest: "/api/v2/users/me/organizations",
			wantError: []string{
				"multiple organizations: first (" + first.ID.String() + "), second (" + second.ID.String() + ")",
				"pass the intended organization's name or ID as organization_id, or use " + ToolNameListOrganizations,
			},
		},
		{name: "LookupFailure", lookupFailure: true, wantRequest: "/api/v2/users/me/organizations", wantError: []string{"lookup failed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.NotEmpty(t, tc.wantRequest, "unexpected request to %s", r.URL.Path)
				assert.Equal(t, tc.wantRequest, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case tc.lookupFailure:
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "lookup failed"})
				case r.URL.Path == "/api/v2/organizations/second":
					_ = json.NewEncoder(w).Encode(second)
				case r.URL.Path == "/api/v2/users/me/organizations":
					_ = json.NewEncoder(w).Encode(tc.memberships)
				default:
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "not found"})
				}
			}))
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			deps, err := NewDeps(codersdk.New(serverURL))
			require.NoError(t, err)
			got, err := resolveOrganization(t.Context(), deps, tc.organization)
			if len(tc.wantError) > 0 {
				for _, want := range tc.wantError {
					require.ErrorContains(t, err, want)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}
