package codersdk_test

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

func TestAgentTimeSDK(t *testing.T) {
	t.Parallel()
	for _, organization := range []string{"", uuid.NewString()} {
		t.Run("Organization/"+organization, func(t *testing.T) {
			t.Parallel()
			milliseconds := "18446744073709551614"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := "/api/v2/agent-time"
				if organization != "" {
					path = "/api/v2/organizations/" + organization + "/agent-time"
				}
				assert.Equal(t, path, r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, url.Values{
					"start_date": {"2025-01-01"}, "end_date": {"2026-01-01"}, "interval": {"month"},
					"group_by": {"user"}, "limit": {"10"}, "offset": {"20"}, "sort_by": {"name"}, "sort_order": {"asc"},
				}, r.URL.Query())
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(codersdk.AgentTimeReport{TotalAgentTimeMS: milliseconds, Buckets: []codersdk.AgentTimeBucket{{AgentTimeMS: &milliseconds}, {AgentTimeMS: nil}}})
				assert.NoError(t, err)
			}))
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			client := codersdk.New(serverURL)
			request := codersdk.AgentTimeRequest{StartDate: "2025-01-01", EndDate: "2026-01-01", Interval: codersdk.AgentTimeIntervalMonth, GroupBy: "user", Limit: 10, Offset: 20, SortBy: "name", SortOrder: "asc"}
			var report codersdk.AgentTimeReport
			if organization == "" {
				report, err = client.AgentTime(t.Context(), request)
			} else {
				report, err = client.OrganizationAgentTime(t.Context(), organization, request)
			}
			require.NoError(t, err)
			require.Equal(t, milliseconds, report.TotalAgentTimeMS)
			require.Equal(t, milliseconds, *report.Buckets[0].AgentTimeMS)
			require.Nil(t, report.Buckets[1].AgentTimeMS)
		})
	}
}
