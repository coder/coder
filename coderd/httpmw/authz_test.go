package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestAsAuthzSystem(t *testing.T) {
	t.Parallel()
	userActor := coderdtest.RandomRBACSubject()

	base := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		actor, ok := dbauthz.ActorFromContext(r.Context())
		assert.True(t, ok, "actor should exist")
		assert.True(t, userActor.Equal(actor), "actor should be the user actor")
	})

	mwSetUser := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			r = r.WithContext(dbauthz.As(r.Context(), userActor))
			next.ServeHTTP(rw, r)
		})
	}

	mwAssertSystem := mwAssert(func(req *http.Request) {
		actor, ok := dbauthz.ActorFromContext(req.Context())
		assert.True(t, ok, "actor should exist")
		assert.False(t, userActor.Equal(actor), "systemActor should not be the user actor")
		assert.Contains(t, actor.Roles.Names(), rbac.RoleIdentifier{Name: "system"}, "should have system role")
	})

	mwAssertUser := mwAssert(func(req *http.Request) {
		actor, ok := dbauthz.ActorFromContext(req.Context())
		assert.True(t, ok, "actor should exist")
		assert.True(t, userActor.Equal(actor), "should be the useractor")
	})

	mwAssertNoUser := mwAssert(func(req *http.Request) {
		_, ok := dbauthz.ActorFromContext(req.Context())
		assert.False(t, ok, "actor should not exist")
	})

	// Request as the user actor
	const pattern = "/"
	req := httptest.NewRequest("GET", pattern, nil)
	res := httptest.NewRecorder()

	handler := chi.NewRouter()
	handler.Route(pattern, func(r chi.Router) {
		r.Use(
			// First assert there is no actor context
			mwAssertNoUser,
			httpmw.AsAuthzSystem(
				// Assert the system actor
				mwAssertSystem,
				mwAssertSystem,
			),
			// Assert no user present outside of the AsAuthzSystem chain
			mwAssertNoUser,
			// ----
			// Set to the user actor
			mwSetUser,
			// Assert the user actor
			mwAssertUser,
			httpmw.AsAuthzSystem(
				// Assert the system actor
				mwAssertSystem,
				mwAssertSystem,
			),
			// Check the user actor was returned to the context
			mwAssertUser,
		)
		r.Handle("/", base)
		r.NotFound(func(writer http.ResponseWriter, request *http.Request) {
			assert.Fail(t, "should not hit not found, the route should be correct")
		})
	})

	handler.ServeHTTP(res, req)
}

func mwAssert(assertF func(req *http.Request)) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			assertF(r)
			next.ServeHTTP(rw, r)
		})
	}
}
