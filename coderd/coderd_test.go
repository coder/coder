package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	buildInfo, err := client.BuildInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, buildinfo.ExternalURL(), buildInfo.ExternalURL, "external URL")
	require.Equal(t, buildinfo.Version(), buildInfo.Version, "version")
}

// TestAuthorizeAllEndpoints will check `authorize` is called on every endpoint registered.
func TestAuthorizeAllEndpoints(t *testing.T) {
	t.Parallel()

	// skipRoutes allows skipping routes from being checked.
	type routeCheck struct {
		NoAuthorize bool
	}
	assertRoute := map[string]routeCheck{
		"GET:/api/v2":           {NoAuthorize: true},
		"GET:/api/v2/buildinfo": {NoAuthorize: true},
	}

	authorizer := &fakeAuthorizer{}
	srv, client := coderdtest.NewMemoryCoderd(t, &coderdtest.Options{
		Authorizer: authorizer,
	})
	admin := coderdtest.CreateFirstUser(t, client)
	var _ = admin

	c := srv.Config.Handler.(*chi.Mux)
	err := chi.Walk(c, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		name := method + ":" + route
		t.Run(name, func(t *testing.T) {
			authorizer.reset()
			routeAssertions, ok := assertRoute[name]
			if !ok {
				// By default, all omitted routes check for just "authorize" called
				routeAssertions = routeCheck{}
			}

			resp, err := client.Request(context.Background(), method, route, nil)
			require.NoError(t, err, "do req")
			_ = resp.Body.Close()

			if !routeAssertions.NoAuthorize {
				assert.NotNil(t, authorizer.Called, "authorizer expected")
			} else {
				assert.Nil(t, authorizer.Called, "authorize not expected")
			}
		})
		return nil
	})
	require.NoError(t, err)
}

type authCall struct {
	SubjectID string
	Roles     []string
	Action    rbac.Action
	Object    rbac.Object
}

type fakeAuthorizer struct {
	Called *authCall
}

func (f *fakeAuthorizer) AuthorizeByRoleName(ctx context.Context, subjectID string, roleNames []string, action rbac.Action, object rbac.Object) error {
	f.Called = &authCall{
		SubjectID: subjectID,
		Roles:     roleNames,
		Action:    action,
		Object:    object,
	}
	return nil
}

func (f *fakeAuthorizer) reset() {
	f.Called = nil
}
