package codersdk

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/wsrelated"
)

func TestWorkspaceFilterAsRequestOption(t *testing.T) {
	t.Parallel()

	// applyFilter applies the filter's request option to a blank request and
	// returns the resulting "q" query parameter.
	applyFilter := func(f WorkspaceFilter) string {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
		require.NoError(t, err)
		f.asRequestOption()(req)
		return req.URL.Query().Get("q")
	}

	tests := []struct {
		name     string
		filter   WorkspaceFilter
		contains []string
		empty    bool
	}{
		{
			name:   "Empty",
			filter: WorkspaceFilter{},
			empty:  true,
		},
		{
			name:     "Owner",
			filter:   WorkspaceFilter{Owner: "alice"},
			contains: []string{`owner:"alice"`},
		},
		{
			name:     "Name",
			filter:   WorkspaceFilter{Name: "my-workspace"},
			contains: []string{`name:"my-workspace"`},
		},
		{
			name:     "Template",
			filter:   WorkspaceFilter{Template: "base"},
			contains: []string{`template:"base"`},
		},
		{
			name:     "Status",
			filter:   WorkspaceFilter{Status: "running"},
			contains: []string{`status:"running"`},
		},
		{
			name:     "Organization",
			filter:   WorkspaceFilter{Organization: "acme"},
			contains: []string{`organization:"acme"`},
		},
		{
			name:     "OrganizationByUUID",
			filter:   WorkspaceFilter{Organization: "550e8400-e29b-41d4-a716-446655440000"},
			contains: []string{`organization:"550e8400-e29b-41d4-a716-446655440000"`},
		},
		{
			name:   "MultipleFields",
			filter: WorkspaceFilter{Owner: "alice", Organization: "acme", Status: "running"},
			contains: []string{
				`owner:"alice"`,
				`organization:"acme"`,
				`status:"running"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := applyFilter(tt.filter)
			if tt.empty {
				require.Empty(t, q)
				return
			}
			for _, s := range tt.contains {
				require.Contains(t, q, s)
			}
		})
	}
}

// TestWorkspaceOptionsAsRequestOption verifies that WorkspaceOptions sets the
// include_related query parameter from its IncludeRelated selection.
func TestWorkspaceOptionsAsRequestOption(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	require.NoError(t, err)

	config := wsrelated.Config{Template: true}
	WorkspaceOptions{IncludeRelated: &config}.asRequestOption()(req)

	require.Equal(t, config.QueryParam(), req.URL.Query().Get("include_related"))
}
