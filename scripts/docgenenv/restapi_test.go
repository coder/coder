package docgenenv_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

func TestRESTAPIRoutes(t *testing.T) {
	t.Parallel()

	// CRLF line endings, as in a Windows checkout, are JSON whitespace.
	swagger := strings.ReplaceAll(`{
  "paths": {
    "/users": {
      "parameters": [],
      "get": {"tags": ["Users"]},
      "post": {"tags": ["Users", "Organizations"]}
    },
    "/": {"get": {}},
    "/ports": {"get": {"tags": ["Port Sharing"]}},
    "/ai": {"get": {"tags": ["AI Gateway"]}},
    "/mcp": {"get": {"tags": ["MCP"], "x-apidocgen": {"skip": true}}}
  }
}`, "\n", "\r\n")
	routes, err := docgenenv.RESTAPIRoutes([]byte(swagger))
	require.NoError(t, err)
	// An operation counts under its first tag only, a tag whose operations
	// are all skipped gets no page, and an untagged operation is General.
	require.Equal(t, []docgenenv.Route{
		{Title: "General", Path: "./reference/api/general.md"},
		{Title: "AI Gateway", Path: "./reference/api/aigateway.md"},
		{Title: "Authentication", Path: "./reference/api/authentication.md"},
		{Title: "Port Sharing", Path: "./reference/api/portsharing.md"},
		{Title: "Schemas", Path: "./reference/api/schemas.md"},
		{Title: "Users", Path: "./reference/api/users.md"},
	}, routes)
}

func TestRESTAPIRoutesErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		swagger string
		wantErr string
	}{
		{
			name:    "SamePage",
			swagger: `{"paths": {"/a": {"get": {"tags": ["Port Sharing"]}}, "/b": {"get": {"tags": ["PortSharing"]}}}}`,
			wantErr: `Swagger tag "PortSharing" would have the page ./reference/api/portsharing.md, which is the page of Swagger tag "Port Sharing"`,
		},
		{
			name:    "HandWrittenPage",
			swagger: `{"paths": {"/a": {"get": {"tags": ["Authentication"]}}}}`,
			wantErr: "which is the hand-written Authentication page",
		},
		{
			name:    "SchemasPage",
			swagger: `{"paths": {"/a": {"get": {"tags": ["Schemas"]}}}}`,
			wantErr: "which is the schemas page",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := docgenenv.RESTAPIRoutes([]byte(tc.swagger))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
