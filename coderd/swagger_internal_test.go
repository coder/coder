package coderd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rewriteSwaggerSCIMPathForTest mirrors the path rewrite in the Swagger UI
// request interceptor.
func rewriteSwaggerSCIMPathForTest(pathname string) string {
	if pathname != swaggerGeneratedSCIMPathPrefix &&
		!strings.HasPrefix(pathname, swaggerGeneratedSCIMPathPrefix+"/") {
		return pathname
	}
	return swaggerActualSCIMPathPrefix +
		strings.TrimPrefix(pathname, swaggerGeneratedSCIMPathPrefix)
}

func TestRewriteSwaggerSCIMPath(t *testing.T) {
	t.Parallel()

	require.Contains(t, swaggerRequestInterceptor, swaggerGeneratedSCIMPathPrefix)
	require.Contains(t, swaggerRequestInterceptor, swaggerActualSCIMPathPrefix)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "scim root",
			path: "/api/v2/scim/v2",
			want: "/scim/v2",
		},
		{
			name: "scim users",
			path: "/api/v2/scim/v2/Users",
			want: "/scim/v2/Users",
		},
		{
			name: "scim user by id",
			path: "/api/v2/scim/v2/Users/00000000-0000-0000-0000-000000000000",
			want: "/scim/v2/Users/00000000-0000-0000-0000-000000000000",
		},
		{
			name: "main API route",
			path: "/api/v2/users/me",
			want: "/api/v2/users/me",
		},
		{
			name: "actual scim route",
			path: "/scim/v2/Users",
			want: "/scim/v2/Users",
		},
		{
			name: "prefix only at path start",
			path: "/prefix/api/v2/scim/v2/Users",
			want: "/prefix/api/v2/scim/v2/Users",
		},
		{
			name: "path segment boundary",
			path: "/api/v2/scim/v20/Users",
			want: "/api/v2/scim/v20/Users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, rewriteSwaggerSCIMPathForTest(tt.path))
		})
	}
}
