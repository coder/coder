package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
)

func TestWorkspaceAppCors(t *testing.T) {
	t.Parallel()

	regex, err := appurl.CompileHostnamePattern("*--apps.dev.coder.com")
	require.NoError(t, err)
	userID := uuid.New()
	user2ID := uuid.New()
	resolverErr := xerrors.New("resolve origin owner")

	methods := []string{
		http.MethodOptions,
		http.MethodHead,
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	tests := []struct {
		name            string
		origin          string
		regex           bool
		targetOwnerID   uuid.UUID
		resolvedOwner   uuid.UUID
		resolveErr      bool
		missingResolver bool
		allowed         bool
	}{
		{
			name:          "SameWorkspace",
			origin:        "https://8000--agent--ws--user--apps.dev.coder.com",
			regex:         true,
			targetOwnerID: userID,
			resolvedOwner: userID,
			allowed:       true,
		},
		{
			name:          "SameUser",
			origin:        "https://8000--agent2--ws2--user--apps.dev.coder.com",
			regex:         true,
			targetOwnerID: userID,
			resolvedOwner: userID,
			allowed:       true,
		},
		{
			name:          "DifferentOriginOwner",
			origin:        "https://3000--agent--ws--user2--apps.dev.coder.com",
			regex:         true,
			targetOwnerID: userID,
			resolvedOwner: user2ID,
			allowed:       false,
		},
		{
			name:          "OriginOwnerResolutionFails",
			origin:        "https://3000--agent--ws--user--apps.dev.coder.com",
			regex:         true,
			targetOwnerID: userID,
			resolveErr:    true,
			allowed:       false,
		},
		{
			name:          "MissingTargetOwner",
			origin:        "https://3000--agent--ws--user--apps.dev.coder.com",
			regex:         true,
			targetOwnerID: uuid.Nil,
			resolvedOwner: userID,
			allowed:       false,
		},
		{
			name:            "MissingResolver",
			origin:          "https://3000--agent--ws--user--apps.dev.coder.com",
			regex:           true,
			targetOwnerID:   userID,
			missingResolver: true,
			allowed:         false,
		},
		{
			name:          "MissingRegex",
			origin:        "https://3000--agent--ws--user--apps.dev.coder.com",
			targetOwnerID: userID,
			resolvedOwner: userID,
			allowed:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, method := range methods {
				r := httptest.NewRequest(method, "http://localhost", nil)
				r.Header.Set("Origin", test.origin)
				rw := httptest.NewRecorder()

				// Preflight requests need to know what method will be requested.
				if method == http.MethodOptions {
					r.Header.Set("Access-Control-Request-Method", method)
				}

				originResolver := func(context.Context, appurl.ApplicationURL) (uuid.UUID, error) {
					if test.resolveErr {
						return uuid.Nil, resolverErr
					}
					return test.resolvedOwner, nil
				}
				if test.missingResolver {
					originResolver = nil
				}

				testRegex := regex
				if !test.regex {
					testRegex = nil
				}

				handler := httpmw.WorkspaceAppCors(
					testRegex,
					test.targetOwnerID,
					originResolver,
				)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusNoContent)
				}))

				handler.ServeHTTP(rw, r)

				if test.allowed {
					require.Equal(t, test.origin, rw.Header().Get("Access-Control-Allow-Origin"))
				} else {
					require.Equal(t, "", rw.Header().Get("Access-Control-Allow-Origin"))
				}

				// For options we should never get to our handler as the middleware
				// short-circuits with a 200.
				if method == http.MethodOptions {
					require.Equal(t, http.StatusOK, rw.Code)
				} else {
					require.Equal(t, http.StatusNoContent, rw.Code)
				}
			}
		})
	}
}
